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

// VLLMProvider talks to an OpenAI-compatible server such as vLLM or
// llama-swap. API key is optional. TranslateGemma models get a special
// request format because their chat template accepts only a single user
// message with structured content (no system prompt).
type VLLMProvider struct {
	OpenAIProvider
}

type translateGemmaRequest struct {
	Model       string                  `json:"model"`
	Messages    []translateGemmaMessage `json:"messages"`
	Temperature float64                 `json:"temperature"`
	MaxTokens   int                     `json:"max_tokens,omitempty"`
}

type translateGemmaMessage struct {
	Role    string                  `json:"role"`
	Content []translateGemmaContent `json:"content"`
}

type translateGemmaContent struct {
	Type           string `json:"type"`
	SourceLangCode string `json:"source_lang_code"`
	TargetLangCode string `json:"target_lang_code"`
	Text           string `json:"text"`
}

func NewVLLMProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	return &VLLMProvider{
		OpenAIProvider: OpenAIProvider{
			BaseProvider: BaseProvider{
				name:       "vllm",
				config:     cfg,
				httpClient: client,
			},
		},
	}
}

func (p *VLLMProvider) ValidateConfig() error {
	if p.config.BaseURL == "" {
		return fmt.Errorf("base URL is required for provider %s", p.name)
	}
	if p.config.Model == "" {
		return fmt.Errorf("model is required for provider %s", p.name)
	}
	return nil
}

func (p *VLLMProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	if !strings.Contains(strings.ToLower(p.config.Model), "translategemma") {
		return p.OpenAIProvider.Translate(ctx, req)
	}
	return p.translateGemma(ctx, req)
}

func (p *VLLMProvider) translateGemma(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	sourceLang := strings.TrimSpace(req.SourceLang)
	if sourceLang == "" || sourceLang == "auto" {
		return TranslateResponse{}, fmt.Errorf("model %s requires explicit source language (use --from)", p.config.Model)
	}

	tgReq := translateGemmaRequest{
		Model:       p.config.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Messages: []translateGemmaMessage{{
			Role: "user",
			Content: []translateGemmaContent{{
				Type:           "text",
				SourceLangCode: sourceLang,
				TargetLangCode: strings.TrimSpace(req.TargetLang),
				Text:           req.Text,
			}},
		}},
	}

	jsonData, err := json.Marshal(tgReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to unmarshal response (status %d): %w", resp.StatusCode, err)
	}

	if apiResp.Error != nil {
		return TranslateResponse{}, fmt.Errorf("vLLM API error: %s", apiResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return TranslateResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if len(apiResp.Choices) == 0 {
		return TranslateResponse{}, fmt.Errorf("no choices in response")
	}

	return TranslateResponse{
		Text:       strings.TrimSpace(apiResp.Choices[0].Message.Content),
		TokensUsed: apiResp.Usage.TotalTokens,
	}, nil
}

func init() {
	Register("vllm", NewVLLMProvider)
}
