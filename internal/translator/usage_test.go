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
