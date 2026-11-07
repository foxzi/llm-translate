package cli

import (
	"context"
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
