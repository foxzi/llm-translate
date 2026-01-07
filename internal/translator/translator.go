package translator

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/user/llm-translate/internal/config"
	"github.com/user/llm-translate/internal/provider"
	"github.com/user/llm-translate/internal/proxy"
	"github.com/user/llm-translate/internal/validator"
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
				
				for retry := 1; retry <= req.StrongRetries; retry++ {
					if t.verbose {
						t.logInfo("Retry %d/%d: requesting re-translation...", retry, req.StrongRetries)
					}
					
					retryReq := providerReq
					retryReq.Context = fmt.Sprintf(
						"Previous translation contained untranslated text. Please ensure all text is properly translated to %s. %s",
						req.TargetLang, req.Context,
					)
					
					retryResp, err := t.translateWithRetry(ctx, retryReq)
					if err != nil {
						continue
					}
					
					validated, err = t.validateTranslation(ctx, chunk, retryResp.Text, req)
					if err == nil {
						translatedChunk = validated
						if t.verbose {
							t.logInfo("Strong validation passed")
						}
						break
					}
				}
				
				if err != nil {
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
		return proxy.NewHTTPClient(proxyCfg)
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

func applyGlossaryPreProcessing(text string, glossary []config.GlossaryEntry) string {
	return text
}

func applyGlossaryPostProcessing(text string, glossary []config.GlossaryEntry) string {
	return text
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