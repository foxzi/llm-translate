package provider

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/llm-translate/internal/config"
)

func TestHTTPErrorFromProviders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>Bad Gateway</body></html>"))
	}))
	defer srv.Close()

	cfg := config.ProviderConfig{APIKey: "k", BaseURL: srv.URL, Model: "m"}
	factories := map[string]func(config.ProviderConfig, *http.Client) Provider{
		"openai":     NewOpenAIProvider,
		"openrouter": NewOpenRouterProvider,
		"vllm":       NewVLLMProvider,
		"ollama":     NewOllamaProvider,
		"anthropic":  NewAnthropicProvider,
		"google":     NewGoogleProvider,
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			p := factory(cfg, srv.Client())
			_, errTranslate := p.Translate(t.Context(), TranslateRequest{Text: "x", SourceLang: "en", TargetLang: "de"})
			_, errAnalyze := p.AnalyzeSentiment(t.Context(), "x")
			for _, err := range []error{errTranslate, errAnalyze} {
				var he *HTTPError
				if !errors.As(err, &he) {
					t.Fatalf("error %v is not an HTTPError", err)
				}
				if he.StatusCode != http.StatusBadGateway || he.RetryAfter != 7*time.Second || !strings.Contains(he.Message, "Bad Gateway") {
					t.Errorf("HTTPError = %+v", he)
				}
			}
		})
	}
}

func TestErrorMessage(t *testing.T) {
	long := strings.Repeat("я", 500)
	tests := []struct {
		name, body, want string
	}{
		{"openai", `{"error":{"message":"bad model","type":"x"}}`, "bad model"},
		{"string error", `{"error":"model not found"}`, "model not found"},
		{"top-level message", `{"message":"slow down"}`, "slow down"},
		{"html", "  <html>502</html>\n", "<html>502</html>"},
		{"empty", "", ""},
		{"long", long, strings.Repeat("я", maxErrorBody) + "..."},
	}
	for _, tc := range tests {
		if got := errorMessage([]byte(tc.body)); got != tc.want {
			t.Errorf("%s: errorMessage = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("30"); got != 30*time.Second {
		t.Errorf("seconds = %v", got)
	}
	date := time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(date); got < 18*time.Second || got > 20*time.Second {
		t.Errorf("date = %v", got)
	}
	for _, v := range []string{"", "abc", "-5", "0", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)} {
		if got := parseRetryAfter(v); got != 0 {
			t.Errorf("parseRetryAfter(%q) = %v, want 0", v, got)
		}
	}
}
