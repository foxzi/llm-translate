package provider

import "testing"

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
