package translator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

func TestTranslateReusesHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hallo"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.DefaultProvider = "openai"
	cfg.Providers["openai"] = config.ProviderConfig{APIKey: "k", BaseURL: srv.URL, Model: "m"}
	tr := New(cfg, false)

	var first *http.Client
	for i := 0; i < 2; i++ {
		if _, err := tr.Translate(context.Background(), TranslateRequest{Text: "hi", SourceLang: "en", TargetLang: "de"}); err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = tr.client
		} else if tr.client != first {
			t.Error("HTTP client was rebuilt on the second Translate call")
		}
	}
}

func TestProxyClientIsShared(t *testing.T) {
	cfg := config.ProxyConfig{URL: "http://127.0.0.1:3128", NoProxy: []string{"localhost"}}
	a, err := proxyClient(cfg, 30)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := proxyClient(cfg, 30); a != b {
		t.Error("same proxy settings gave different clients")
	}
	if c, _ := proxyClient(cfg, 31); a == c {
		t.Error("different timeout shared a client")
	}
}
