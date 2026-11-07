package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

func TestTranslateRejectsTruncatedAndEmpty(t *testing.T) {
	tests := []struct {
		name    string
		factory func(config.ProviderConfig, *http.Client) Provider
		body    string
		want    string // empty means success
	}{
		{"openai ok", NewOpenAIProvider, `{"choices":[{"message":{"content":"Hallo"},"finish_reason":"stop"}]}`, ""},
		{"openai length", NewOpenAIProvider, `{"choices":[{"message":{"content":"Hal"},"finish_reason":"length"}]}`, "finish_reason=length"},
		{"openai empty", NewOpenAIProvider, `{"choices":[{"message":{"content":"  \n"},"finish_reason":"stop"}]}`, "empty response"},
		{"openrouter length", NewOpenRouterProvider, `{"choices":[{"message":{"content":"Hal"},"finish_reason":"length"}]}`, "finish_reason=length"},
		{"vllm length", NewVLLMProvider, `{"choices":[{"message":{"content":"Hal"},"finish_reason":"length"}]}`, "finish_reason=length"},
		{"gemma length", NewVLLMProvider, `{"choices":[{"message":{"content":"Hal"},"finish_reason":"length"}]}`, "finish_reason=length"},
		{"anthropic ok", NewAnthropicProvider, `{"content":[{"type":"text","text":"Hallo"}],"stop_reason":"end_turn"}`, ""},
		{"anthropic max_tokens", NewAnthropicProvider, `{"content":[{"type":"text","text":"Hal"}],"stop_reason":"max_tokens"}`, "finish_reason=max_tokens"},
		{"anthropic empty", NewAnthropicProvider, `{"content":[{"type":"text","text":" "}],"stop_reason":"end_turn"}`, "empty response"},
		{"google ok", NewGoogleProvider, `{"candidates":[{"content":{"parts":[{"text":"Hallo"}]},"finishReason":"STOP"}]}`, ""},
		{"google MAX_TOKENS", NewGoogleProvider, `{"candidates":[{"content":{"parts":[{"text":"Hal"}]},"finishReason":"MAX_TOKENS"}]}`, "finish_reason=MAX_TOKENS"},
		{"google safety without parts", NewGoogleProvider, `{"candidates":[{"content":{},"finishReason":"SAFETY"}]}`, "empty response"},
		{"ollama length", NewOllamaProvider, `{"response":"Hal","done":true,"done_reason":"length"}`, "finish_reason=length"},
		{"ollama empty", NewOllamaProvider, `{"response":"","done":true}`, "empty response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			model := ""
			if tt.name == "gemma length" {
				model = "translategemma-4b"
			}
			p := tt.factory(config.ProviderConfig{APIKey: "k", BaseURL: srv.URL, Model: model}, srv.Client())
			resp, err := p.Translate(context.Background(), TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "de"})
			if tt.want == "" {
				if err != nil || resp.Text != "Hallo" {
					t.Fatalf("got %q, %v", resp.Text, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}
