package cli

import (
	"testing"
)

func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name, in, front, body string
	}{
		{"plain", "---\ntitle: A\n---\nBody\n", "---\ntitle: A\n---\n", "Body\n"},
		{"no body", "---\ntitle: A\n---", "---\ntitle: A\n---", ""},
		{"crlf and trailing spaces", "---\r\ntitle: A\r\n---  \r\nBody", "---\r\ntitle: A\r\n---  \r\n", "Body"},
		{"empty frontmatter", "---\n---\nBody", "---\n---\n", "Body"},
		{"horizontal rule only", "---\n\nText\n\n---\n\nMore", "---\n\nText\n\n---\n", "\nMore"},
		{"closing must be a whole line", "---\ntitle: A\n----\nBody\n--- x\n", "", "---\ntitle: A\n----\nBody\n--- x\n"},
		{"dashes in value", "---\ntitle: a\n---b\n---\nBody", "---\ntitle: a\n---b\n---\n", "Body"},
		{"opening must be exact", "----\ntitle: A\n---\nBody", "", "----\ntitle: A\n---\nBody"},
		{"opening needs newline", "--- title\n---\nBody", "", "--- title\n---\nBody"},
		{"unclosed", "---\ntitle: A\nBody", "", "---\ntitle: A\nBody"},
		{"no frontmatter", "Body\n---\nx\n---\n", "", "Body\n---\nx\n---\n"},
		{"empty", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			front, body := extractFrontmatter(tc.in)
			if front != tc.front || body != tc.body {
				t.Errorf("got (%q, %q), want (%q, %q)", front, body, tc.front, tc.body)
			}
			if front+body != tc.in {
				t.Errorf("front+body = %q, want the input", front+body)
			}
		})
	}
}

func TestUpdateFrontmatter(t *testing.T) {
	got := updateFrontmatter("---\ntitle: A\n---\n", map[string]interface{}{"lang": "ru"})
	if got != "---\nlang: ru\ntitle: A\n---\n" {
		t.Errorf("update = %q", got)
	}

	got = updateFrontmatter("", map[string]interface{}{"lang": "ru"})
	if got != "---\nlang: ru\n---\n" {
		t.Errorf("new = %q", got)
	}

	got = updateFrontmatter("---\n---\n", map[string]interface{}{"lang": "ru"})
	if got != "---\nlang: ru\n---\n" {
		t.Errorf("empty = %q", got)
	}
}

func TestUpdateFrontmatterKeepsInvalidYAML(t *testing.T) {
	orig := "---\ntitle: [unclosed\nother: : :\n---\n"
	if got := updateFrontmatter(orig, map[string]interface{}{"lang": "ru"}); got != orig {
		t.Errorf("invalid frontmatter changed: %q", got)
	}
}
