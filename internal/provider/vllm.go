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
// message with structured content (no system prompt). Analysis requests for
// these models bypass the chat template via /completions with a raw Gemma
// prompt.
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

func (p *VLLMProvider) isTranslateGemma() bool {
	return strings.Contains(strings.ToLower(p.config.Model), "translategemma")
}

func (p *VLLMProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	if !p.isTranslateGemma() {
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

type completionRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Text string `json:"text"`
	} `json:"choices"`
	Error *openAIError `json:"error,omitempty"`
}

// gemmaComplete sends instruction and text as a single Gemma user turn to
// /completions, since the TranslateGemma chat template rejects free-form prompts.
func (p *VLLMProvider) gemmaComplete(ctx context.Context, instruction, text string, temperature float64, maxTokens int) (string, error) {
	prompt := "<start_of_turn>user\n" + instruction + "\n\n" + text + "<end_of_turn>\n<start_of_turn>model\n"
	jsonData, err := json.Marshal(completionRequest{
		Model:       p.config.Model,
		Prompt:      prompt,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp completionResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response (status %d): %w", resp.StatusCode, err)
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("vLLM API error: %s", apiResp.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return apiResp.Choices[0].Text, nil
}

func (p *VLLMProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeSentiment(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, SentimentPrompt, text, 0.1, 100)
	if err != nil {
		return SentimentResponse{}, err
	}
	return ParseSentimentResponse(out)
}

func (p *VLLMProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.ExtractTags(ctx, text, count)
	}
	out, err := p.gemmaComplete(ctx, fmt.Sprintf(TagsPromptTemplate, count), text, 0.3, 200)
	if err != nil {
		return TagsResponse{}, err
	}
	return ParseTagsResponse(out)
}

func (p *VLLMProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.Classify(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, ClassifyPrompt, text, 0.1, 200)
	if err != nil {
		return ClassifyResponse{}, err
	}
	return ParseClassifyResponse(out)
}

func (p *VLLMProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeEmotions(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, EmotionsPrompt, text, 0.1, 200)
	if err != nil {
		return EmotionsResponse{}, err
	}
	return ParseEmotionsResponse(out)
}

func (p *VLLMProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeFactuality(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, FactualityPrompt, text, 0.1, 200)
	if err != nil {
		return FactualityResponse{}, err
	}
	return ParseFactualityResponse(out)
}

func (p *VLLMProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeImpact(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, ImpactPrompt, text, 0.1, 100)
	if err != nil {
		return ImpactResponse{}, err
	}
	return ParseImpactResponse(out)
}

func (p *VLLMProvider) AnalyzeSensationalism(ctx context.Context, text string) (SensationalismResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeSensationalism(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, SensationalismPrompt, text, 0.1, 150)
	if err != nil {
		return SensationalismResponse{}, err
	}
	return ParseSensationalismResponse(out)
}

func (p *VLLMProvider) ExtractEntities(ctx context.Context, text string) (EntitiesResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.ExtractEntities(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, EntitiesPrompt, text, 0.1, 300)
	if err != nil {
		return EntitiesResponse{}, err
	}
	return ParseEntitiesResponse(out)
}

func (p *VLLMProvider) ExtractEvents(ctx context.Context, text string) (EventsResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.ExtractEvents(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, EventsPrompt, text, 0.1, 200)
	if err != nil {
		return EventsResponse{}, err
	}
	return ParseEventsResponse(out)
}

func (p *VLLMProvider) AnalyzeUsefulness(ctx context.Context, text string) (UsefulnessResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeUsefulness(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, UsefulnessPrompt, text, 0.1, 200)
	if err != nil {
		return UsefulnessResponse{}, err
	}
	return ParseUsefulnessResponse(out)
}

func (p *VLLMProvider) AnalyzeAdDetect(ctx context.Context, text string) (AdDetectResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeAdDetect(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, AdDetectPrompt, text, 0.1, 200)
	if err != nil {
		return AdDetectResponse{}, err
	}
	return ParseAdDetectResponse(out)
}

func (p *VLLMProvider) AnalyzeTimeFocus(ctx context.Context, text string) (TimeFocusResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeTimeFocus(ctx, text)
	}
	out, err := p.gemmaComplete(ctx, TimeFocusPrompt, text, 0.1, 200)
	if err != nil {
		return TimeFocusResponse{}, err
	}
	return ParseTimeFocusResponse(out)
}

func (p *VLLMProvider) AnalyzeCombined(ctx context.Context, req CombinedAnalysisRequest) (CombinedAnalysisResponse, error) {
	if !p.isTranslateGemma() {
		return p.OpenAIProvider.AnalyzeCombined(ctx, req)
	}
	out, err := p.gemmaComplete(ctx, BuildCombinedPrompt(req), req.Text, 0.2, 2000)
	if err != nil {
		return CombinedAnalysisResponse{}, err
	}
	return ParseCombinedResponse(out, req), nil
}

func init() {
	Register("vllm", NewVLLMProvider)
}
