package cli

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

// fakeAnalysis is a combined analysis answer covering every section.
const fakeAnalysis = `SENTIMENT: positive (0.6)
TAGS: tesla, electric cars, sales, market, elon musk, extra
TOPICS: technology, economics
SCOPE: international
TYPE: corporate
EMOTIONS: hope:0.6, optimism:0.5
FACTUALITY: confirmed (0.9)
EVIDENCE: statistics, official_source
AFFECTED: business, investors
SENSATIONALISM: neutral (0.8)
MARKERS: superlatives
USEFULNESS: useful (0.9)
REASONS: facts, numbers
ENTITY_PERSONS: Elon Musk
ENTITY_ORGANIZATIONS: Tesla
ENTITY_LOCATIONS: USA
ENTITY_DATES: 2025
ENTITY_AMOUNTS: $1 billion
EVENTS: Tesla reported record sales; Shares rose
TIME_FOCUS: past (0.8)
IS_PREDICTION: false
INDICATORS: last quarter
AD_TYPE: none (0.95)
AD_MARKERS: none`

// fakeLLM is an OpenAI-compatible server. Translations are returned as
// "[<target>] <text>", analysis prompts get fakeAnalysis. Every response
// reports 10 prompt and 5 completion tokens.
type fakeLLM struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string // prompts seen, for assertions
	// fail, if set, makes every request return this status and error message.
	failStatus int
	failMsg    string
}

func newFakeLLM(t *testing.T) *fakeLLM {
	f := &fakeLLM{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeLLM) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	if f.failStatus != 0 {
		w.WriteHeader(f.failStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": f.failMsg}})
		return
	}

	var system, user, target string
	if p, ok := body["prompt"].(string); ok { // /completions
		system = p
	}
	msgs, _ := body["messages"].([]any)
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		switch c := mm["content"].(type) {
		case string:
			if mm["role"] == "system" {
				system = c
			} else {
				user = c
			}
		case []any: // TranslateGemma content parts
			part, _ := c[0].(map[string]any)
			user, _ = part["text"].(string)
			target, _ = part["target_lang_code"].(string)
		}
	}
	f.mu.Lock()
	f.requests = append(f.requests, system+"\n"+user)
	f.mu.Unlock()

	var out string
	switch {
	case strings.Contains(system, "Analyze the following text"):
		out = fakeAnalysis
	case target != "":
		out = "[" + target + "] " + user
	default:
		// "... from X to Y. ..." or "... translate the text to Y. ..."
		i := strings.Index(system, " to ")
		target = strings.SplitN(system[i+4:], ".", 2)[0]
		out = "[" + target + "] " + user
	}

	usage := map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	resp := map[string]any{"usage": usage}
	if r.URL.Path == "/completions" {
		resp["choices"] = []any{map[string]any{"text": out}}
	} else {
		resp["choices"] = []any{map[string]any{"message": map[string]any{"role": "assistant", "content": out}}}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// testConfig loads testdata/newsportal.yaml (the NewsPortal client config)
// and points providerName at the fake server.
func testConfig(t *testing.T, f *fakeLLM, providerName, model string) *config.Config {
	t.Helper()
	cfg, err := config.Load("testdata/newsportal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultProvider = providerName
	cfg.Proxy = config.ProxyConfig{}
	cfg.Providers[providerName] = config.ProviderConfig{APIKey: "k", BaseURL: f.URL, Model: model}
	return cfg
}

// checkGolden compares got (JSON-decodable value) with testdata/golden/<name>.json.
// Keys present in the golden file must match exactly; new keys are allowed so
// that additive API changes do not break the check.
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		var v any
		if err := json.Unmarshal(got, &v); err != nil {
			t.Fatal(err)
		}
		data, _ := json.MarshalIndent(v, "", "  ")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var w, g any
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, got)
	}
	if path, ok := subset(w, g, "$"); !ok {
		t.Fatalf("mismatch at %s\nwant: %s\ngot:  %s", path, want, got)
	}
}

// subset reports whether every key of want exists in got with an equal value.
// Arrays and scalars must be equal.
func subset(want, got any, path string) (string, bool) {
	wm, ok := want.(map[string]any)
	if !ok {
		return path, reflect.DeepEqual(want, got)
	}
	gm, ok := got.(map[string]any)
	if !ok {
		return path, false
	}
	for k, wv := range wm {
		gv, ok := gm[k]
		if !ok {
			return path + "." + k, false
		}
		if p, ok := subset(wv, gv, path+"."+k); !ok {
			return p, false
		}
	}
	return "", true
}
