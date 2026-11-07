package cli

import (
	"reflect"
	"testing"
)

func TestSanitizeAnalysis(t *testing.T) {
	valid := map[string]interface{}{
		"sentiment": "neutral", "sentiment_score": 0.0, "topics": []string{"technology"},
		"emotions": map[string]float64{"fear": 0.2}, "ad_type": "pr", "tags": []string{"anything"},
	}
	want := map[string]interface{}{
		"sentiment": "neutral", "sentiment_score": 0.0, "topics": []string{"technology"},
		"emotions": map[string]float64{"fear": 0.2}, "ad_type": "pr", "tags": []string{"anything"},
	}
	if got := sanitizeAnalysis(valid); !reflect.DeepEqual(got, want) {
		t.Errorf("valid values changed: %v", got)
	}

	bad := map[string]interface{}{
		"time_focus": "someday", "time_focus_confidence": 0.7, "is_prediction": true,
		"scope": []string{"galactic"}, "affected": []string{"business", "aliens"},
		"emotions": map[string]float64{"joy": 0.5, "panic": 3},
	}
	got := sanitizeAnalysis(bad)
	want = map[string]interface{}{"is_prediction": true, "affected": []string{"business"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}
