package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/foxzi/llm-translate/internal/translator"
)

func TestRunAnalysisNoneAddsNoKeys(t *testing.T) {
	f := newFakeLLM(t)
	f.analysis = "SENTIMENT: neutral (0.0)\nTOPICS: none\nSCOPE: none\nTYPE: none\nEMOTIONS: none\nPERSONS: none\nEVENTS: none"
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings = configWithoutAnalyses(cfg.Settings)
	cfg.Settings.Sentiment, cfg.Settings.Classify, cfg.Settings.Emotions = true, true, true
	cfg.Settings.Entities, cfg.Settings.Events = true, true

	fm, failed := runAnalysis(context.Background(), translator.New(cfg, false), cfg, "text", "", false, false)
	if len(failed) != 0 {
		t.Errorf("failed = %v", failed)
	}
	for _, k := range []string{"topics", "scope", "news_type", "emotions", "persons", "events"} {
		if _, ok := fm[k]; ok {
			t.Errorf("empty result added key %q", k)
		}
	}
	if fm["sentiment"] != "neutral" {
		t.Errorf("frontmatter = %v", fm)
	}
}

func TestRunAnalysisLangUsesCombinedPrompt(t *testing.T) {
	f := newFakeLLM(t)
	f.analysis = "TAGS: a, b"
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings = configWithoutAnalyses(cfg.Settings)
	cfg.Settings.TagsCount = 2

	fm, failed := runAnalysis(context.Background(), translator.New(cfg, false), cfg, "text", "de", false, false)
	if len(failed) != 0 || !reflect.DeepEqual(fm["tags"], []string{"a", "b"}) {
		t.Fatalf("fm = %v, failed = %v", fm, failed)
	}
	if len(f.requests) != 1 || !strings.Contains(f.requests[0], "=== TAGS ===") || !strings.Contains(f.requests[0], `in language "de"`) {
		t.Errorf("requests = %q", f.requests)
	}

	// Without a language a single analysis stays an individual call.
	f.requests = nil
	runAnalysis(context.Background(), translator.New(cfg, false), cfg, "text", "", false, false)
	if len(f.requests) != 1 || strings.Contains(f.requests[0], "=== TAGS ===") {
		t.Errorf("requests = %q", f.requests)
	}
}

func TestParallelAnalysisSingleAnalysisUsesTargetLanguage(t *testing.T) {
	f := newFakeLLM(t)
	f.analysis = "TAGS: a, b"
	cfg := testConfig(t, f, "openai", "gpt-test")
	cfg.Settings = configWithoutAnalyses(cfg.Settings)
	cfg.Settings.TagsCount, cfg.Settings.ParallelAnalysis = 2, true

	_, fm, err := translateAndAnalyze(context.Background(), translator.New(cfg, false), cfg, translator.TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "de"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fm["tags"], []string{"a", "b"}) {
		t.Errorf("fm = %v", fm)
	}
	found := false
	for _, r := range f.requests {
		found = found || strings.Contains(r, "=== TAGS ===") && strings.Contains(r, `in language "de"`)
	}
	if !found {
		t.Errorf("no combined prompt with target language: %q", f.requests)
	}
}
