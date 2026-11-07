package cli

import "strings"

// Allowed values of enum fields, as requested by the analysis prompts.
var (
	scalarEnums = map[string][]string{
		"sentiment":      {"positive", "negative", "neutral"},
		"factuality":     {"confirmed", "rumors", "forecasts", "unsourced"},
		"sensationalism": {"neutral", "emotional", "clickbait", "manipulative"},
		"time_focus":     {"past", "present", "future", "mixed"},
		"ad_type":        {"none", "direct", "native", "sponsored", "pr"},
	}
	// confidenceOf maps an enum field to the confidence field that belongs to it.
	confidenceOf = map[string]string{
		"sentiment":      "sentiment_score",
		"factuality":     "factuality_confidence",
		"sensationalism": "sensationalism_confidence",
		"time_focus":     "time_focus_confidence",
		"ad_type":        "ad_confidence",
	}
	listEnums = map[string][]string{
		"topics":              {"politics", "economics", "technology", "medicine", "incidents"},
		"scope":               {"regional", "international"},
		"news_type":           {"corporate", "regulatory", "macro"},
		"affected":            {"individuals", "business", "government", "investors", "consumers"},
		"factuality_evidence": {"official_source", "statistics", "quotes", "documents", "expert_opinion", "anonymous_source", "speculation", "prediction"},
	}
	emotionNames = []string{"fear", "anger", "hope", "uncertainty", "optimism", "panic"}
)

// sanitizeAnalysis drops enum values outside the allowed sets: a scalar
// field is removed together with its confidence, list items are filtered and
// emotion scores must be known and within 0..1. Valid values are kept as is.
func sanitizeAnalysis(m map[string]interface{}) map[string]interface{} {
	for key, allowed := range scalarEnums {
		v, ok := m[key]
		if !ok {
			continue
		}
		if s, _ := v.(string); !contains(allowed, s) {
			delete(m, key)
			delete(m, confidenceOf[key])
		}
	}
	for key, allowed := range listEnums {
		items, ok := m[key].([]string)
		if !ok {
			continue
		}
		var kept []string
		for _, it := range items {
			if contains(allowed, it) {
				kept = append(kept, it)
			}
		}
		if len(kept) == 0 {
			delete(m, key)
		} else if len(kept) != len(items) {
			m[key] = kept
		}
	}
	if emotions, ok := m["emotions"].(map[string]float64); ok {
		for name, score := range emotions {
			if !contains(emotionNames, name) || score < 0 || score > 1 {
				delete(emotions, name)
			}
		}
		if len(emotions) == 0 {
			delete(m, "emotions")
		}
	}
	return m
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == strings.TrimSpace(s) {
			return true
		}
	}
	return false
}
