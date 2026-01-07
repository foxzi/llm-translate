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

type GoogleProvider struct {
	BaseProvider
}

type googleRequest struct {
	Contents         []googleContent     `json:"contents"`
	GenerationConfig googleGenConfig     `json:"generationConfig,omitempty"`
	SystemInstruction *googleContent     `json:"systemInstruction,omitempty"`
}

type googleContent struct {
	Parts []googlePart `json:"parts"`
	Role  string      `json:"role,omitempty"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
}

type googleResponse struct {
	Candidates     []googleCandidate `json:"candidates"`
	UsageMetadata  googleUsage      `json:"usageMetadata"`
	Error          *googleError     `json:"error,omitempty"`
}

type googleCandidate struct {
	Content       googleContent `json:"content"`
	FinishReason  string       `json:"finishReason"`
	Index         int          `json:"index"`
}

type googleUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type googleError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func NewGoogleProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	if cfg.Model == "" {
		cfg.Model = "gemini-2.0-flash"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	
	return &GoogleProvider{
		BaseProvider: BaseProvider{
			name:       "google",
			config:     cfg,
			httpClient: client,
		},
	}
}

func (p *GoogleProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
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
	
	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: req.Text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: fullPrompt},
			},
		},
	}
	
	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", 
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}
	
	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if googleResp.Error != nil {
		return TranslateResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}
	
	if resp.StatusCode != http.StatusOK {
		return TranslateResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	if len(googleResp.Candidates) == 0 {
		return TranslateResponse{}, fmt.Errorf("no candidates in response")
	}
	
	var translatedText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		translatedText += part.Text
	}
	
	return TranslateResponse{
		Text:       translatedText,
		TokensUsed: googleResp.UsageMetadata.TotalTokenCount,
	}, nil
}

func init() {
	Register("google", NewGoogleProvider)
}