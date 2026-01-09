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

const ClassifyPrompt = `Classify the following text into categories. Respond ONLY in this exact format:
TOPICS: <comma-separated list from: politics, economics, technology, medicine, incidents>
SCOPE: <comma-separated list from: regional, international>
TYPE: <comma-separated list from: corporate, regulatory, macro>

Rules:
- Select one or more values for each category
- Use only the exact values listed above
- If category doesn't apply, use "none"

Text to classify:`

const EmotionsPrompt = `Analyze the emotional tone of the following text. Respond ONLY in this exact format:
EMOTIONS: <comma-separated list of detected emotions with scores>

Available emotions and format:
fear:<0.0-1.0>, anger:<0.0-1.0>, hope:<0.0-1.0>, uncertainty:<0.0-1.0>, optimism:<0.0-1.0>, panic:<0.0-1.0>

Rules:
- Include only emotions with score > 0.1
- Score represents intensity (0.0 = absent, 1.0 = very strong)
- List emotions in descending order by score

Example response:
EMOTIONS: fear:0.8, uncertainty:0.6, panic:0.3

Text to analyze:`

const FactualityPrompt = `Analyze the factuality and speculativeness of the following text. Respond ONLY in this exact format:
FACTUALITY: <type> (<confidence 0.0-1.0>)
EVIDENCE: <comma-separated list of evidence types found>

Types (choose one):
- confirmed: verified facts with clear sources or official data
- rumors: unverified information, hearsay, "sources say"
- forecasts: predictions, projections, future expectations
- unsourced: claims without attribution or evidence

Evidence types to detect:
- official_source, statistics, quotes, documents, expert_opinion, anonymous_source, speculation, prediction

Example response:
FACTUALITY: rumors (0.7)
EVIDENCE: anonymous_source, speculation

Text to analyze:`

type Provider interface {
	Name() string
	Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error)
	AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error)
	ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error)
	Classify(ctx context.Context, text string) (ClassifyResponse, error)
	AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error)
	AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error)
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

type ClassifyResponse struct {
	Topics   []string // politics, economics, technology, medicine, incidents
	Scope    []string // regional, international
	NewsType []string // corporate, regulatory, macro
}

type EmotionsResponse struct {
	Emotions map[string]float64 // emotion -> intensity (0.0-1.0)
}

type FactualityResponse struct {
	Type       string   // confirmed, rumors, forecasts, unsourced
	Confidence float64  // 0.0-1.0
	Evidence   []string // evidence types found
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

func ParseClassifyResponse(response string) (ClassifyResponse, error) {
	response = strings.TrimSpace(response)
	result := ClassifyResponse{}

	// Parse TOPICS
	topicsRe := regexp.MustCompile(`(?i)TOPICS:\s*(.+)`)
	if matches := topicsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Topics = parseCommaSeparated(matches[1])
	}

	// Parse SCOPE
	scopeRe := regexp.MustCompile(`(?i)SCOPE:\s*(.+)`)
	if matches := scopeRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Scope = parseCommaSeparated(matches[1])
	}

	// Parse TYPE
	typeRe := regexp.MustCompile(`(?i)TYPE:\s*(.+)`)
	if matches := typeRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.NewsType = parseCommaSeparated(matches[1])
	}

	if len(result.Topics) == 0 && len(result.Scope) == 0 && len(result.NewsType) == 0 {
		return ClassifyResponse{}, fmt.Errorf("invalid classify response format: %s", response)
	}

	return result, nil
}

func parseCommaSeparated(s string) []string {
	var result []string
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" && p != "none" {
			result = append(result, p)
		}
	}
	return result
}

func ParseEmotionsResponse(response string) (EmotionsResponse, error) {
	response = strings.TrimSpace(response)
	result := EmotionsResponse{
		Emotions: make(map[string]float64),
	}

	re := regexp.MustCompile(`(?i)EMOTIONS:\s*(.+)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 2 {
		return EmotionsResponse{}, fmt.Errorf("invalid emotions response format: %s", response)
	}

	emotionRe := regexp.MustCompile(`(\w+):([0-9.]+)`)
	emotionMatches := emotionRe.FindAllStringSubmatch(matches[1], -1)

	for _, m := range emotionMatches {
		if len(m) >= 3 {
			emotion := strings.ToLower(m[1])
			score, err := strconv.ParseFloat(m[2], 64)
			if err == nil && score > 0 {
				result.Emotions[emotion] = score
			}
		}
	}

	if len(result.Emotions) == 0 {
		return EmotionsResponse{}, fmt.Errorf("no emotions found in response")
	}

	return result, nil
}

func ParseFactualityResponse(response string) (FactualityResponse, error) {
	response = strings.TrimSpace(response)
	result := FactualityResponse{}

	// Parse FACTUALITY line
	factRe := regexp.MustCompile(`(?i)FACTUALITY:\s*(\w+)\s*\(([0-9.]+)\)`)
	if matches := factRe.FindStringSubmatch(response); len(matches) >= 3 {
		result.Type = strings.ToLower(matches[1])
		if score, err := strconv.ParseFloat(matches[2], 64); err == nil {
			result.Confidence = score
		}
	}

	// Parse EVIDENCE line
	evidenceRe := regexp.MustCompile(`(?i)EVIDENCE:\s*(.+)`)
	if matches := evidenceRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Evidence = parseCommaSeparated(matches[1])
	}

	if result.Type == "" {
		return FactualityResponse{}, fmt.Errorf("invalid factuality response format: %s", response)
	}

	return result, nil
}