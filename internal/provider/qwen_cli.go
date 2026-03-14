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

type QwenCLIProvider struct {
	BaseProvider
	cliPath string
}

type qwenJSONEvent struct {
	Type    string           `json:"type"`
	SubType string           `json:"subtype,omitempty"`
	Result  string           `json:"result,omitempty"`
	IsError bool             `json:"is_error,omitempty"`
	Usage   *qwenUsage       `json:"usage,omitempty"`
	Message *qwenJSONMessage `json:"message,omitempty"`
}

type qwenJSONMessage struct {
	Content []qwenContentBlock `json:"content,omitempty"`
	Usage   *qwenUsage         `json:"usage,omitempty"`
}

type qwenContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type qwenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func NewQwenCLIProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	cliPath := cfg.BaseURL
	if cliPath == "" {
		cliPath = "qwen"
	}

	return &QwenCLIProvider{
		BaseProvider: BaseProvider{
			name:       "qwen-cli",
			config:     cfg,
			httpClient: client,
		},
		cliPath: cliPath,
	}
}

func (p *QwenCLIProvider) ValidateConfig() error {
	_, err := exec.LookPath(p.cliPath)
	if err != nil {
		return fmt.Errorf("qwen CLI not found at '%s': %w", p.cliPath, err)
	}
	return nil
}

func (p *QwenCLIProvider) runCLI(ctx context.Context, prompt string, input string) (string, error) {
	args := []string{"-p", prompt, "-o", "text"}

	cmd := exec.CommandContext(ctx, p.cliPath, args...)

	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("qwen CLI error: %w, stderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (p *QwenCLIProvider) runCLIJSON(ctx context.Context, prompt string, input string) (string, int, error) {
	args := []string{"-p", prompt, "-o", "json"}

	cmd := exec.CommandContext(ctx, p.cliPath, args...)

	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", 0, fmt.Errorf("qwen CLI error: %w, stderr: %s", err, stderr.String())
	}

	var events []qwenJSONEvent
	if err := json.Unmarshal(stdout.Bytes(), &events); err != nil {
		return strings.TrimSpace(stdout.String()), 0, nil
	}

	var resultText string
	var tokensUsed int

	for _, event := range events {
		if event.Type == "result" && !event.IsError {
			resultText = event.Result
			if event.Usage != nil {
				tokensUsed = event.Usage.TotalTokens
				if tokensUsed == 0 {
					tokensUsed = event.Usage.InputTokens + event.Usage.OutputTokens
				}
			}
		}
	}

	if resultText == "" {
		// Fallback: look for assistant messages with text content
		for i := len(events) - 1; i >= 0; i-- {
			event := events[i]
			if event.Type == "assistant" && event.Message != nil {
				for _, block := range event.Message.Content {
					if block.Type == "text" && block.Text != "" {
						resultText = block.Text
						break
					}
				}
				if resultText != "" {
					if event.Message.Usage != nil {
						tokensUsed = event.Message.Usage.TotalTokens
						if tokensUsed == 0 {
							tokensUsed = event.Message.Usage.InputTokens + event.Message.Usage.OutputTokens
						}
					}
					break
				}
			}
		}
	}

	if resultText == "" {
		return strings.TrimSpace(stdout.String()), tokensUsed, nil
	}

	return resultText, tokensUsed, nil
}

func (p *QwenCLIProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
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

	result, tokensUsed, err := p.runCLIJSON(ctx, fullPrompt, req.Text)
	if err != nil {
		result, err = p.runCLI(ctx, fullPrompt, req.Text)
		if err != nil {
			return TranslateResponse{}, err
		}
	}

	return TranslateResponse{
		Text:       result,
		TokensUsed: tokensUsed,
	}, nil
}

func (p *QwenCLIProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	result, _, err := p.runCLIJSON(ctx, SentimentPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, SentimentPrompt, text)
		if err != nil {
			return SentimentResponse{}, err
		}
	}

	return ParseSentimentResponse(result)
}

func (p *QwenCLIProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	prompt := fmt.Sprintf(TagsPromptTemplate, count)

	result, _, err := p.runCLIJSON(ctx, prompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, prompt, text)
		if err != nil {
			return TagsResponse{}, err
		}
	}

	return ParseTagsResponse(result)
}

func (p *QwenCLIProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	result, _, err := p.runCLIJSON(ctx, ClassifyPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, ClassifyPrompt, text)
		if err != nil {
			return ClassifyResponse{}, err
		}
	}

	return ParseClassifyResponse(result)
}

func (p *QwenCLIProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	result, _, err := p.runCLIJSON(ctx, EmotionsPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, EmotionsPrompt, text)
		if err != nil {
			return EmotionsResponse{}, err
		}
	}

	return ParseEmotionsResponse(result)
}

func (p *QwenCLIProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	result, _, err := p.runCLIJSON(ctx, FactualityPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, FactualityPrompt, text)
		if err != nil {
			return FactualityResponse{}, err
		}
	}

	return ParseFactualityResponse(result)
}

func (p *QwenCLIProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	result, _, err := p.runCLIJSON(ctx, ImpactPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, ImpactPrompt, text)
		if err != nil {
			return ImpactResponse{}, err
		}
	}

	return ParseImpactResponse(result)
}

func (p *QwenCLIProvider) AnalyzeSensationalism(ctx context.Context, text string) (SensationalismResponse, error) {
	result, _, err := p.runCLIJSON(ctx, SensationalismPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, SensationalismPrompt, text)
		if err != nil {
			return SensationalismResponse{}, err
		}
	}

	return ParseSensationalismResponse(result)
}

func (p *QwenCLIProvider) ExtractEntities(ctx context.Context, text string) (EntitiesResponse, error) {
	result, _, err := p.runCLIJSON(ctx, EntitiesPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, EntitiesPrompt, text)
		if err != nil {
			return EntitiesResponse{}, err
		}
	}

	return ParseEntitiesResponse(result)
}

func (p *QwenCLIProvider) ExtractEvents(ctx context.Context, text string) (EventsResponse, error) {
	result, _, err := p.runCLIJSON(ctx, EventsPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, EventsPrompt, text)
		if err != nil {
			return EventsResponse{}, err
		}
	}

	return ParseEventsResponse(result)
}

func (p *QwenCLIProvider) AnalyzeUsefulness(ctx context.Context, text string) (UsefulnessResponse, error) {
	result, _, err := p.runCLIJSON(ctx, UsefulnessPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, UsefulnessPrompt, text)
		if err != nil {
			return UsefulnessResponse{}, err
		}
	}

	return ParseUsefulnessResponse(result)
}

func (p *QwenCLIProvider) AnalyzeTimeFocus(ctx context.Context, text string) (TimeFocusResponse, error) {
	result, _, err := p.runCLIJSON(ctx, TimeFocusPrompt, text)
	if err != nil {
		result, err = p.runCLI(ctx, TimeFocusPrompt, text)
		if err != nil {
			return TimeFocusResponse{}, err
		}
	}

	return ParseTimeFocusResponse(result)
}

func (p *QwenCLIProvider) AnalyzeCombined(ctx context.Context, req CombinedAnalysisRequest) (CombinedAnalysisResponse, error) {
	prompt := BuildCombinedPrompt(req)

	result, _, err := p.runCLIJSON(ctx, prompt, req.Text)
	if err != nil {
		result, err = p.runCLI(ctx, prompt, req.Text)
		if err != nil {
			return CombinedAnalysisResponse{}, err
		}
	}

	return ParseCombinedResponse(result, req), nil
}

func init() {
	Register("qwen-cli", NewQwenCLIProvider)
}
