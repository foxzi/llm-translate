package validator

import (
	"regexp"
	"strings"
	"unicode"

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

	// Check if source language text leaked into translation
	if v.containsSourceLanguage(cleanedText, sourceLang) {
		fragments := v.extractProblematicFragments(text, cleanedText)
		return false, fragments
	}

	return true, nil
}

// containsSourceLanguage checks whether the translated text still contains
// significant portions of the source language. Uses common-word detection
// for Latin-script languages and Unicode script ratio for CJK/Cyrillic.
func (v *Validator) containsSourceLanguage(text, sourceLang string) bool {
	switch strings.ToLower(sourceLang) {
	case "en":
		return v.containsEnglishText(text)
	case "es":
		return v.containsCommonWords(text, spanishCommonWords, 3)
	case "fr":
		return v.containsCommonWords(text, frenchCommonWords, 3)
	case "de":
		return v.containsCommonWords(text, germanCommonWords, 3)
	case "pt":
		return v.containsCommonWords(text, portugueseCommonWords, 3)
	case "it":
		return v.containsCommonWords(text, italianCommonWords, 3)
	case "zh":
		return v.containsScriptRatio(text, unicode.Han, 0.3)
	case "ja":
		return v.containsScriptRatio(text, unicode.Hiragana, 0.1) ||
			v.containsScriptRatio(text, unicode.Katakana, 0.1)
	case "ko":
		return v.containsScriptRatio(text, unicode.Hangul, 0.3)
	case "ru":
		return v.containsScriptRatio(text, unicode.Cyrillic, 0.3)
	case "ar":
		return v.containsScriptRatio(text, unicode.Arabic, 0.3)
	default:
		return false
	}
}

// containsCommonWords checks if text contains more than threshold common words
// from the given language.
func (v *Validator) containsCommonWords(text string, words []string, threshold int) bool {
	textLower := strings.ToLower(text)
	count := 0
	for _, word := range words {
		if strings.Contains(textLower, " "+word+" ") ||
			strings.HasPrefix(textLower, word+" ") ||
			strings.HasSuffix(textLower, " "+word) {
			count++
		}
		if count > threshold {
			return true
		}
	}
	return false
}

// containsScriptRatio checks if more than the given ratio of letter characters
// in the text belong to the specified Unicode script/range table.
func (v *Validator) containsScriptRatio(text string, script *unicode.RangeTable, threshold float64) bool {
	var total, matched int
	for _, r := range text {
		if unicode.IsLetter(r) {
			total++
			if unicode.Is(script, r) {
				matched++
			}
		}
	}
	if total == 0 {
		return false
	}
	return float64(matched)/float64(total) > threshold
}

// Common words for supported source languages (used for same-script detection).
var spanishCommonWords = []string{
	"el", "la", "los", "las", "de", "del", "en", "que", "por", "con",
	"para", "una", "como", "pero", "sus", "este", "esta", "entre",
	"cuando", "sobre", "desde", "tiene", "puede", "todos", "durante",
}

var frenchCommonWords = []string{
	"le", "la", "les", "des", "dans", "pour", "avec", "sur", "que",
	"une", "est", "pas", "sont", "mais", "cette", "qui", "par",
	"aux", "peut", "entre", "comme", "tout", "depuis", "aussi",
}

var germanCommonWords = []string{
	"der", "die", "das", "und", "ist", "von", "mit", "auf", "den",
	"eine", "nicht", "sich", "auch", "aus", "noch", "nach", "wie",
	"wird", "bei", "oder", "nur", "hat", "aber", "seine", "kann",
}

var portugueseCommonWords = []string{
	"que", "para", "com", "uma", "por", "dos", "das", "mais",
	"como", "foi", "mas", "sua", "pelo", "esta", "pode", "entre",
	"quando", "sobre", "desde", "seus", "tem", "todos", "ainda",
}

var italianCommonWords = []string{
	"che", "per", "con", "una", "sono", "della", "delle", "degli",
	"nella", "come", "questo", "questa", "anche", "stato", "essere",
	"hanno", "suo", "suoi", "dalla", "alle", "ogni", "dopo", "molto",
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
