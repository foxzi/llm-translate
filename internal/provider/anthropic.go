package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/foxzi/llm-translate/internal/config"
)

type AnthropicProvider struct {
	BaseProvider
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature,omitempty"`
	System      string            `json:"system,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []anthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence *string           `json:"stop_sequence"`
	Usage        anthropicUsage    `json:"usage"`
	Error        *anthropicError   `json:"error,omitempty"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func NewAnthropicProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	if cfg.Model == "" {
		cfg.Model = "claude-3-5-sonnet-20241022"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	
	return &AnthropicProvider{
		BaseProvider: BaseProvider{
			name:       "anthropic",
			config:     cfg,
			httpClient: client,
		},
	}
}

func (p *AnthropicProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	systemPrompt := fmt.Sprintf(
		"You are a professional translator. Translate the following text from %s to %s. "+
			"Preserve the original formatting and structure. "+
			"Output only the translation without explanations.",
		req.SourceLang, req.TargetLang,
	)
	
	if req.SourceLang == "auto" {
		systemPrompt = fmt.Sprintf(
			"You are a professional translator. Detect the source language and translate the text to %s. "+
				"Preserve the original formatting and structure. "+
				"Output only the translation without explanations.",
			req.TargetLang,
		)
	}
	
	fullPrompt := p.buildPrompt(req, systemPrompt)
	
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      fullPrompt,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: req.Text,
			},
		},
	}
	
	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}
	
	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if anthropicResp.Error != nil {
		return TranslateResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}
	
	if resp.StatusCode != http.StatusOK {
		return TranslateResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	if len(anthropicResp.Content) == 0 {
		return TranslateResponse{}, fmt.Errorf("no content in response")
	}
	
	var translatedText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			translatedText += content.Text
		}
	}
	
	return TranslateResponse{
		Text:       translatedText,
		TokensUsed: anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
	}, nil
}

func (p *AnthropicProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      SentimentPrompt,
		MaxTokens:   100,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return SentimentResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return SentimentResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseSentimentResponse(responseText)
}

func (p *AnthropicProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	tagsPrompt := fmt.Sprintf(TagsPromptTemplate, count)

	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      tagsPrompt,
		MaxTokens:   200,
		Temperature: 0.3,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return TagsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return TagsResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return TagsResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseTagsResponse(responseText)
}

func (p *AnthropicProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      ClassifyPrompt,
		MaxTokens:   200,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return ClassifyResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return ClassifyResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseClassifyResponse(responseText)
}

func (p *AnthropicProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      EmotionsPrompt,
		MaxTokens:   200,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return EmotionsResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return EmotionsResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseEmotionsResponse(responseText)
}

func (p *AnthropicProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      FactualityPrompt,
		MaxTokens:   200,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return FactualityResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return FactualityResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseFactualityResponse(responseText)
}

func (p *AnthropicProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      ImpactPrompt,
		MaxTokens:   100,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return ImpactResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return ImpactResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseImpactResponse(responseText)
}

func (p *AnthropicProvider) AnalyzeSensationalism(ctx context.Context, text string) (SensationalismResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      SensationalismPrompt,
		MaxTokens:   150,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return SensationalismResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return SensationalismResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return SensationalismResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SensationalismResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return SensationalismResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return SensationalismResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return SensationalismResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseSensationalismResponse(responseText)
}

func (p *AnthropicProvider) ExtractEntities(ctx context.Context, text string) (EntitiesResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      EntitiesPrompt,
		MaxTokens:   300,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return EntitiesResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return EntitiesResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return EntitiesResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EntitiesResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return EntitiesResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return EntitiesResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return EntitiesResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseEntitiesResponse(responseText)
}

func (p *AnthropicProvider) ExtractEvents(ctx context.Context, text string) (EventsResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      EventsPrompt,
		MaxTokens:   200,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return EventsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return EventsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return EventsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EventsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return EventsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return EventsResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return EventsResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseEventsResponse(responseText)
}

func (p *AnthropicProvider) AnalyzeUsefulness(ctx context.Context, text string) (UsefulnessResponse, error) {
	anthropicReq := anthropicRequest{
		Model:       p.config.Model,
		System:      UsefulnessPrompt,
		MaxTokens:   200,
		Temperature: 0.1,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return UsefulnessResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return UsefulnessResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return UsefulnessResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UsefulnessResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return UsefulnessResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if anthropicResp.Error != nil {
		return UsefulnessResponse{}, fmt.Errorf("Anthropic API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return UsefulnessResponse{}, fmt.Errorf("no content in response")
	}

	var responseText string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return ParseUsefulnessResponse(responseText)
}

func init() {
	Register("anthropic", NewAnthropicProvider)
}