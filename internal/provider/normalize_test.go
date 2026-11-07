package provider

import (
	"reflect"
	"testing"
)

func TestNormalizedResponses(t *testing.T) {
	sent := func(in string) any {
		r, err := ParseSentimentResponse(in)
		return []any{r.Sentiment, r.Score, err}
	}
	tags := func(in string) any {
		r, err := ParseTagsResponse(in)
		return []any{r.Tags, err}
	}
	emo := func(in string) any {
		r, err := ParseEmotionsResponse(in)
		return []any{r.Emotions, err}
	}
	comb := func(in string) any {
		r := ParseCombinedResponse(in, CombinedAnalysisRequest{Sentiment: true, TagsCount: 5, Emotions: true})
		return []any{r.Sentiment.Sentiment, r.Sentiment.Score, r.Tags.Tags, r.Emotions.Emotions, r.Failed}
	}
	wantSent := []any{"positive", 0.7, error(nil)}
	wantTags := []any{[]string{"a", "b c"}, error(nil)}
	wantEmo := []any{map[string]float64{"fear": 0.8, "hope": 0.3}, error(nil)}

	tests := []struct {
		name string
		fn   func(string) any
		in   string
		want any
	}{
		{"sentiment plain", sent, "SENTIMENT: positive (0.7)", wantSent},
		{"sentiment fence", sent, "```\nSENTIMENT: positive (0.7)\n```", wantSent},
		{"sentiment fence lang", sent, "```text\nSENTIMENT: positive (0.7)\n```", wantSent},
		{"sentiment inline fence", sent, "```SENTIMENT: positive (0.7)```", wantSent},
		{"sentiment bullet", sent, "- SENTIMENT: positive (0.7)", wantSent},
		{"sentiment star bullet", sent, "* SENTIMENT: positive (0.7)", wantSent},
		{"sentiment numbered", sent, "1. SENTIMENT: positive (0.7)", wantSent},
		{"sentiment bold colon inside", sent, "**SENTIMENT:** positive (0.7)", wantSent},
		{"sentiment bold colon outside", sent, "**SENTIMENT**: positive (0.7)", wantSent},
		{"sentiment italic", sent, "*SENTIMENT*: positive (0.7)", wantSent},
		{"sentiment bold line", sent, "**SENTIMENT: positive (0.7)**", wantSent},
		{"sentiment lowercase", sent, "sentiment: Positive (0.7)", wantSent},
		{"sentiment decimal comma", sent, "SENTIMENT: positive (0,7)", wantSent},
		{"sentiment unicode minus", sent, "SENTIMENT: negative (−0.7)", []any{"negative", -0.7, error(nil)}},
		{"sentiment unicode minus comma", sent, "SENTIMENT: negative (−0,7)", []any{"negative", -0.7, error(nil)}},
		{"sentiment bullet bold fence", sent, "```\n- **SENTIMENT:** positive (0,7)\n```", wantSent},
		{"tags plain", tags, "TAGS: a, b c", wantTags},
		{"tags bold", tags, "**TAGS:** a, b c", wantTags},
		{"tags bold outside", tags, "**TAGS**: a, b c", wantTags},
		{"tags bullet fence", tags, "```\n- tags: a, b c\n```", wantTags},
		{"tags decimal list kept", tags, "TAGS: 0,5, 1,2", []any{[]string{"0", "5", "1", "2"}, error(nil)}},
		{"emotions plain", emo, "EMOTIONS: fear:0.8, hope:0.3", wantEmo},
		{"emotions spaces", emo, "EMOTIONS: fear: 0.8, hope : 0.3", wantEmo},
		{"emotions decimal comma", emo, "EMOTIONS: fear:0,8, hope:0,3", wantEmo},
		{"emotions decimal comma spaces", emo, "EMOTIONS: fear: 0,8, hope: 0,3", wantEmo},
		{"emotions bold bullet", emo, "- **EMOTIONS:** fear:0.8, hope:0.3", wantEmo},
		{"emotions lowercase", emo, "emotions: fear:0.8, hope:0.3", wantEmo},
		{"combined", comb, "```\n- **SENTIMENT:** positive (0,7)\n- **TAGS**: a, b c\n* EMOTIONS: fear: 0,8, hope: 0,3\n```",
			[]any{"positive", 0.7, []string{"a", "b c"}, map[string]float64{"fear": 0.8, "hope": 0.3}, []string(nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeKeepsNumbersOutsideScores(t *testing.T) {
	in := "AMOUNTS: 0,5 million, 1,2 billion\nDATES: 12:30,15"
	if got := normalizeResponse(in); got != in {
		t.Errorf("got %q", got)
	}
}
