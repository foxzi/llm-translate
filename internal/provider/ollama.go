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

type OllamaProvider struct {
	BaseProvider
}

type ollamaRequest struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	System   string         `json:"system,omitempty"`
	Stream   bool           `json:"stream"`
	Options  ollamaOptions  `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int    `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	Context   []int  `json:"context,omitempty"`
	TotalDuration    int64 `json:"total_duration,omitempty"`
	LoadDuration     int64 `json:"load_duration,omitempty"`
	PromptEvalCount  int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount        int   `json:"eval_count,omitempty"`
	EvalDuration     int64 `json:"eval_duration,omitempty"`
	Error     string `json:"error,omitempty"`
}

func NewOllamaProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	if cfg.Model == "" {
		cfg.Model = "llama3.2"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	
	return &OllamaProvider{
		BaseProvider: BaseProvider{
			name:       "ollama",
			config:     cfg,
			httpClient: client,
		},
	}
}

func (p *OllamaProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
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
	
	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: fullPrompt,
		Prompt: req.Text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}
	
	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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
	
	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if ollamaResp.Error != "" {
		return TranslateResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}
	
	if resp.StatusCode != http.StatusOK {
		return TranslateResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	tokensUsed := 0
	if ollamaResp.PromptEvalCount > 0 && ollamaResp.EvalCount > 0 {
		tokensUsed = ollamaResp.PromptEvalCount + ollamaResp.EvalCount
	}
	
	return TranslateResponse{
		Text:       ollamaResp.Response,
		TokensUsed: tokensUsed,
	}, nil
}

func (p *OllamaProvider) ValidateConfig() error {
	if p.config.BaseURL == "" {
		return fmt.Errorf("base URL is required for provider %s", p.name)
	}
	return nil
}

func (p *OllamaProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: SentimentPrompt,
		Prompt: text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.1,
			NumPredict:  100,
		},
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return SentimentResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return SentimentResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	return ParseSentimentResponse(ollamaResp.Response)
}

func (p *OllamaProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	tagsPrompt := fmt.Sprintf(TagsPromptTemplate, count)

	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: tagsPrompt,
		Prompt: text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.3,
			NumPredict:  200,
		},
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return TagsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return TagsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return TagsResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	return ParseTagsResponse(ollamaResp.Response)
}

func (p *OllamaProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: ClassifyPrompt,
		Prompt: text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.1,
			NumPredict:  200,
		},
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return ClassifyResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return ClassifyResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	return ParseClassifyResponse(ollamaResp.Response)
}

func (p *OllamaProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: EmotionsPrompt,
		Prompt: text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.1,
			NumPredict:  200,
		},
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return EmotionsResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return EmotionsResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	return ParseEmotionsResponse(ollamaResp.Response)
}

func (p *OllamaProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: FactualityPrompt,
		Prompt: text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.1,
			NumPredict:  200,
		},
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return FactualityResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return FactualityResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	return ParseFactualityResponse(ollamaResp.Response)
}

func (p *OllamaProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	ollamaReq := ollamaRequest{
		Model:  p.config.Model,
		System: ImpactPrompt,
		Prompt: text,
		Stream: false,
		Options: ollamaOptions{
			Temperature: 0.1,
			NumPredict:  100,
		},
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/api/generate"
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

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return ImpactResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if ollamaResp.Error != "" {
		return ImpactResponse{}, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	return ParseImpactResponse(ollamaResp.Response)
}

func init() {
	Register("ollama", NewOllamaProvider)
}