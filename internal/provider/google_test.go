package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

func TestGoogleSendsKeyInHeaderOnly(t *testing.T) {
	var gotHeader, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-goog-api-key")
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hallo"}]},"finishReason":"STOP"}]}`))
	}))
	defer srv.Close()

	p := NewGoogleProvider(config.ProviderConfig{APIKey: "secret", BaseURL: srv.URL}, srv.Client())
	if _, err := p.Translate(context.Background(), TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "de"}); err != nil {
		t.Fatal(err)
	}
	if gotHeader != "secret" {
		t.Errorf("x-goog-api-key = %q", gotHeader)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want empty", gotQuery)
	}
}
