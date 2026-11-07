package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeWaitsForInflightRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, srv, ln, 10*time.Second) }()

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		got <- result{string(b), err}
	}()

	<-started
	cancel()
	select {
	case err := <-served:
		t.Fatalf("serve returned while a request was in flight: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	if err := <-served; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if r := <-got; r.err != nil || r.body != "done" {
		t.Errorf("response = %q, err = %v", r.body, r.err)
	}
}

func TestServeShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, srv, ln, 100*time.Millisecond) }()

	go func() { _, _ = http.Get("http://" + ln.Addr().String()) }()
	<-started
	cancel()
	if err := <-served; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("serve = %v, want deadline exceeded", err)
	}
}

func TestPublicMessageStripsURLs(t *testing.T) {
	err := fmt.Errorf(`Post "https://llm.internal:8443/v1/chat/completions?key=SECRET": dial tcp: refused, see http://x.test/a`)
	got := publicMessage(err)
	if strings.Contains(got, "llm.internal") || strings.Contains(got, "SECRET") || strings.Contains(got, "x.test") {
		t.Errorf("URL leaked: %s", got)
	}
	if !strings.Contains(got, "<url>") || !strings.Contains(got, "dial tcp: refused") {
		t.Errorf("message = %q", got)
	}
}

func TestAPIProviderErrorHidesURLs(t *testing.T) {
	f := newFakeLLM(t)
	f.failStatus, f.failMsg = http.StatusBadRequest, "cannot reach https://backend.internal/v1?key=SECRET now"
	cfg := testConfig(t, f, "openai", "gpt-test")

	code, body := postTranslate(t, cfg, "", articleBody("en", "de"))
	out := decodeExt(t, body)
	if code != http.StatusBadGateway || out.Code != codeProviderError {
		t.Fatalf("status %d: %s", code, body)
	}
	if strings.Contains(out.Error, "backend.internal") || strings.Contains(out.Error, f.URL) {
		t.Errorf("URL leaked: %s", out.Error)
	}
	if !strings.Contains(out.Error, "cannot reach <url>") {
		t.Errorf("error = %q", out.Error)
	}
}

// A request that is slow to arrive must not hold a max_concurrent slot.
func TestAPIBodyReadBeforeSlot(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.MaxConcurrent = 1
	resetFlags()
	ts := httptest.NewServer(newServerHandler(cfg, nil))
	defer ts.Close()

	// The first request sends headers and half of the body, then stalls.
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "POST /v1/translate HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{\"text\":")

	time.Sleep(100 * time.Millisecond)
	resp, err := http.Post(ts.URL+"/v1/translate", "application/json", strings.NewReader(articleBody("en", "de")))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d: the stalled request holds the slot", resp.StatusCode)
	}
}

// The fake LLM echoes the text, so with strong validation on an English
// source that has many common words counts as untranslated.
func TestAPIStrongValidationFromConfig(t *testing.T) {
	body := `{"text":"the quick fox and the dog that will see this one with all of it","from":"en","to":"de","analysis":[]}`
	for _, enabled := range []bool{false, true} {
		f := newFakeLLM(t)
		cfg := testConfig(t, f, "openai", "gpt-test")
		cfg.StrongValidation.Enabled = enabled
		cfg.StrongValidation.MaxRetries = 1
		cfg.Settings.RetryCount = 0

		code, out := postTranslate(t, cfg, "", body)
		want := http.StatusOK
		if enabled {
			want = http.StatusBadGateway
		}
		if code != want {
			t.Errorf("enabled=%v: status %d, want %d: %s", enabled, code, want, out)
		}
	}
}
