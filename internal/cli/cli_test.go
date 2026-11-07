package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// writeTestConfig writes a config pointing provider "openai" at the fake server.
func writeTestConfig(t *testing.T, f *fakeLLM, analyses bool) string {
	t.Helper()
	cfg := testConfig(t, f, "openai", "gpt-test")
	if !analyses {
		cfg.Settings.Sentiment, cfg.Settings.TagsCount = false, 0
		cfg.Settings.Classify, cfg.Settings.Emotions, cfg.Settings.Factuality = false, false, false
		cfg.Settings.Impact, cfg.Settings.Sensationalism, cfg.Settings.Entities = false, false, false
		cfg.Settings.Events, cfg.Settings.Usefulness, cfg.Settings.TimeFocus, cfg.Settings.AdDetect = false, false, false, false
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCLI(args ...string) error {
	cmd := newRootCmd(context.Background())
	cmd.SetArgs(args)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	return cmd.Execute()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCLIFileWithFrontmatter(t *testing.T) {
	f := newFakeLLM(t)
	cfgPath := writeTestConfig(t, f, true)
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.md"), filepath.Join(dir, "out.md")
	if err := os.WriteFile(in, []byte("---\ntitle: Tesla\n---\n"+testArticle), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runCLI("-c", cfgPath, "-i", in, "-o", out, "-f", "en", "-t", "de"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, out)
	front, body := extractFrontmatter(got)
	if body != "[de] "+testArticle {
		t.Errorf("body = %q", body)
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(strings.Trim(front, "-\n")), &fm); err != nil {
		t.Fatalf("frontmatter: %v\n%s", err, front)
	}
	if fm["title"] != "Tesla" || fm["sentiment"] != "positive" || fm["ad_type"] != "none" {
		t.Errorf("frontmatter = %v", fm)
	}
	if tags, _ := fm["tags"].([]any); len(tags) != 5 {
		t.Errorf("tags = %v", fm["tags"])
	}
}

func TestCLIFlagsOverrideConfig(t *testing.T) {
	f := newFakeLLM(t)
	cfgPath := writeTestConfig(t, f, true)
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.md"), filepath.Join(dir, "out.md")
	_ = os.WriteFile(in, []byte("hello"), 0o644)

	if err := runCLI("-c", cfgPath, "-i", in, "-o", out, "-t", "fr", "--tags", "2"); err != nil {
		t.Fatal(err)
	}
	front, body := extractFrontmatter(readFile(t, out))
	if body != "[fr] hello" {
		t.Errorf("body = %q", body)
	}
	if !strings.Contains(front, "tags:\n    - tesla\n    - electric cars\n") || strings.Contains(front, "- sales") {
		t.Errorf("--tags 2 not applied:\n%s", front)
	}
}

func TestCLINoAnalysisNoFrontmatter(t *testing.T) {
	f := newFakeLLM(t)
	cfgPath := writeTestConfig(t, f, false)
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.md"), filepath.Join(dir, "out.md")
	_ = os.WriteFile(in, []byte("hello"), 0o644)

	if err := runCLI("-c", cfgPath, "-i", in, "-o", out, "-f", "en", "-t", "de"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, out); got != "[de] hello" {
		t.Errorf("output = %q", got)
	}
}

func TestCLIDirectory(t *testing.T) {
	f := newFakeLLM(t)
	cfgPath := writeTestConfig(t, f, false)
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "a.md"), []byte("one"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("two"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "skip.go"), []byte("three"), 0o644)

	if err := runCLI("-c", cfgPath, "-d", dir, "--suffix", "_de", "-f", "en", "-t", "de"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "a_de.md")); got != "[de] one" {
		t.Errorf("a_de.md = %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "sub", "b_de.txt")); got != "[de] two" {
		t.Errorf("b_de.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "skip_de.go")); err == nil {
		t.Error("file with other extension was translated")
	}
	// A second run must not treat its own outputs as inputs.
	if err := runCLI("-c", cfgPath, "-d", dir, "--suffix", "_de", "-f", "en", "-t", "de"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a_de_de.md")); err == nil {
		t.Error("translated output was translated again")
	}
}

func TestCLIErrors(t *testing.T) {
	f := newFakeLLM(t)
	cfgPath := writeTestConfig(t, f, false)
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.md")
	_ = os.WriteFile(empty, nil, 0o644)

	cases := map[string][]string{
		"missing config": {"-c", filepath.Join(dir, "nope.yaml"), "-i", empty},
		"missing input":  {"-c", cfgPath, "-i", filepath.Join(dir, "nope.md")},
		"empty input":    {"-c", cfgPath, "-i", empty},
		"unknown flag":   {"--no-such-flag"},
	}
	for name, args := range cases {
		if err := runCLI(args...); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}

	f.failStatus, f.failMsg = 400, "model not found"
	in := filepath.Join(dir, "in.md")
	_ = os.WriteFile(in, []byte("hello"), 0o644)
	err := runCLI("-c", cfgPath, "-i", in, "-o", filepath.Join(dir, "out.md"))
	if err == nil || !strings.Contains(err.Error(), "translation failed") {
		t.Errorf("provider error: %v", err)
	}
}
