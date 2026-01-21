package validator

import (
	"regexp"
	"strings"

	"github.com/foxzi/llm-translate/internal/config"
)

type Validator struct {
	config config.StrongValidation
}

func New(cfg config.StrongValidation) *Validator {
	return &Validator{
		config: cfg,
	}
}

func (v *Validator) Validate(text, sourceLang, targetLang string) (bool, []string) {
	if !v.config.Enabled {
		return true, nil
	}
	
	cleanedText := v.cleanTextForValidation(text)
	
	if len(strings.TrimSpace(cleanedText)) < 10 {
		return true, nil
	}
	
	// Simplified validation - just check for common English words if source is English
	if sourceLang == "en" && v.containsEnglishText(cleanedText) {
		fragments := v.extractProblematicFragments(text, cleanedText)
		return false, fragments
	}
	
	return true, nil
}

func (v *Validator) cleanTextForValidation(text string) string {
	result := text
	
	for _, pattern := range v.config.AllowedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		result = re.ReplaceAllString(result, " ")
	}
	
	for _, term := range v.config.AllowedTerms {
		result = strings.ReplaceAll(result, term, " ")
	}
	
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)
	
	return result
}

func (v *Validator) containsEnglishText(text string) bool {
	// Simple heuristic - check for common English words
	commonWords := []string{
		"the", "be", "to", "of", "and", "a", "in", "that", "have", "I",
		"it", "for", "not", "on", "with", "he", "as", "you", "do", "at",
		"this", "but", "his", "by", "from", "they", "we", "say", "her", "she",
		"or", "an", "will", "my", "one", "all", "would", "there", "their",
		"what", "so", "up", "out", "if", "about", "who", "get", "which", "go",
	}
	
	textLower := strings.ToLower(text)
	count := 0
	
	for _, word := range commonWords {
		if strings.Contains(textLower, " "+word+" ") || 
		   strings.HasPrefix(textLower, word+" ") || 
		   strings.HasSuffix(textLower, " "+word) {
			count++
		}
		if count > 3 {
			return true
		}
	}
	
	return false
}

func (v *Validator) extractProblematicFragments(originalText, cleanedText string) []string {
	var fragments []string
	
	words := strings.Fields(cleanedText)
	for _, word := range words {
		if len(word) > 3 && !v.isAllowedWord(word) {
			if v.isEnglishWord(word) {
				fragments = append(fragments, word)
			}
		}
	}
	
	if len(fragments) > 5 {
		return fragments[:5]
	}
	
	return fragments
}

func (v *Validator) isAllowedWord(word string) bool {
	for _, term := range v.config.AllowedTerms {
		if strings.EqualFold(word, term) {
			return true
		}
	}
	
	if regexp.MustCompile(`^\d+$`).MatchString(word) {
		return true
	}
	
	if regexp.MustCompile(`^[A-Z]{2,}$`).MatchString(word) {
		return true
	}
	
	return false
}

func (v *Validator) isEnglishWord(word string) bool {
	// Simple check for common English words
	commonWords := map[string]bool{
		"the": true, "be": true, "to": true, "of": true, "and": true,
		"a": true, "in": true, "that": true, "have": true, "it": true,
		"for": true, "not": true, "on": true, "with": true, "as": true,
		"you": true, "do": true, "at": true, "this": true, "but": true,
		"his": true, "by": true, "from": true, "they": true, "we": true,
		"say": true, "her": true, "she": true, "or": true, "an": true,
		"will": true, "my": true, "one": true, "all": true, "would": true,
		"there": true, "their": true, "what": true, "so": true, "up": true,
		"out": true, "if": true, "about": true, "who": true, "get": true,
		"which": true, "go": true, "me": true, "when": true, "make": true,
		"can": true, "like": true, "time": true, "no": true, "just": true,
		"him": true, "know": true, "take": true, "people": true, "into": true,
		"year": true, "your": true, "good": true, "some": true, "could": true,
		"them": true, "see": true, "other": true, "than": true, "then": true,
		"now": true, "look": true, "only": true, "come": true, "its": true,
		"over": true, "think": true, "also": true, "back": true, "after": true,
		"use": true, "two": true, "how": true, "our": true, "work": true,
	}
	
	return commonWords[strings.ToLower(word)]
}