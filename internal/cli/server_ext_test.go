package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type extResponse struct {
	Text             string         `json:"text"`
	Title            string         `json:"title"`
	Summary          string         `json:"summary"`
	DetectedLanguage string         `json:"detected_language"`
	Analysis         map[string]any `json:"analysis"`
	AnalysisErrors   []string       `json:"analysis_errors"`
	Usage            struct {
		PromptTokens int64    `json:"prompt_tokens"`
		Provider     string   `json:"provider"`
		Model        string   `json:"model"`
		Cost         *float64 `json:"cost"`
	} `json:"usage"`
	Error      string `json:"error"`
	Code       string `json:"code"`
	RetryAfter int    `json:"retry_after"`
}

func decodeExt(t *testing.T, body []byte) extResponse {
	t.Helper()
	var out extResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return out
}

func TestAPITitleAndSummary(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")

	code, body := postTranslate(t, cfg, "", `{"text":"Body.","title":"Head","from":"en","to":"de","summary":2}`)
	out := decodeExt(t, body)
	if code != http.StatusOK || out.Text != "[de] Body." || out.Title != "[de] Head" {
		t.Fatalf("status %d: %s", code, body)
	}
	if out.Summary != "Tesla sold a record number of cars." {
		t.Errorf("summary = %q", out.Summary)
	}
	if _, ok := out.Analysis["summary"]; ok {
		t.Error("summary leaked into analysis")
	}
	// The translated title is analyzed together with the text.
	last := f.requests[len(f.requests)-1]
	if !strings.Contains(last, "[de] Head\n\n[de] Body.") || !strings.Contains(last, "at most 2 sentences") {
		t.Errorf("analysis request = %q", last)
	}
	if out.Usage.Provider != "openai" || out.Usage.Model != "gpt-test" || out.Usage.Cost != nil {
		t.Errorf("usage = %+v", out.Usage)
	}
}

func TestAPISummaryFromConfig(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.Summary = 3
	_, body := postTranslate(t, cfg, "", articleBody("en", "de"))
	if out := decodeExt(t, body); out.Summary == "" {
		t.Errorf("no summary: %s", body)
	}
}

func TestAPIAnalyzeOnly(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "vllm", "translategemma-12b")

	code, body := postTranslate(t, cfg, "", `{"text":"Already German.","title":"Titel","to":"de","translate":false}`)
	out := decodeExt(t, body)
	if code != http.StatusOK || out.Text != "Already German." || out.Title != "Titel" {
		t.Fatalf("status %d: %s", code, body)
	}
	if len(f.requests) != 1 || !strings.Contains(f.requests[0], "Analyze the following text") {
		t.Fatalf("requests = %q", f.requests)
	}
	// Source language is not given, so it is detected.
	if out.DetectedLanguage != "en" {
		t.Errorf("detected_language = %q", out.DetectedLanguage)
	}
	if out.Analysis["sentiment"] != "positive" {
		t.Errorf("analysis = %v", out.Analysis)
	}
}

func TestAPISkipSameLanguage(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.SkipSameLanguage = true
	_, body := postTranslate(t, cfg, "", `{"text":"Hallo","from":"de","to":"DE"}`)
	if out := decodeExt(t, body); out.Text != "Hallo" || len(f.requests) != 1 {
		t.Errorf("text %q, %d requests", out.Text, len(f.requests))
	}
	// Without the option the text is translated as before.
	cfg.Settings.SkipSameLanguage = false
	_, body = postTranslate(t, cfg, "", `{"text":"Hallo","from":"de","to":"de"}`)
	if out := decodeExt(t, body); out.Text != "[de] Hallo" {
		t.Errorf("text %q", out.Text)
	}
}

func TestAPIRequestAnalyses(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")

	_, body := postTranslate(t, cfg, "", `{"text":"x","from":"en","to":"de","analysis":["sentiment","tags"],"tags":2}`)
	out := decodeExt(t, body)
	keys := []string{}
	for k := range out.Analysis {
		keys = append(keys, k)
	}
	if len(out.Analysis) != 3 || out.Analysis["sentiment"] != "positive" || len(out.Analysis["tags"].([]any)) != 2 {
		t.Errorf("analysis keys = %v", keys)
	}

	// An empty list disables analysis.
	f.requests = nil
	_, body = postTranslate(t, cfg, "", `{"text":"x","from":"en","to":"de","analysis":[]}`)
	if out := decodeExt(t, body); out.Analysis != nil || len(f.requests) != 1 {
		t.Errorf("analysis = %v, %d requests", out.Analysis, len(f.requests))
	}

	code, body := postTranslate(t, cfg, "", `{"text":"x","analysis":["horoscope"]}`)
	if out := decodeExt(t, body); code != http.StatusBadRequest || out.Code != "invalid_request" {
		t.Errorf("status %d: %s", code, body)
	}
}

func TestAPIAnalysisErrors(t *testing.T) {
	f := newFakeLLM(t)
	f.analysis = "SENTIMENT: positive (0.5)\nTOPICS: technology"
	cfg := testConfig(t, f, "openai", "gpt-test")

	_, body := postTranslate(t, cfg, "", `{"text":"x","from":"en","to":"de","analysis":["sentiment","classify","entities"]}`)
	out := decodeExt(t, body)
	if !reflect.DeepEqual(out.AnalysisErrors, []string{"entities"}) {
		t.Errorf("analysis_errors = %v", out.AnalysisErrors)
	}
}

