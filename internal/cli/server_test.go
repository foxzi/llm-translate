package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

const testArticle = "# Tesla sales\n\nTesla reported record sales last quarter."

// resetFlags restores flag variables to their CLI defaults, as the API server sees them.
func resetFlags() { newRootCmd(context.Background()) }

func postTranslate(t *testing.T, cfg *config.Config, token, body string) (int, []byte) {
	t.Helper()
	resetFlags()
	req := httptest.NewRequest(http.MethodPost, "/v1/translate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	newServerHandler(cfg, nil).ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	data, _ := io.ReadAll(rec.Body)
	return rec.Code, data
}

func articleBody(from, to string) string {
	b, _ := json.Marshal(map[string]string{"text": testArticle, "from": from, "to": to})
	return string(b)
}

// TestAPINewsPortal reproduces the NewsPortal client: its config (vllm +
// TranslateGemma, all analyses), a Bearer token and {text, from, to}.
func TestAPINewsPortal(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "vllm", "translategemma-12b")
	cfg.Settings.APIToken = "secret"

	code, body := postTranslate(t, cfg, "secret", articleBody("en", "ru"))
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	checkGolden(t, "api_newsportal", body)
}

func TestAPIOpenAI(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")

	code, body := postTranslate(t, cfg, "", articleBody("en", "de"))
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	checkGolden(t, "api_openai", body)
}

func TestAPIParallelAnalysis(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.ParallelAnalysis = true

	code, body := postTranslate(t, cfg, "", articleBody("en", "de"))
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	checkGolden(t, "api_openai_parallel", body)
}

func TestAPINoAnalysis(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings = config.DefaultConfig().Settings

	code, body := postTranslate(t, cfg, "", articleBody("en", "de"))
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	checkGolden(t, "api_no_analysis", body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if _, ok := out["analysis"]; ok {
		t.Errorf("analysis must be omitted when no analyses are enabled: %s", body)
	}
	if len(f.requests) != 1 {
		t.Errorf("expected 1 LLM request, got %d", len(f.requests))
	}
}

func TestAPIDefaultLanguages(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings = config.DefaultConfig().Settings

	// "to" falls back to default_target_language (ru), "from" to auto-detection.
	code, body := postTranslate(t, cfg, "", `{"text":"hello"}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var out struct{ Text string }
	_ = json.Unmarshal(body, &out)
	if out.Text != "[ru] hello" {
		t.Errorf("text = %q", out.Text)
	}
	if !strings.Contains(f.requests[0], "Detect the source language") {
		t.Errorf("expected auto-detect prompt, got %q", f.requests[0])
	}
}

func TestAPIGemmaRequiresSource(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "vllm", "translategemma-12b")

	code, body := postTranslate(t, cfg, "", `{"text":"hello","to":"ru"}`)
	if code != http.StatusBadGateway || !strings.Contains(string(body), "requires explicit source language") {
		t.Fatalf("status %d: %s", code, body)
	}
}

func TestAPIAuth(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.APIToken = "secret"

	for _, token := range []string{"", "wrong"} {
		code, body := postTranslate(t, cfg, token, articleBody("en", "de"))
		if code != http.StatusUnauthorized {
			t.Fatalf("token %q: status %d", token, code)
		}
		checkGolden(t, "api_unauthorized", body)
	}
	if len(f.requests) != 0 {
		t.Errorf("unauthorized requests reached the provider")
	}
}

func TestAPIBadRequest(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")

	code, body := postTranslate(t, cfg, "", `{"text":`)
	if code != http.StatusBadRequest || !strings.Contains(string(body), `"error":"invalid JSON`) {
		t.Fatalf("status %d: %s", code, body)
	}
	code, body = postTranslate(t, cfg, "", `{"text":"  ","to":"de"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", code, body)
	}
	checkGolden(t, "api_empty_text", body)
}

func TestAPIProviderError(t *testing.T) {
	f := newFakeLLM(t)
	f.failStatus, f.failMsg = http.StatusBadRequest, "model not found"
	cfg := testConfig(t, f, "openai", "gpt-test")

	code, body := postTranslate(t, cfg, "", articleBody("en", "de"))
	if code != http.StatusBadGateway {
		t.Fatalf("status %d: %s", code, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "model not found") {
		t.Errorf("error = %q", msg)
	}
}

func TestAPIHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newServerHandler(config.DefaultConfig(), nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	checkGolden(t, "health", rec.Body.Bytes())
}
