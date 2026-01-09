package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/user/llm-translate/internal/config"
)

type OpenRouterProvider struct {
	BaseProvider
}

type openRouterRequest struct {
	Model       string       `json:"model"`
	Messages    []message    `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	TopP        float64      `json:"top_p,omitempty"`
	Stream      bool         `json:"stream"`
}

type openRouterResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Choices []choice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func NewOpenRouterProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	if cfg.Model == "" {
		cfg.Model = "anthropic/claude-3.5-sonnet"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai/api/v1"
	}
	
	return &OpenRouterProvider{
		BaseProvider: BaseProvider{
			name:       "openrouter",
			config:     cfg,
			httpClient: client,
		},
	}
}

func (p *OpenRouterProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
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
	
	openRouterReq := openRouterRequest{
		Model:       p.config.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
		Messages: []message{
			{
				Role:    "system",
				Content: fullPrompt,
			},
			{
				Role:    "user",
				Content: req.Text,
			},
		},
	}
	
	jsonData, err := json.Marshal(openRouterReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/user/llm-translate")
	httpReq.Header.Set("X-Title", "LLM Translate CLI")
	
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}
	
	var openRouterResp openRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if openRouterResp.Error != nil {
		return TranslateResponse{}, fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}
	
	if resp.StatusCode != http.StatusOK {
		return TranslateResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	if len(openRouterResp.Choices) == 0 {
		return TranslateResponse{}, fmt.Errorf("no choices in response")
	}
	
	return TranslateResponse{
		Text:       openRouterResp.Choices[0].Message.Content,
		TokensUsed: openRouterResp.Usage.TotalTokens,
	}, nil
}

func (p *OpenRouterProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	openRouterReq := openRouterRequest{
		Model:       p.config.Model,
		Temperature: 0.1,
		MaxTokens:   100,
		Stream:      false,
		Messages: []message{
			{
				Role:    "system",
				Content: SentimentPrompt,
			},
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(openRouterReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/user/llm-translate")
	httpReq.Header.Set("X-Title", "LLM Translate CLI")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openRouterResp openRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openRouterResp.Error != nil {
		return SentimentResponse{}, fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}

	if len(openRouterResp.Choices) == 0 {
		return SentimentResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseSentimentResponse(openRouterResp.Choices[0].Message.Content)
}

func (p *OpenRouterProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	tagsPrompt := fmt.Sprintf(TagsPromptTemplate, count)

	openRouterReq := openRouterRequest{
		Model:       p.config.Model,
		Temperature: 0.3,
		MaxTokens:   200,
		Stream:      false,
		Messages: []message{
			{
				Role:    "system",
				Content: tagsPrompt,
			},
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(openRouterReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/user/llm-translate")
	httpReq.Header.Set("X-Title", "LLM Translate CLI")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openRouterResp openRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return TagsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openRouterResp.Error != nil {
		return TagsResponse{}, fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}

	if len(openRouterResp.Choices) == 0 {
		return TagsResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseTagsResponse(openRouterResp.Choices[0].Message.Content)
}

func (p *OpenRouterProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	openRouterReq := openRouterRequest{
		Model:       p.config.Model,
		Temperature: 0.1,
		MaxTokens:   200,
		Stream:      false,
		Messages: []message{
			{
				Role:    "system",
				Content: ClassifyPrompt,
			},
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(openRouterReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/user/llm-translate")
	httpReq.Header.Set("X-Title", "LLM Translate CLI")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openRouterResp openRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openRouterResp.Error != nil {
		return ClassifyResponse{}, fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}

	if len(openRouterResp.Choices) == 0 {
		return ClassifyResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseClassifyResponse(openRouterResp.Choices[0].Message.Content)
}

func (p *OpenRouterProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	openRouterReq := openRouterRequest{
		Model:       p.config.Model,
		Temperature: 0.1,
		MaxTokens:   200,
		Stream:      false,
		Messages: []message{
			{
				Role:    "system",
				Content: EmotionsPrompt,
			},
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(openRouterReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/user/llm-translate")
	httpReq.Header.Set("X-Title", "LLM Translate CLI")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openRouterResp openRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openRouterResp.Error != nil {
		return EmotionsResponse{}, fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}

	if len(openRouterResp.Choices) == 0 {
		return EmotionsResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseEmotionsResponse(openRouterResp.Choices[0].Message.Content)
}

func init() {
	Register("openrouter", NewOpenRouterProvider)
}