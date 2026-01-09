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

type OpenAIProvider struct {
	BaseProvider
}

type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	ID      string  `json:"id"`
	Object  string  `json:"object"`
	Created int64   `json:"created"`
	Model   string  `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage   `json:"usage"`
	Error   *openAIError `json:"error,omitempty"`
}

type choice struct {
	Index   int     `json:"index"`
	Message message `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func NewOpenAIProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	
	return &OpenAIProvider{
		BaseProvider: BaseProvider{
			name:       "openai",
			config:     cfg,
			httpClient: client,
		},
	}
}

func (p *OpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
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
	
	openAIReq := openAIRequest{
		Model:       p.config.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
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
	
	jsonData, err := json.Marshal(openAIReq)
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
	
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}
	
	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if openAIResp.Error != nil {
		return TranslateResponse{}, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}
	
	if resp.StatusCode != http.StatusOK {
		return TranslateResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	if len(openAIResp.Choices) == 0 {
		return TranslateResponse{}, fmt.Errorf("no choices in response")
	}
	
	return TranslateResponse{
		Text:       openAIResp.Choices[0].Message.Content,
		TokensUsed: openAIResp.Usage.TotalTokens,
	}, nil
}

func (p *OpenAIProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	openAIReq := openAIRequest{
		Model:       p.config.Model,
		Temperature: 0.1,
		MaxTokens:   100,
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

	jsonData, err := json.Marshal(openAIReq)
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return SentimentResponse{}, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return SentimentResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseSentimentResponse(openAIResp.Choices[0].Message.Content)
}

func (p *OpenAIProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	tagsPrompt := fmt.Sprintf(TagsPromptTemplate, count)

	openAIReq := openAIRequest{
		Model:       p.config.Model,
		Temperature: 0.3,
		MaxTokens:   200,
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

	jsonData, err := json.Marshal(openAIReq)
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return TagsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return TagsResponse{}, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return TagsResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseTagsResponse(openAIResp.Choices[0].Message.Content)
}

func (p *OpenAIProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	openAIReq := openAIRequest{
		Model:       p.config.Model,
		Temperature: 0.1,
		MaxTokens:   200,
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

	jsonData, err := json.Marshal(openAIReq)
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return ClassifyResponse{}, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return ClassifyResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseClassifyResponse(openAIResp.Choices[0].Message.Content)
}

func (p *OpenAIProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	openAIReq := openAIRequest{
		Model:       p.config.Model,
		Temperature: 0.1,
		MaxTokens:   200,
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

	jsonData, err := json.Marshal(openAIReq)
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return EmotionsResponse{}, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return EmotionsResponse{}, fmt.Errorf("no choices in response")
	}

	return ParseEmotionsResponse(openAIResp.Choices[0].Message.Content)
}

func init() {
	Register("openai", NewOpenAIProvider)
}