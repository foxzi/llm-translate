package translator

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/foxzi/llm-translate/internal/config"
	"github.com/foxzi/llm-translate/internal/provider"
	"github.com/foxzi/llm-translate/internal/proxy"
	"github.com/foxzi/llm-translate/internal/validator"
)

type Translator struct {
	config   *config.Config
	provider provider.Provider
	verbose  bool
	client   *http.Client
}

type TranslateRequest struct {
	Text           string
	SourceLang     string
	TargetLang     string
	Style          string
	Context        string
	Glossary       []config.GlossaryEntry
	Temperature    float64
	MaxTokens      int
	PreserveFormat bool
	StrongMode     bool
	StrongRetries  int
}

type TranslateResponse struct {
	Text         string
	DetectedLang string
	TokensUsed   int
}

func New(cfg *config.Config, verbose bool) *Translator {
	return &Translator{
		config:  cfg,
		verbose: verbose,
	}
}

func (t *Translator) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	client, err := t.createHTTPClient()
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	t.client = client

	providerCfg, ok := t.config.Providers[t.config.DefaultProvider]
	if !ok {
		return TranslateResponse{}, fmt.Errorf("provider %s not configured", t.config.DefaultProvider)
	}

	p, err := provider.Get(t.config.DefaultProvider, providerCfg, t.client)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to initialize provider: %w", err)
	}
	t.provider = p

	text := req.Text
	if len(req.Glossary) > 0 {
		text = applyGlossaryPreProcessing(text, req.Glossary)
	}

	chunks := t.splitIntoChunks(text, t.config.Settings.ChunkSize)
	if t.verbose && len(chunks) > 1 {
		t.logInfo("Text split into %d chunks", len(chunks))
	}

	var results []string
	totalTokens := 0

	for i, chunk := range chunks {
		if t.verbose && len(chunks) > 1 {
			t.logInfo("Translating chunk %d/%d...", i+1, len(chunks))
		}

		providerReq := provider.TranslateRequest{
			Text:           chunk,
			SourceLang:     req.SourceLang,
			TargetLang:     req.TargetLang,
			Style:          req.Style,
			Context:        req.Context,
			Glossary:       req.Glossary,
			Temperature:    req.Temperature,
			MaxTokens:      req.MaxTokens,
			PreserveFormat: req.PreserveFormat,
		}

		resp, err := t.translateWithRetry(ctx, providerReq)
		if err != nil {
			return TranslateResponse{}, fmt.Errorf("failed to translate chunk %d: %w", i+1, err)
		}

		translatedChunk := resp.Text

		if req.StrongMode {
			validated, err := t.validateTranslation(ctx, chunk, translatedChunk, req)
			if err != nil {
				if t.verbose {
					t.logWarn("Strong validation failed for chunk %d: %v", i+1, err)
				}

				retrySuccess := false
				for retry := 1; retry <= req.StrongRetries; retry++ {
					if t.verbose {
						t.logInfo("Retry %d/%d: requesting re-translation...", retry, req.StrongRetries)
					}

					retryReq := providerReq
					retryReq.Context = fmt.Sprintf(
						"Previous translation contained untranslated text. Please ensure all text is properly translated to %s. %s",
						req.TargetLang, req.Context,
					)

					retryResp, retryErr := t.translateWithRetry(ctx, retryReq)
					if retryErr != nil {
						continue
					}

					retryValidated, validateErr := t.validateTranslation(ctx, chunk, retryResp.Text, req)
					if validateErr == nil {
						translatedChunk = retryValidated
						retrySuccess = true
						if t.verbose {
							t.logInfo("Strong validation passed")
						}
						break
					}
				}

				if !retrySuccess {
					return TranslateResponse{}, fmt.Errorf("strong validation failed after %d retries", req.StrongRetries)
				}
			} else {
				translatedChunk = validated
			}
		}

		results = append(results, translatedChunk)
		totalTokens += resp.TokensUsed
	}

	finalText := strings.Join(results, "\n\n")

	if len(req.Glossary) > 0 {
		finalText = applyGlossaryPostProcessing(finalText, req.Glossary)
	}

	return TranslateResponse{
		Text:       finalText,
		TokensUsed: totalTokens,
	}, nil
}

