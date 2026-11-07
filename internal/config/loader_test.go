package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadNewsPortalConfig checks that the NewsPortal client config, including
// the unknown batch_analysis key, keeps loading with the same values.
func TestLoadNewsPortalConfig(t *testing.T) {
	t.Setenv("TEST_SECRET", "s3cret")
	cfg, err := Load("testdata/newsportal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Settings
	if cfg.DefaultProvider != "vllm" || cfg.DefaultTargetLanguage != "ru" {
		t.Errorf("provider/lang = %s/%s", cfg.DefaultProvider, cfg.DefaultTargetLanguage)
	}
	if s.Temperature != 0.3 || s.MaxTokens != 4096 || s.Timeout != 900 || s.ChunkSize != 3000 ||
		!s.PreserveFormat || s.RetryCount != 3 || s.RetryDelay != 1 {
		t.Errorf("settings = %+v", s)
	}
	if !s.Sentiment || s.TagsCount != 5 || !s.Classify || !s.Emotions || !s.Factuality || !s.Impact ||
		!s.Sensationalism || !s.Entities || !s.Events || !s.Usefulness || !s.TimeFocus || !s.AdDetect {
		t.Errorf("analyses = %+v", s)
	}
	if s.ParallelAnalysis || s.APIToken != "" {
		t.Errorf("unset options must stay off: %+v", s)
	}
	if p := cfg.Providers["vllm"]; p.BaseURL != "http://islacruses.main.lan:8000/v1" || p.Model != "translategemma-12b" {
		t.Errorf("vllm = %+v", p)
	}
	if cfg.Providers["openai"].APIKey != "s3cret" {
		t.Errorf("env vars are not expanded in api_key")
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "batch_analysis") {
		t.Errorf("warnings = %q", cfg.Warnings)
	}
	if len(cfg.Providers) != 7 || len(cfg.Glossary) != 2 || cfg.StrongValidation.Enabled {
		t.Errorf("providers=%d glossary=%d strong=%v", len(cfg.Providers), len(cfg.Glossary), cfg.StrongValidation.Enabled)
	}
}

func TestLoadDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte("default_provider: openai\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	def := DefaultConfig()
	if len(cfg.Warnings) != 0 {
		t.Errorf("warnings = %q", cfg.Warnings)
	}
	if cfg.Settings != def.Settings || cfg.DefaultTargetLanguage != "en" {
		t.Errorf("defaults changed: %+v", cfg.Settings)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("testdata/missing.yaml"); err == nil {
		t.Fatal("expected error for missing config")
	}
}
