package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DefaultProvider        string                     `yaml:"default_provider"`
	DefaultTargetLanguage  string                     `yaml:"default_target_language"`
	Settings              Settings                   `yaml:"settings"`
	StrongValidation      StrongValidation           `yaml:"strong_validation"`
	Proxy                 ProxyConfig                `yaml:"proxy"`
	Providers             map[string]ProviderConfig  `yaml:"providers"`
	Prompts               Prompts                    `yaml:"prompts"`
	Glossary              []GlossaryEntry            `yaml:"glossary"`
}

type Settings struct {
	Temperature     float64 `yaml:"temperature"`
	MaxTokens       int     `yaml:"max_tokens"`
	Timeout         int     `yaml:"timeout"`
	ChunkSize       int     `yaml:"chunk_size"`
	PreserveFormat  bool    `yaml:"preserve_format"`
	RetryCount      int     `yaml:"retry_count"`
	RetryDelay      int     `yaml:"retry_delay"`
	Sentiment       bool    `yaml:"sentiment"`
	TagsCount       int     `yaml:"tags_count"`
	Classify        bool    `yaml:"classify"`
	Emotions        bool    `yaml:"emotions"`
	Factuality      bool    `yaml:"factuality"`
}

type StrongValidation struct {
	Enabled         bool     `yaml:"enabled"`
	MaxRetries      int      `yaml:"max_retries"`
	AllowedPatterns []string `yaml:"allowed_patterns"`
	AllowedTerms    []string `yaml:"allowed_terms"`
}

type ProxyConfig struct {
	URL      string   `yaml:"url"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	NoProxy  []string `yaml:"no_proxy"`
}

type ProviderConfig struct {
	APIKey   string      `yaml:"api_key"`
	BaseURL  string      `yaml:"base_url"`
	Model    string      `yaml:"model"`
	Proxy    ProxyConfig `yaml:"proxy"`
}

type Prompts struct {
	System string            `yaml:"system"`
	Styles map[string]string `yaml:"styles"`
}

type GlossaryEntry struct {
	Term         string `yaml:"term"`
	Source       string `yaml:"source"`
	Target       string `yaml:"target"`
	Translation  string `yaml:"translation"`
	Note         string `yaml:"note"`
	CaseSensitive bool   `yaml:"case_sensitive"`
	Context      string `yaml:"context"`
}

func DefaultConfig() *Config {
	return &Config{
		DefaultProvider:       "openai",
		DefaultTargetLanguage: "en",
		Settings: Settings{
			Temperature:    0.3,
			MaxTokens:      4096,
			Timeout:        60,
			ChunkSize:      3000,
			PreserveFormat: false,
			RetryCount:     3,
			RetryDelay:     1,
			Sentiment:      false,
			TagsCount:      0,
			Classify:       false,
			Emotions:       false,
			Factuality:     false,
		},
		StrongValidation: StrongValidation{
			Enabled:    false,
			MaxRetries: 3,
			AllowedPatterns: []string{
				`\b[A-Z]{2,}\b`,
				`\b[a-z]+[A-Z][a-zA-Z]*\b`,
				`\b[A-Z][a-z]+[A-Z][a-zA-Z]*\b`,
				`"[^"]+`, `'[^']+'`,
				"`[^`]+`",
				`\b[a-z_]+\([^)]*\)`,
				`\b(Google|Microsoft|Apple|Amazon|OpenAI|Anthropic)\b`,
				`https?://[^\s]+`,
				`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
			},
			AllowedTerms: []string{
				"API", "HTTP", "JSON", "XML", "SQL", "REST", "GraphQL",
				"OAuth", "JWT", "SDK", "IDE", "CLI", "GUI", "URL", "IP",
				"DNS", "SSL", "TLS", "SSH", "FTP", "CPU", "GPU", "RAM",
				"SSD", "PDF", "HTML", "CSS", "iOS", "Android", "Linux",
				"Windows", "macOS",
			},
		},
		Providers: make(map[string]ProviderConfig),
		Prompts: Prompts{
			System: `You are a professional translator. Translate the following text from {source_lang} to {target_lang}. Preserve the original formatting and structure. Output only the translation without explanations.`,
			Styles: map[string]string{
				"formal":    "Use formal language appropriate for official documents.",
				"informal":  "Use casual, conversational language.",
				"technical": "Preserve technical terminology accurately.",
				"literary":  "Maintain literary style and artistic expression.",
			},
		},
	}
}

func ExpandEnvVars(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envVar := s[2 : len(s)-1]
		return os.Getenv(envVar)
	}
	return s
}

func GetConfigPaths() []string {
	paths := []string{
		"./llm-translate.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "llm-translate", "config.yaml"),
		"/etc/llm-translate/config.yaml",
	}
	return paths
}