func (t *Translator) ensureProvider() error {
	if t.provider != nil {
		return nil
	}

	client, err := t.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %w", err)
	}
	t.client = client

	providerCfg, ok := t.config.Providers[t.config.DefaultProvider]
	if !ok {
		return fmt.Errorf("provider %s not configured", t.config.DefaultProvider)
	}

	p, err := provider.Get(t.config.DefaultProvider, providerCfg, t.client)
	if err != nil {
		return fmt.Errorf("failed to initialize provider: %w", err)
	}
	t.provider = p
	return nil
}

func (t *Translator) AnalyzeSentiment(ctx context.Context, text string) (provider.SentimentResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.SentimentResponse{}, err
	}
	return t.provider.AnalyzeSentiment(ctx, text)
}

func (t *Translator) ExtractTags(ctx context.Context, text string, count int) (provider.TagsResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.TagsResponse{}, err
	}
	return t.provider.ExtractTags(ctx, text, count)
}

func (t *Translator) Classify(ctx context.Context, text string) (provider.ClassifyResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.ClassifyResponse{}, err
	}
	return t.provider.Classify(ctx, text)
}

func (t *Translator) AnalyzeEmotions(ctx context.Context, text string) (provider.EmotionsResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.EmotionsResponse{}, err
	}
	return t.provider.AnalyzeEmotions(ctx, text)
}

func (t *Translator) AnalyzeFactuality(ctx context.Context, text string) (provider.FactualityResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.FactualityResponse{}, err
	}
	return t.provider.AnalyzeFactuality(ctx, text)
}

func (t *Translator) AnalyzeImpact(ctx context.Context, text string) (provider.ImpactResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.ImpactResponse{}, err
	}
	return t.provider.AnalyzeImpact(ctx, text)
}

func (t *Translator) AnalyzeSensationalism(ctx context.Context, text string) (provider.SensationalismResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.SensationalismResponse{}, err
	}
	return t.provider.AnalyzeSensationalism(ctx, text)
}

func (t *Translator) ExtractEntities(ctx context.Context, text string) (provider.EntitiesResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.EntitiesResponse{}, err
	}
	return t.provider.ExtractEntities(ctx, text)
}

func (t *Translator) ExtractEvents(ctx context.Context, text string) (provider.EventsResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.EventsResponse{}, err
	}
	return t.provider.ExtractEvents(ctx, text)
}

func (t *Translator) AnalyzeUsefulness(ctx context.Context, text string) (provider.UsefulnessResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.UsefulnessResponse{}, err
	}
	return t.provider.AnalyzeUsefulness(ctx, text)
}

func (t *Translator) AnalyzeTimeFocus(ctx context.Context, text string) (provider.TimeFocusResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.TimeFocusResponse{}, err
	}
	return t.provider.AnalyzeTimeFocus(ctx, text)
}

func (t *Translator) AnalyzeCombined(ctx context.Context, req provider.CombinedAnalysisRequest) (provider.CombinedAnalysisResponse, error) {
	if err := t.ensureProvider(); err != nil {
		return provider.CombinedAnalysisResponse{}, err
	}
	return t.provider.AnalyzeCombined(ctx, req)
}

func (t *Translator) translateWithRetry(ctx context.Context, req provider.TranslateRequest) (provider.TranslateResponse, error) {
	var lastErr error
	retryCount := t.config.Settings.RetryCount
	if retryCount == 0 {
		retryCount = 3
	}

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			delay := time.Duration(t.config.Settings.RetryDelay) * time.Second
			if delay == 0 {
				delay = time.Second
			}
			delay = delay * time.Duration(1<<(attempt-1))

			if t.verbose {
				t.logInfo("Retrying after %v (attempt %d/%d)...", delay, attempt, retryCount)
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return provider.TranslateResponse{}, ctx.Err()
			}
		}

		resp, err := t.provider.Translate(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if !isRetryableError(err) {
			return provider.TranslateResponse{}, err
		}

		if t.verbose {
			t.logWarn("Request failed: %v", err)
		}
	}

	return provider.TranslateResponse{}, fmt.Errorf("failed after %d retries: %w", retryCount, lastErr)
}

func (t *Translator) createHTTPClient() (*http.Client, error) {
	providerCfg, ok := t.config.Providers[t.config.DefaultProvider]
	if !ok {
		providerCfg = config.ProviderConfig{}
	}

	proxyCfg := providerCfg.Proxy
	if proxyCfg.URL == "" {
		proxyCfg = t.config.Proxy
	}

	if proxyCfg.URL != "" {
		return proxy.NewHTTPClient(proxyCfg, t.config.Settings.Timeout)
	}

	return &http.Client{
		Timeout: time.Duration(t.config.Settings.Timeout) * time.Second,
	}, nil
}

