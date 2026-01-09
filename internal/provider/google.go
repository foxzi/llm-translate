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

func (p *GoogleProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     0.1,
			MaxOutputTokens: 100,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: SentimentPrompt},
			},
		},
	}

	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if googleResp.Error != nil {
		return SentimentResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}

	if len(googleResp.Candidates) == 0 {
		return SentimentResponse{}, fmt.Errorf("no candidates in response")
	}

	var responseText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	return ParseSentimentResponse(responseText)
}

func (p *GoogleProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	tagsPrompt := fmt.Sprintf(TagsPromptTemplate, count)

	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     0.3,
			MaxOutputTokens: 200,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: tagsPrompt},
			},
		},
	}

	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return TagsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if googleResp.Error != nil {
		return TagsResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}

	if len(googleResp.Candidates) == 0 {
		return TagsResponse{}, fmt.Errorf("no candidates in response")
	}

	var responseText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	return ParseTagsResponse(responseText)
}

func (p *GoogleProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     0.1,
			MaxOutputTokens: 200,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: ClassifyPrompt},
			},
		},
	}

	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if googleResp.Error != nil {
		return ClassifyResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}

	if len(googleResp.Candidates) == 0 {
		return ClassifyResponse{}, fmt.Errorf("no candidates in response")
	}

	var responseText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	return ParseClassifyResponse(responseText)
}

func (p *GoogleProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     0.1,
			MaxOutputTokens: 200,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: EmotionsPrompt},
			},
		},
	}

	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if googleResp.Error != nil {
		return EmotionsResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}

	if len(googleResp.Candidates) == 0 {
		return EmotionsResponse{}, fmt.Errorf("no candidates in response")
	}

	var responseText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	return ParseEmotionsResponse(responseText)
}

func (p *GoogleProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     0.1,
			MaxOutputTokens: 200,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: FactualityPrompt},
			},
		},
	}

	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if googleResp.Error != nil {
		return FactualityResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}

	if len(googleResp.Candidates) == 0 {
		return FactualityResponse{}, fmt.Errorf("no candidates in response")
	}

	var responseText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	return ParseFactualityResponse(responseText)
}

func (p *GoogleProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	googleReq := googleRequest{
		Contents: []googleContent{
			{
				Parts: []googlePart{
					{Text: text},
				},
				Role: "user",
			},
		},
		GenerationConfig: googleGenConfig{
			Temperature:     0.1,
			MaxOutputTokens: 100,
		},
		SystemInstruction: &googleContent{
			Parts: []googlePart{
				{Text: ImpactPrompt},
			},
		},
	}

	jsonData, err := json.Marshal(googleReq)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimRight(p.config.BaseURL, "/"),
		p.config.Model,
		p.config.APIKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var googleResp googleResponse
	if err := json.Unmarshal(body, &googleResp); err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if googleResp.Error != nil {
		return ImpactResponse{}, fmt.Errorf("Google API error: %s", googleResp.Error.Message)
	}

	if len(googleResp.Candidates) == 0 {
		return ImpactResponse{}, fmt.Errorf("no candidates in response")
	}

	var responseText string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		responseText += part.Text
	}

	return ParseImpactResponse(responseText)
}

func init() {
	Register("google", NewGoogleProvider)
}