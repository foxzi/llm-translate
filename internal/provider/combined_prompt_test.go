package provider

import (
	"strings"
	"testing"
)

func TestCombinedPromptRules(t *testing.T) {
	all := CombinedAnalysisRequest{
		Sentiment: true, TagsCount: 5, Classify: true, Emotions: true, Impact: true,
		Sensationalism: true, AdDetect: true, Summary: 2,
	}
	p := BuildCombinedPrompt(all)
	for _, want := range []string{
		"Analyze the following text",
		"Answer only with the labeled lines, one per line: no markdown, no explanations, no code fences.",
		"Score must be consistent with label",
		"Rules: use only the exact values listed above. Use 'none' for a category that does not apply.",
		"Rules: use only the exact values listed above. Use 'none' if no one applies.",
		"EMOTIONS: <comma-separated name:score pairs, score 0.0-1.0>",
		"Allowed emotions: fear, anger, hope, uncertainty, optimism, panic.",
		"MARKERS: <comma-separated, from: " + sensationalismMarkers + ">",
		"AD_MARKERS: <comma-separated, from: " + adMarkers + ">",
		"same language as the text.",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "fear:X.X") {
		t.Error("emotions template still lists all emotions")
	}
}

func TestCombinedPromptLanguage(t *testing.T) {
	req := CombinedAnalysisRequest{Sentiment: true, Sensationalism: true, Lang: "de"}
	p := BuildCombinedPrompt(req)
	if !strings.Contains(p, `(tags, entities, events, indicators, summary) in language "de".`) || strings.Contains(p, "markers, summary") {
		t.Errorf("target language sentence wrong:\n%s", p)
	}
	if strings.Contains(p, "same language as the text.") {
		t.Error("source language sentence with a target language")
	}
	req.Lang = ""
	if p := BuildCombinedPrompt(req); !strings.Contains(p, "Write all free-text values (tags, entities, events, indicators, summary) in the same language as the text.") {
		t.Error("source language sentence missing")
	}
}

func TestMarkerVocabulariesMatchIndividualPrompts(t *testing.T) {
	if !strings.Contains(SensationalismPrompt, "- "+sensationalismMarkers+"\n") {
		t.Error("sensationalism markers differ from SensationalismPrompt")
	}
	if !strings.Contains(AdDetectPrompt, "- "+adMarkers+"\n") {
		t.Error("ad markers differ from AdDetectPrompt")
	}
}
