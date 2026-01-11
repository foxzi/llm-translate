package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/user/llm-translate/internal/config"
)

type CodexCLIProvider struct {
	BaseProvider
	cliPath string
}

type codexJSONEvent struct {
	Type   string         `json:"type"`
	Item   *codexJSONItem `json:"item,omitempty"`
	Usage  *codexUsage    `json:"usage,omitempty"`
}

type codexJSONItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Text   string `json:"text"`
	Status string `json:"status,omitempty"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func NewCodexCLIProvider(cfg config.ProviderConfig, client *http.Client) Provider {
	cliPath := cfg.BaseURL
	if cliPath == "" {
		cliPath = "codex"
	}

	return &CodexCLIProvider{
		BaseProvider: BaseProvider{
			name:       "codex-cli",
			config:     cfg,
			httpClient: client,
		},
		cliPath: cliPath,
	}
}

func (p *CodexCLIProvider) ValidateConfig() error {
	_, err := exec.LookPath(p.cliPath)
	if err != nil {
		return fmt.Errorf("codex CLI not found at '%s': %w", p.cliPath, err)
	}
	return nil
}

func (p *CodexCLIProvider) runCLI(ctx context.Context, prompt string) (string, error) {
	args := []string{"exec", prompt}

	cmd := exec.CommandContext(ctx, p.cliPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("codex CLI error: %w, stderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (p *CodexCLIProvider) runCLIJSON(ctx context.Context, prompt string) (string, int, error) {
	args := []string{"exec", "--json", prompt}

	cmd := exec.CommandContext(ctx, p.cliPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", 0, fmt.Errorf("codex CLI error: %w, stderr: %s", err, stderr.String())
	}

	var lastMessage string
	var tokensUsed int

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event codexJSONEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.Type == "item.completed" && event.Item != nil {
			if event.Item.Type == "agent_message" || event.Item.Type == "message" {
				lastMessage = event.Item.Text
			}
		}

		if event.Type == "turn.completed" && event.Usage != nil {
			tokensUsed = event.Usage.InputTokens + event.Usage.OutputTokens
		}
	}

	if lastMessage == "" {
		return strings.TrimSpace(stdout.String()), tokensUsed, nil
	}

	return lastMessage, tokensUsed, nil
}

func (p *CodexCLIProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	prompt := fmt.Sprintf(
		"You are a professional translator. Translate the following text from %s to %s. "+
			"Preserve the original formatting and structure. "+
			"Output only the translation without explanations.\n\nText to translate:\n%s",
		req.SourceLang, req.TargetLang, req.Text,
	)

	if req.SourceLang == "auto" {
		prompt = fmt.Sprintf(
			"You are a professional translator. Detect the source language and translate the text to %s. "+
				"Preserve the original formatting and structure. "+
				"Output only the translation without explanations.\n\nText to translate:\n%s",
			req.TargetLang, req.Text,
		)
	}

	if req.Style != "" {
		stylePrompts := map[string]string{
			"formal":    "Use formal language appropriate for official documents.",
			"informal":  "Use casual, conversational language.",
			"technical": "Preserve technical terminology accurately.",
			"literary":  "Maintain literary style and artistic expression.",
		}
		if stylePrompt, ok := stylePrompts[req.Style]; ok {
			prompt = stylePrompt + " " + prompt
		}
	}

	result, tokensUsed, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return TranslateResponse{}, err
		}
	}

	return TranslateResponse{
		Text:       result,
		TokensUsed: tokensUsed,
	}, nil
}

func (p *CodexCLIProvider) AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error) {
	prompt := SentimentPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return SentimentResponse{}, err
		}
	}

	return ParseSentimentResponse(result)
}

func (p *CodexCLIProvider) ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error) {
	prompt := fmt.Sprintf(TagsPromptTemplate, count) + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return TagsResponse{}, err
		}
	}

	return ParseTagsResponse(result)
}

func (p *CodexCLIProvider) Classify(ctx context.Context, text string) (ClassifyResponse, error) {
	prompt := ClassifyPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return ClassifyResponse{}, err
		}
	}

	return ParseClassifyResponse(result)
}

func (p *CodexCLIProvider) AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error) {
	prompt := EmotionsPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return EmotionsResponse{}, err
		}
	}

	return ParseEmotionsResponse(result)
}

func (p *CodexCLIProvider) AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error) {
	prompt := FactualityPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return FactualityResponse{}, err
		}
	}

	return ParseFactualityResponse(result)
}

func (p *CodexCLIProvider) AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error) {
	prompt := ImpactPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return ImpactResponse{}, err
		}
	}

	return ParseImpactResponse(result)
}

func (p *CodexCLIProvider) AnalyzeSensationalism(ctx context.Context, text string) (SensationalismResponse, error) {
	prompt := SensationalismPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return SensationalismResponse{}, err
		}
	}

	return ParseSensationalismResponse(result)
}

func (p *CodexCLIProvider) ExtractEntities(ctx context.Context, text string) (EntitiesResponse, error) {
	prompt := EntitiesPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return EntitiesResponse{}, err
		}
	}

	return ParseEntitiesResponse(result)
}

func (p *CodexCLIProvider) ExtractEvents(ctx context.Context, text string) (EventsResponse, error) {
	prompt := EventsPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return EventsResponse{}, err
		}
	}

	return ParseEventsResponse(result)
}

func (p *CodexCLIProvider) AnalyzeUsefulness(ctx context.Context, text string) (UsefulnessResponse, error) {
	prompt := UsefulnessPrompt + "\n\n" + text

	result, _, err := p.runCLIJSON(ctx, prompt)
	if err != nil {
		result, err = p.runCLI(ctx, prompt)
		if err != nil {
			return UsefulnessResponse{}, err
		}
	}

	return ParseUsefulnessResponse(result)
}

func init() {
	Register("codex-cli", NewCodexCLIProvider)
}
