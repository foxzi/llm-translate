package provider

import (
	"regexp"
	"strings"
)

var (
	fenceLineRe    = regexp.MustCompile("(?m)^[ \\t]*```[A-Za-z]*[ \\t]*$\\n?")
	bulletRe       = regexp.MustCompile(`(?m)^[ \t]*(?:[-*•]|\d+[.)])[ \t]+`)
	boldLineRe     = regexp.MustCompile(`(?m)^\*{1,3}([A-Za-z_]+:[^\n]*?)\*{1,3}[ \t]*$`)
	boldLabelRe    = regexp.MustCompile(`(?m)^[ \t]*\*{1,3}([A-Za-z_]+)(?:\*{1,3}[ \t]*:|[ \t]*:\*{1,3})[ \t]*`)
	unicodeMinus   = regexp.MustCompile(`\(\s*−`)
	parenCommaRe   = regexp.MustCompile(`\(\s*([+-]?[01]),(\d+)\s*\)`)
	emotionsLine   = regexp.MustCompile(`(?im)^EMOTIONS:.*$`)
	emotionCommaRe = regexp.MustCompile(`([A-Za-z]+[ \t]*:[ \t]*[01]),(\d+)`)
)

// normalizeResponse prepares an analysis answer for the Parse* functions: it
// strips reasoning blocks, code fences, list markers and bold markers around
// labels, and fixes a unicode minus and a decimal comma in scores.
func normalizeResponse(s string) string {
	s = StripReasoning(s)
	s = strings.ReplaceAll(fenceLineRe.ReplaceAllString(s, ""), "```", "")
	s = bulletRe.ReplaceAllString(s, "")
	s = boldLineRe.ReplaceAllString(s, "$1")
	s = boldLabelRe.ReplaceAllString(s, "$1: ")
	s = unicodeMinus.ReplaceAllString(s, "(-")
	s = parenCommaRe.ReplaceAllString(s, "($1.$2)")
	return emotionsLine.ReplaceAllStringFunc(s, func(line string) string {
		return emotionCommaRe.ReplaceAllString(line, "$1.$2")
	})
}