func (t *Translator) validateTranslation(ctx context.Context, original, translated string, req TranslateRequest) (string, error) {
	if !req.StrongMode {
		return translated, nil
	}

	v := validator.New(t.config.StrongValidation)
	isValid, problematicFragments := v.Validate(translated, req.SourceLang, req.TargetLang)

	if !isValid && len(problematicFragments) > 0 {
		return "", fmt.Errorf("found source language text: %v", strings.Join(problematicFragments, ", "))
	}

	return translated, nil
}

func (t *Translator) splitIntoChunks(text string, chunkSize int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	paragraphs := strings.Split(text, "\n\n")

	currentChunk := ""
	for _, paragraph := range paragraphs {
		if len(currentChunk)+len(paragraph)+2 > chunkSize {
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
				currentChunk = ""
			}

			if len(paragraph) > chunkSize {
				sentences := splitIntoSentences(paragraph)
				for _, sentence := range sentences {
					if len(currentChunk)+len(sentence)+1 > chunkSize {
						if currentChunk != "" {
							chunks = append(chunks, strings.TrimSpace(currentChunk))
							currentChunk = ""
						}

						if len(sentence) > chunkSize {
							words := strings.Fields(sentence)
							for _, word := range words {
								if len(currentChunk)+len(word)+1 > chunkSize {
									chunks = append(chunks, strings.TrimSpace(currentChunk))
									currentChunk = word
								} else {
									if currentChunk != "" {
										currentChunk += " "
									}
									currentChunk += word
								}
							}
						} else {
							currentChunk = sentence
						}
					} else {
						if currentChunk != "" {
							currentChunk += " "
						}
						currentChunk += sentence
					}
				}
			} else {
				currentChunk = paragraph
			}
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += paragraph
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

func splitIntoSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for i, r := range text {
		current.WriteRune(r)

		if r == '.' || r == '!' || r == '?' {
			if i < len(text)-1 && text[i+1] == ' ' {
				sentences = append(sentences, strings.TrimSpace(current.String()))
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		sentences = append(sentences, strings.TrimSpace(current.String()))
	}

	return sentences
}

// applyGlossaryPreProcessing is a no-op: glossary terms are injected into
// the LLM prompt via provider.buildPrompt, so no text-level pre-processing
// is needed.
func applyGlossaryPreProcessing(text string, glossary []config.GlossaryEntry) string {
	return text
}

// applyGlossaryPostProcessing verifies that glossary terms were translated
// correctly. If a source term still appears in the translated text, it gets
// replaced with the expected target translation.
func applyGlossaryPostProcessing(text string, glossary []config.GlossaryEntry) string {
	for _, entry := range glossary {
		source, target := glossarySourceTarget(entry)
		if source == "" || target == "" {
			continue
		}

		if entry.CaseSensitive {
			if strings.Contains(text, source) {
				text = strings.ReplaceAll(text, source, target)
			}
		} else {
			text = replaceAllCaseInsensitive(text, source, target)
		}
	}
	return text
}

// glossarySourceTarget resolves the source/target pair from a GlossaryEntry,
// handling both Source/Target and Term/Translation field conventions.
func glossarySourceTarget(entry config.GlossaryEntry) (string, string) {
	source := entry.Source
	target := entry.Target
	if source == "" {
		source = entry.Term
	}
	if target == "" {
		target = entry.Translation
	}
	return source, target
}

// replaceAllCaseInsensitive replaces all occurrences of old in text
// with new_, case-insensitively, preserving surrounding text.
func replaceAllCaseInsensitive(text, old, new_ string) string {
	if old == "" {
		return text
	}
	lower := strings.ToLower(text)
	oldLower := strings.ToLower(old)

	var result strings.Builder
	i := 0
	for {
		idx := strings.Index(lower[i:], oldLower)
		if idx == -1 {
			result.WriteString(text[i:])
			break
		}
		result.WriteString(text[i : i+idx])
		result.WriteString(new_)
		i += idx + len(old)
	}
	return result.String()
}

func isRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "500")
}

func (t *Translator) logInfo(format string, args ...interface{}) {
	if t.verbose {
		fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
	}
}

func (t *Translator) logWarn(format string, args ...interface{}) {
	if t.verbose {
		fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
	}
}