func TestAPIStrictAnalysis(t *testing.T) {
	f := newFakeLLM(t)
	f.analysis = "SENTIMENT: positive (0.5)\nFACTUALITY: maybe (0.4)\nTOPICS: technology, sports\nEMOTIONS: hope:0.5, joy:0.3"
	cfg := testConfig(t, f, "openai", "gpt-test")
	body := `{"text":"x","from":"en","to":"de","analysis":["sentiment","factuality","classify","emotions"]}`

	// Off by default: values are returned as parsed.
	_, data := postTranslate(t, cfg, "", body)
	if a := decodeExt(t, data).Analysis; a["factuality"] != "maybe" || len(a["topics"].([]any)) != 2 {
		t.Errorf("analysis = %v", a)
	}

	cfg.Settings.StrictAnalysis = true
	_, data = postTranslate(t, cfg, "", body)
	a := decodeExt(t, data).Analysis
	if _, ok := a["factuality"]; ok {
		t.Errorf("invalid factuality kept: %v", a)
	}
	if _, ok := a["factuality_confidence"]; ok {
		t.Errorf("confidence of invalid factuality kept: %v", a)
	}
	if !reflect.DeepEqual(a["topics"], []any{"technology"}) || a["sentiment"] != "positive" || a["sentiment_score"] != 0.5 {
		t.Errorf("analysis = %v", a)
	}
	if !reflect.DeepEqual(a["emotions"], map[string]any{"hope": 0.5}) {
		t.Errorf("emotions = %v", a["emotions"])
	}
}

func TestAPICost(t *testing.T) {
	f := newFakeLLM(t)
	f.cost = 0.001
	cfg := testConfig(t, f, "openrouter", "some/model")
	cfg.Settings = configWithoutAnalyses(cfg.Settings)
	_, body := postTranslate(t, cfg, "", `{"text":"x","from":"en","to":"de"}`)
	out := decodeExt(t, body)
	if out.Usage.Cost == nil || *out.Usage.Cost != 0.001 || out.Usage.Provider != "openrouter" {
		t.Errorf("usage = %s", body)
	}
}

func TestAPIRateLimited(t *testing.T) {
	f := newFakeLLM(t)
	f.failStatus, f.failMsg, f.retryAfter = http.StatusTooManyRequests, "rate limit exceeded", "30"
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.RetryCount, cfg.Settings.RetryDelay = 1, 0

	start := time.Now()
	code, body := postTranslate(t, cfg, "", `{"text":"x","from":"en","to":"de"}`)
	out := decodeExt(t, body)
	if code != http.StatusTooManyRequests || out.Code != "rate_limited" || out.RetryAfter != 30 || out.Error == "" {
		t.Errorf("status %d: %s", code, body)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("too slow")
	}
}

func TestAPIErrorCodes(t *testing.T) {
	f := newFakeLLM(t)
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.MaxTextLength = 5

	code, body := postTranslate(t, cfg, "", `{"text":"привет мир","to":"de"}`)
	if out := decodeExt(t, body); code != http.StatusBadRequest || out.Code != "invalid_request" || !strings.Contains(out.Error, "too long") {
		t.Errorf("status %d: %s", code, body)
	}
	code, body = postTranslate(t, cfg, "", `{"text":"приве","to":"de"}`)
	if code != http.StatusOK {
		t.Errorf("5 characters must pass: status %d: %s", code, body)
	}
}

func TestAPIRequestTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	f := &fakeLLM{Server: slow}
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings.RequestTimeout = 1

	code, body := postTranslate(t, cfg, "", `{"text":"x","from":"en","to":"de"}`)
	if out := decodeExt(t, body); code != http.StatusGatewayTimeout || out.Code != "timeout" {
		t.Errorf("status %d: %s", code, body)
	}
}

func TestAPIMaxConcurrent(t *testing.T) {
	var mu chan struct{} = make(chan struct{}, 10)
	active, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu <- struct{}{}
		active++
		if active > peak {
			peak = active
		}
		<-mu
		time.Sleep(50 * time.Millisecond)
		mu <- struct{}{}
		active--
		<-mu
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}}})
	}))
	defer srv.Close()
	cfg := testConfig(t, &fakeLLM{Server: srv}, "openai", "gpt-test")
	cfg.Settings = configWithoutAnalyses(cfg.Settings)
	cfg.Settings.MaxConcurrent = 1
	resetFlags()
	h := newServerHandler(cfg, nil)

	done := make(chan int, 3)
	for i := 0; i < 3; i++ {
		go func() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/translate", strings.NewReader(`{"text":"x","from":"en"}`)))
			done <- rec.Code
		}()
	}
	for i := 0; i < 3; i++ {
		if c := <-done; c != http.StatusOK {
			t.Errorf("status %d", c)
		}
	}
	if peak != 1 {
		t.Errorf("peak concurrency %d, want 1", peak)
	}
}

func TestHealthVersion(t *testing.T) {
	old := Version
	Version = "1.2.3"
	defer func() { Version = old }()
	rec := httptest.NewRecorder()
	newServerHandler(testConfig(t, newFakeLLM(t), "openai", "m"), nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["status"] != "ok" || out["version"] != "1.2.3" {
		t.Errorf("health = %v", out)
	}
}
