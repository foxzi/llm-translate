package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/user/llm-translate/internal/config"
)

type Provider interface {
	Name() string
	Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error)
	ValidateConfig() error
}

type TranslateRequest struct {
	Text           string
	SourceLang     string
	TargetLang     string
	Style          string
	Context        string
	Glossary       []config.GlossaryEntry
	Temperature    float64
	MaxTokens      int
	PreserveFormat bool
}

type TranslateResponse struct {
	Text         string
	DetectedLang string
	TokensUsed   int
}

type BaseProvider struct {
	name       string
	config     config.ProviderConfig
	httpClient *http.Client
}

func (b *BaseProvider) Name() string {
	return b.name
}

func (b *BaseProvider) ValidateConfig() error {
	if b.config.BaseURL == "" {
		return fmt.Errorf("base URL is required for provider %s", b.name)
	}
	if b.name != "ollama" && b.config.APIKey == "" {
		return fmt.Errorf("API key is required for provider %s", b.name)
	}
	return nil
}

func (b *BaseProvider) buildPrompt(req TranslateRequest, systemPrompt string) string {
	prompt := systemPrompt
	
	if req.Context != "" {
		prompt += "\n\nContext: " + req.Context
	}
	
	if req.Style != "" {
		stylePrompts := map[string]string{
			"formal":    "Use formal language appropriate for official documents.",
			"informal":  "Use casual, conversational language.",
			"technical": "Preserve technical terminology accurately.",
			"literary":  "Maintain literary style and artistic expression.",
		}
		if stylePrompt, ok := stylePrompts[req.Style]; ok {
			prompt += "\n\n" + stylePrompt
		}
	}
	
	if len(req.Glossary) > 0 {
		prompt += "\n\nGlossary (use these translations):\n"
		for _, entry := range req.Glossary {
			if entry.Source != "" && entry.Target != "" {
				prompt += fmt.Sprintf("- %s -> %s", entry.Source, entry.Target)
				if entry.Note != "" {
					prompt += " (" + entry.Note + ")"
				}
				prompt += "\n"
			}
		}
	}
	
	if req.PreserveFormat {
		prompt += "\n\nPreserve all formatting including markdown, HTML tags, and code blocks."
	}
	
	return prompt
}

type Registry struct {
	providers map[string]func(config.ProviderConfig, *http.Client) Provider
}

var registry = &Registry{
	providers: make(map[string]func(config.ProviderConfig, *http.Client) Provider),
}

func Register(name string, factory func(config.ProviderConfig, *http.Client) Provider) {
	registry.providers[name] = factory
}

func Get(name string, cfg config.ProviderConfig, client *http.Client) (Provider, error) {
	factory, ok := registry.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	
	provider := factory(cfg, client)
	if err := provider.ValidateConfig(); err != nil {
		return nil, err
	}
	
	return provider, nil
}

func ListProviders() []string {
	names := make([]string, 0, len(registry.providers))
	for name := range registry.providers {
		names = append(names, name)
	}
	return names
}