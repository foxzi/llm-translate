package validator

import (
	"testing"

	"github.com/foxzi/llm-translate/internal/config"
)

func newEnabled() *Validator {
	cfg := config.DefaultConfig().StrongValidation
	cfg.Enabled = true
	return New(cfg)
}

func TestValidateDisabled(t *testing.T) {
	cfg := config.DefaultConfig().StrongValidation
	cfg.Enabled = false
	ok, frags := New(cfg).Validate("the quick fox and the dog that will see this one with all of it", "en", "ru")
	if !ok || frags != nil {
		t.Errorf("disabled validator = %v, %v", ok, frags)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name, text, from string
		ok               bool
	}{
		{"english left in text", "the quick fox and the dog that will see this one with all of it", "en", false},
		{"translated to russian", "Быстрая лиса и собака, которые увидят это всё вместе", "en", true},
		{"short text is skipped", "the and of", "en", true},
		{"cyrillic left in text", "Это предложение осталось без перевода полностью", "ru", false},
		{"latin text for cyrillic source", "Dies ist ein vollständig übersetzter Satz hier", "ru", true},
		{"unsupported source language", "anything goes here in the translated text", "xx", true},
		{"only allowed terms", "API HTTP JSON XML SQL REST GraphQL OAuth", "en", true},
		{"url and code are ignored", "Смотрите https://example.com/the/and/of и `the and of to in`", "en", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, _ := newEnabled().Validate(tc.text, tc.from, "xx")
			if ok != tc.ok {
				t.Errorf("Validate(%q, %q) ok = %v, want %v", tc.text, tc.from, ok, tc.ok)
			}
		})
	}
}

func TestValidateReportsFragments(t *testing.T) {
	ok, frags := newEnabled().Validate("the quick fox and the dog that will see this one with all of it", "en", "ru")
	if ok || len(frags) == 0 || len(frags) > 5 {
		t.Fatalf("ok = %v, fragments = %v", ok, frags)
	}
}

func TestAllowedTermsAreNotFragments(t *testing.T) {
	cfg := config.StrongValidation{Enabled: true, AllowedTerms: []string{"with"}}
	_, frags := New(cfg).Validate("the quick fox and the dog that will see this one with all of it", "en", "ru")
	for _, f := range frags {
		if f == "with" {
			t.Errorf("allowed term reported: %v", frags)
		}
	}
}
