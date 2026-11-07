package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

func TestStripReasoning(t *testing.T) {
	tests := map[string]string{
		"<think>x</think>\nHallo":               "Hallo",
		"<think>a\nb</think>Hi<think>c</think>": "Hi",
		"  plain  ":                             "plain",
		"<think>never closed":                   "",
	}
	for in, want := range tests {
		if got := StripReasoning(in); got != want {
			t.Errorf("StripReasoning(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTranslateStripsReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"<think>x</think>\nHallo"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{APIKey: "k", BaseURL: srv.URL}, srv.Client())
	resp, err := p.Translate(context.Background(), TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "de"})
	if err != nil || resp.Text != "Hallo" {
		t.Fatalf("got %q, %v", resp.Text, err)
	}
}

func TestTranslateUnclosedThinkIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"<think>reasoning"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{APIKey: "k", BaseURL: srv.URL}, srv.Client())
	_, err := p.Translate(context.Background(), TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "de"})
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseCombinedIgnoresReasoning(t *testing.T) {
	resp := ParseCombinedResponse("<think>\nSENTIMENT: neutral (0.0)\n</think>\nSENTIMENT: positive (0.6)", CombinedAnalysisRequest{Sentiment: true})
	if resp.Sentiment == nil || resp.Sentiment.Sentiment != "positive" || resp.Sentiment.Score != 0.6 {
		t.Fatalf("sentiment = %+v", resp.Sentiment)
	}
}
