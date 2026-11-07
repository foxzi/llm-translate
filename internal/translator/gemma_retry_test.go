package translator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

func TestRetryTemperature(t *testing.T) {
	for _, tc := range []struct {
		base  float64
		retry int
		want  float64
	}{{0.3, 1, 0.5}, {0.3, 2, 0.7}, {0.3, 5, 1.0}, {1.5, 1, 1.5}} {
		if got := retryTemperature(tc.base, tc.retry); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("retryTemperature(%v, %d) = %v, want %v", tc.base, tc.retry, got, tc.want)
		}
	}
}

// TranslateGemma ignores the retry notice in Context, so strong retries must
// differ by temperature.
func TestStrongRetriesRaiseTemperatureForGemma(t *testing.T) {
	var temps []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Temperature float64 `json:"temperature"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		temps = append(temps, req.Temperature)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"the quick fox and the dog that will see this one"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.DefaultProvider = "vllm"
	cfg.Providers["vllm"] = config.ProviderConfig{BaseURL: srv.URL, Model: "translategemma-4b"}
	cfg.StrongValidation.Enabled = true
	cfg.Settings.RetryCount = 0

	_, err := New(cfg, false).Translate(context.Background(), TranslateRequest{
		Text: "x", SourceLang: "en", TargetLang: "ru", Temperature: 0.3, StrongMode: true, StrongRetries: 2,
	})
	if err == nil {
		t.Fatal("expected strong validation to fail")
	}
	if want := []float64{0.3, 0.5, 0.7}; !reflect.DeepEqual(temps, want) {
		t.Errorf("temperatures = %v, want %v", temps, want)
	}
}
