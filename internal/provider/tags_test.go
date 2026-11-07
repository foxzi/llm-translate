package provider

import (
	"strings"
	"testing"
)

func TestParseCombinedResponseLimitsTags(t *testing.T) {
	resp := ParseCombinedResponse("TAGS: a, b, c, d, e, f, g", CombinedAnalysisRequest{TagsCount: 5})
	if resp.Tags == nil || len(resp.Tags.Tags) != 5 || resp.Tags.Tags[4] != "e" {
		t.Fatalf("expected first 5 tags, got %+v", resp.Tags)
	}
}

func TestLimitTagsKeepsShorterList(t *testing.T) {
	if got := LimitTags([]string{"a", "b"}, 5); len(got) != 2 {
		t.Fatalf("expected 2 tags, got %v", got)
	}
}

func TestCombinedSummaryAndFailures(t *testing.T) {
	req := CombinedAnalysisRequest{Sentiment: true, Entities: true, Events: true, AdDetect: true, Summary: 2}
	resp := ParseCombinedResponse("SENTIMENT: maybe\nENTITY_PERSONS: none\nEVENTS: none\nSUMMARY:  Short text. Second.\n", req)
	if resp.Summary != "Short text. Second." {
		t.Errorf("summary = %q", resp.Summary)
	}
	// Empty entities and events are valid answers; bad sentiment and a missing ad section are not.
	if got := strings.Join(resp.Failed, ","); got != "sentiment,ad_detect" {
		t.Errorf("failed = %q", got)
	}
	if p := BuildCombinedPrompt(CombinedAnalysisRequest{}); strings.Contains(p, "SUMMARY") {
		t.Error("summary section without request")
	}
	if p := BuildCombinedPrompt(req); !strings.Contains(p, "at most 2 sentences") {
		t.Error("summary section missing")
	}
}

func TestCombinedLanguage(t *testing.T) {
	req := CombinedAnalysisRequest{Sentiment: true, DetectLanguage: true}
	if !strings.Contains(BuildCombinedPrompt(req), "=== LANGUAGE ===") {
		t.Error("language section missing")
	}
	if strings.Contains(BuildCombinedPrompt(CombinedAnalysisRequest{Sentiment: true}), "LANGUAGE:") {
		t.Error("language section without request")
	}
	resp := ParseCombinedResponse("SENTIMENT: neutral (0.0)\nLANGUAGE: DE\n", req)
	if resp.Language != "de" || len(resp.Failed) != 0 {
		t.Errorf("language = %q, failed = %v", resp.Language, resp.Failed)
	}
}
