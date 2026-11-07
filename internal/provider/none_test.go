package provider

import (
	"strings"
	"testing"
)

func TestParseNoneIsEmptyResult(t *testing.T) {
	if r, err := ParseEventsResponse("EVENTS: none"); err != nil || len(r.Events) != 0 {
		t.Errorf("events: %+v, %v", r, err)
	}
	if r, err := ParseEventsResponse("EVENTS:"); err != nil || len(r.Events) != 0 {
		t.Errorf("empty events: %+v, %v", r, err)
	}
	all := "PERSONS: none\nORGANIZATIONS: none\nLOCATIONS: none\nDATES: none\nAMOUNTS: none"
	if r, err := ParseEntitiesResponse(all); err != nil || len(r.Persons)+len(r.Amounts) != 0 {
		t.Errorf("entities: %+v, %v", r, err)
	}
	if r, err := ParseClassifyResponse("TOPICS: none\nSCOPE: none\nTYPE: none"); err != nil || len(r.Topics)+len(r.Scope)+len(r.NewsType) != 0 {
		t.Errorf("classify: %+v, %v", r, err)
	}
	for _, in := range []string{"EMOTIONS: none", "EMOTIONS: None", "EMOTIONS:"} {
		if r, err := ParseEmotionsResponse(in); err != nil || len(r.Emotions) != 0 {
			t.Errorf("emotions %q: %+v, %v", in, r, err)
		}
	}
}

func TestParseMissingLabelIsError(t *testing.T) {
	const text = "I could not analyze this."
	if _, err := ParseEventsResponse(text); err == nil {
		t.Error("events: no error")
	}
	if _, err := ParseEntitiesResponse(text); err == nil {
		t.Error("entities: no error")
	}
	if _, err := ParseClassifyResponse(text); err == nil {
		t.Error("classify: no error")
	}
	if _, err := ParseEmotionsResponse(text); err == nil {
		t.Error("emotions: no label: no error")
	}
	if _, err := ParseEmotionsResponse("EMOTIONS: very afraid"); err == nil {
		t.Error("emotions: garbage: no error")
	}
}

func TestCombinedNoneIsNotFailure(t *testing.T) {
	req := CombinedAnalysisRequest{Classify: true, Emotions: true, Entities: true, Events: true}
	resp := ParseCombinedResponse("TOPICS: none\nSCOPE: none\nTYPE: none\nEMOTIONS: none\nPERSONS: none\nEVENTS: none", req)
	if len(resp.Failed) != 0 || resp.Classify == nil || resp.Emotions == nil || resp.Entities == nil || resp.Events == nil {
		t.Fatalf("failed = %v, resp = %+v", resp.Failed, resp)
	}
}

func TestEmotionsPromptsMentionNone(t *testing.T) {
	if !strings.Contains(EmotionsPrompt, "EMOTIONS: none") {
		t.Error("EmotionsPrompt has no none instruction")
	}
	if p := BuildCombinedPrompt(CombinedAnalysisRequest{Emotions: true}); !strings.Contains(p, "'none'") {
		t.Error("combined emotions section has no none instruction")
	}
}
