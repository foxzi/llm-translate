package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/user/llm-translate/internal/config"
)

const SentimentPrompt = `Analyze the sentiment of the following text. Respond ONLY with a single line in format:
SENTIMENT: <positive|negative|neutral> (<score from -1.0 to 1.0>)

Text to analyze:`

const TagsPromptTemplate = `Extract the %d most important keywords/tags from the following text.
Respond ONLY with a single line in format:
TAGS: tag1, tag2, tag3, ...

Use lowercase, single words or short phrases. No hashtags.

Text to analyze:`

type Provider interface {
	Name() string
	Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error)
	AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error)
	ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error)
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

type SentimentResponse struct {
	Sentiment  string  // positive, negative, neutral
	Score      float64 // -1.0 to 1.0
	Confidence float64 // 0.0 to 1.0
}

type TagsResponse struct {
	Tags []string
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

func ParseSentimentResponse(response string) (SentimentResponse, error) {
	response = strings.TrimSpace(response)

	re := regexp.MustCompile(`(?i)SENTIMENT:\s*(positive|negative|neutral)\s*\(([+-]?\d*\.?\d+)\)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 3 {
		return SentimentResponse{}, fmt.Errorf("invalid sentiment response format: %s", response)
	}

	sentiment := strings.ToLower(matches[1])
	score, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("invalid score: %w", err)
	}

	confidence := 1.0 - (1.0-abs(score))*0.5

	return SentimentResponse{
		Sentiment:  sentiment,
		Score:      score,
		Confidence: confidence,
	}, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func ParseTagsResponse(response string) (TagsResponse, error) {
	response = strings.TrimSpace(response)

	re := regexp.MustCompile(`(?i)TAGS:\s*(.+)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 2 {
		return TagsResponse{}, fmt.Errorf("invalid tags response format: %s", response)
	}

	tagsStr := matches[1]
	rawTags := strings.Split(tagsStr, ",")

	var tags []string
	for _, tag := range rawTags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	if len(tags) == 0 {
		return TagsResponse{}, fmt.Errorf("no tags found in response")
	}

	return TagsResponse{Tags: tags}, nil
}