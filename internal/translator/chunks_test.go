package translator

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitIntoChunksNoChunking(t *testing.T) {
	text := "one two three\n\nfour five"
	for _, size := range []int{0, -1} {
		got := (&Translator{}).splitIntoChunks(text, size)
		if len(got) != 1 || got[0] != text {
			t.Errorf("size %d: chunks = %q, want the whole text", size, got)
		}
	}
}

func TestSplitIntoChunksParagraphs(t *testing.T) {
	text := "aaaa aaaa\n\nbbbb bbbb\n\ncccc cccc"
	got := (&Translator{}).splitIntoChunks(text, 20)
	want := []string{"aaaa aaaa\n\nbbbb bbbb", "cccc cccc"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("chunks = %q, want %q", got, want)
	}
}

func TestSplitIntoChunksNoEmptyChunks(t *testing.T) {
	texts := []string{
		"supercalifragilistic is long. short words here and there.",
		"supercalifragilistic",
		"a b\n\n   \n\nsupercalifragilistic and more words follow",
		"ok.  Supercalifragilistic is long enough to overflow. Next.",
	}
	for _, text := range texts {
		for _, size := range []int{1, 5, 10, 15} {
			for i, c := range (&Translator{}).splitIntoChunks(text, size) {
				if strings.TrimSpace(c) == "" {
					t.Errorf("size %d, text %q: empty chunk %d", size, text, i)
				}
			}
		}
	}
}

func TestSplitIntoChunksCountsRunes(t *testing.T) {
	text := strings.Repeat("слово ", 10) // 60 runes, 110 bytes
	got := (&Translator{}).splitIntoChunks(text, 60)
	if len(got) != 1 {
		t.Fatalf("60 runes must fit chunk_size 60, got %d chunks", len(got))
	}
	for _, c := range (&Translator{}).splitIntoChunks(text, 20) {
		if n := utf8.RuneCountInString(c); n > 20 {
			t.Errorf("chunk %q has %d runes, limit 20", c, n)
		}
	}
}

func TestSplitIntoChunksKeepsFencedBlock(t *testing.T) {
	for _, fence := range []string{"```", "~~~"} {
		block := fence + "go\nfunc a() {}\n\nfunc b() {}\n\nfunc c() {}\n" + fence
		text := "Intro paragraph.\n\n" + block + "\n\nOutro paragraph."
		got := (&Translator{}).splitIntoChunks(text, 30)
		found := false
		for _, c := range got {
			if strings.Contains(c, "func a") != strings.Contains(c, "func c") {
				t.Errorf("%s block split across chunks: %q", fence, got)
			}
			found = found || c == block
		}
		if !found {
			t.Errorf("%s block must be a chunk of its own: %q", fence, got)
		}
		if joined := strings.Join(got, "\n\n"); joined != text {
			t.Errorf("chunks do not rejoin to the text: %q", joined)
		}
	}
}

func TestSplitParagraphsFences(t *testing.T) {
	text := "a\n\n````\n```\n\ninner\n````\n\nb\n\n~~~\nx\n~~~ y\n\nstill code\n~~~\n\nc"
	got := splitParagraphs(text)
	var texts []string
	for _, p := range got {
		texts = append(texts, p.text)
	}
	want := []string{"a", "````\n```\n\ninner\n````", "b", "~~~\nx\n~~~ y\n\nstill code\n~~~", "c"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("paragraphs = %q, want %q", texts, want)
	}
	for i, p := range got {
		if p.fenced != (i == 1 || i == 3) {
			t.Errorf("paragraph %d fenced = %v", i, p.fenced)
		}
	}
}
