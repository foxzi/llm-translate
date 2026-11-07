package translator

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsageTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)
	}))
	defer srv.Close()

	client := withUsageTransport(&http.Client{})
	var u Usage
	ctx := WithUsage(context.Background(), &u)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(body) == 0 {
			t.Fatal("body was consumed by the transport")
		}
	}
	if u.PromptTokens.Load() != 20 || u.CompletionTokens.Load() != 8 {
		t.Fatalf("got %d/%d, want 20/8", u.PromptTokens.Load(), u.CompletionTokens.Load())
	}
}

func TestUsageCostAndRateLimit(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"rate limit"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"usage":{"prompt_tokens":1,"completion_tokens":1,"cost":0.25}}`)
	}))
	defer srv.Close()

	client := withUsageTransport(&http.Client{})
	var u Usage
	ctx := WithUsage(context.Background(), &u)
	do := func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if _, ok := u.Cost(); ok {
		t.Fatal("cost reported before any response")
	}
	do()
	do()
	if c, ok := u.Cost(); !ok || c != 0.5 {
		t.Errorf("cost = %v, %v", c, ok)
	}
	if limited, _ := u.RateLimited(); limited {
		t.Error("rate limited after 200")
	}
	status = http.StatusTooManyRequests
	do()
	if limited, after := u.RateLimited(); !limited || after != 7 {
		t.Errorf("rate limited = %v, retry after %d", limited, after)
	}
}
