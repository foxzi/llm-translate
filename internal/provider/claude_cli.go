package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/foxzi/llm-translate/internal/config"
)

type ClaudeCLIProvider struct {
	BaseProvider
	cliPath string
}

type claudeCLIResponse struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

func NewClaudeCLIProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	cliPath := cfg.BaseURL
	if cliPath == "" {
		cliPath = "claude"
	}

	return &ClaudeCLIProvider{
		BaseProvider: BaseProvider{
			name:       "claude-cli",
			config:     cfg,
			httpClient: client,
		},
		cliPath: cliPath,
	}
}

func (p *ClaudeCLIProvider) ValidateConfig() error {
	_, err := exec.LookPath(p.cliPath)
	if err != nil {
		return fmt.Errorf("claude CLI not found at '%s': %w", p.cliPath, err)
	}
	return nil
}

func (p *ClaudeCLIProvider) runCLI(ctx context.Context, prompt string, input string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "text"}

	cmd := exec.CommandContext(ctx, p.cliPath, args...)

	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("claude CLI error: %w, stderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (p *ClaudeCLIProvider) runCLIJSON(ctx context.Context, prompt string, input string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "json"}

	cmd := exec.CommandContext(ctx, p.cliPath, args...)

	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("claude CLI error: %w, stderr: %s", err, stderr.String())
	}

	var resp claudeCLIResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return strings.TrimSpace(stdout.String()), nil
	}

	return resp.Result, nil
}

func (p *ClaudeCLIProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	prompt := fmt.Sprintf(
		"You are a professional translator. Translate the following text from %s to %s. "+
			"Preserve the original formatting and structure. "+
			"Output only the translation without explanations.",
		req.SourceLang, req.TargetLang,
	)

	if req.SourceLang == "auto" {
		prompt = fmt.Sprintf(
			"You are a professional translator. Detect the source language and translate the text to %s. "+
				"Preserve the original formatting and structure. "+
				"Output only the translation without explanations.",
			req.TargetLang,
		)
	}

	fullPrompt := p.buildPrompt(req, prompt)

	result, err := p.runCLI(ctx, fullPrompt, req.Text)
	if err != nil {
		return TranslateResponse{}, err
	}

	return TranslateResponse{
		Text: result,
	}, nil
}

func (p *ClaudeCLIProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	prompt := SentimentPrompt

	result, err := p.runCLI(ctx, prompt, text)
	if err != nil {
		return SentimentResponse{}, err
	}

	return ParseSentimentResponse(result)
}

func (p *ClaudeCLIProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	prompt := fmt.Sprintf(TagsPromptTemplate, count)

	result, err := p.runCLI(ctx, prompt, text)
	if err != nil {
		return TagsResponse{}, err
	}

	return ParseTagsResponse(result)
}

func (p *ClaudeCLIProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	result, err := p.runCLI(ctx, ClassifyPrompt, text)
	if err != nil {
		return ClassifyResponse{}, err
	}

	return ParseClassifyResponse(result)
}

func (p *ClaudeCLIProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	result, err := p.runCLI(ctx, EmotionsPrompt, text)
	if err != nil {
		return EmotionsResponse{}, err
	}

	return ParseEmotionsResponse(result)
}

func (p *ClaudeCLIProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	result, err := p.runCLI(ctx, FactualityPrompt, text)
	if err != nil {
		return FactualityResponse{}, err
	}

	return ParseFactualityResponse(result)
}

func (p *ClaudeCLIProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	result, err := p.runCLI(ctx, ImpactPrompt, text)
	if err != nil {
		return ImpactResponse{}, err
	}

	return ParseImpactResponse(result)
}

func (p *ClaudeCLIProvider) AnalyzeSensationalism(ctx context.Context, text string) (SensationalismResponse, error) {
	result, err := p.runCLI(ctx, SensationalismPrompt, text)
	if err != nil {
		return SensationalismResponse{}, err
	}

	return ParseSensationalismResponse(result)
}

func (p *ClaudeCLIProvider) ExtractEntities(ctx context.Context, text string) (EntitiesResponse, error) {
	result, err := p.runCLI(ctx, EntitiesPrompt, text)
	if err != nil {
		return EntitiesResponse{}, err
	}

	return ParseEntitiesResponse(result)
}

func (p *ClaudeCLIProvider) ExtractEvents(ctx context.Context, text string) (EventsResponse, error) {
	result, err := p.runCLI(ctx, EventsPrompt, text)
	if err != nil {
		return EventsResponse{}, err
	}

	return ParseEventsResponse(result)
}

func (p *ClaudeCLIProvider) AnalyzeUsefulness(ctx context.Context, text string) (UsefulnessResponse, error) {
	result, err := p.runCLI(ctx, UsefulnessPrompt, text)
	if err != nil {
		return UsefulnessResponse{}, err
	}

	return ParseUsefulnessResponse(result)
}

func init() {
	Register("claude-cli", NewClaudeCLIProvider)
}
