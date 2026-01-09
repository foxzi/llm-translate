package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"github.com/user/llm-translate/internal/config"
	"github.com/user/llm-translate/internal/translator"
)

var (
	Version = "dev"

	inputFile        string
	outputFile       string
	inputDir         string
	extensions       string
	outSuffix        string
	outPrefix        string
	sourceLang       string
	targetLang       string
	provider         string
	model            string
	configPath       string
	apiKey           string
	baseURL          string
	temperature      float64
	maxTokens        int
	timeout          int
	chunkSize        int
	contextStr       string
	style            string
	glossaryFile     string
	preserveFormat   bool
	strongMode       bool
	strongRetries    int
	verbose          bool
	quiet            bool
	dryRun           bool
	proxyURL         string
	proxyAuth        string
	noProxy          bool
	sentiment        bool
	tagsCount        int
	classify         bool
	emotions         bool
	factuality       bool
)

func Execute(ctx context.Context) error {
	rootCmd := &cobra.Command{
		Use:   "llm-translate",
		Short: "Translate text using LLM APIs",
		Long:  `A CLI tool for translating text between languages using various LLM providers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTranslate(ctx)
		},
	}

	rootCmd.Flags().StringVarP(&inputFile, "input", "i", "", "Input file (default: stdin)")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	rootCmd.Flags().StringVarP(&inputDir, "dir", "d", "", "Input directory for recursive translation")
	rootCmd.Flags().StringVar(&extensions, "ext", ".md,.txt", "File extensions to translate (comma-separated)")
	rootCmd.Flags().StringVar(&outSuffix, "suffix", "", "Output file suffix (e.g., _ru)")
	rootCmd.Flags().StringVar(&outPrefix, "prefix", "", "Output file prefix (e.g., ru_)")
	rootCmd.Flags().StringVarP(&sourceLang, "from", "f", "auto", "Source language")
	rootCmd.Flags().StringVarP(&targetLang, "to", "t", "en", "Target language")
	rootCmd.Flags().StringVarP(&provider, "provider", "p", "", "LLM provider")
	rootCmd.Flags().StringVarP(&model, "model", "m", "", "Model to use")
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "", "Config file path")
	rootCmd.Flags().StringVarP(&apiKey, "api-key", "k", "", "API key (overrides config)")
	rootCmd.Flags().StringVarP(&baseURL, "base-url", "u", "", "Base URL for API")
	rootCmd.Flags().Float64Var(&temperature, "temperature", 0.3, "Generation temperature")
	rootCmd.Flags().IntVar(&maxTokens, "max-tokens", 4096, "Maximum tokens in response")
	rootCmd.Flags().IntVar(&timeout, "timeout", 60, "Request timeout in seconds")
	rootCmd.Flags().IntVar(&chunkSize, "chunk-size", 3000, "Chunk size for long texts")
	rootCmd.Flags().StringVar(&contextStr, "context", "", "Additional context for translation")
	rootCmd.Flags().StringVar(&style, "style", "", "Translation style: formal, informal, technical, literary")
	rootCmd.Flags().StringVarP(&glossaryFile, "glossary", "g", "", "Glossary file")
	rootCmd.Flags().BoolVar(&preserveFormat, "preserve-format", false, "Preserve formatting (markdown, html)")
	rootCmd.Flags().BoolVarP(&strongMode, "strong", "s", false, "Check for absence of source language in translation")
	rootCmd.Flags().IntVar(&strongRetries, "strong-retries", 3, "Number of retries for strong mode")
	rootCmd.Flags().BoolVar(&verbose, "verbose", false, "Verbose output")
	rootCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet mode (only result)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show request without sending")
	rootCmd.Flags().StringVarP(&proxyURL, "proxy", "x", "", "Proxy server URL")
	rootCmd.Flags().StringVar(&proxyAuth, "proxy-auth", "", "Proxy authentication (user:pass)")
	rootCmd.Flags().BoolVar(&noProxy, "no-proxy", false, "Ignore proxy from config")
	rootCmd.Flags().BoolVar(&sentiment, "sentiment", false, "Analyze sentiment of translated text")
	rootCmd.Flags().IntVar(&tagsCount, "tags", 0, "Extract N tags from translated text (0 to disable)")
	rootCmd.Flags().BoolVar(&classify, "classify", false, "Classify text by topics, scope, and type")
	rootCmd.Flags().BoolVar(&emotions, "emotions", false, "Analyze emotions (fear, anger, hope, uncertainty, optimism, panic)")
	rootCmd.Flags().BoolVar(&factuality, "factuality", false, "Check factuality (confirmed, rumors, forecasts, unsourced)")
	rootCmd.Flags().BoolP("help", "h", false, "Show help")

	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("llm-translate version {{.Version}}\n")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("llm-translate version %s\n", Version)
		},
	}
	rootCmd.AddCommand(versionCmd)

	return rootCmd.ExecuteContext(ctx)
}

func runTranslate(ctx context.Context) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	applyCLIOverrides(cfg)

	// Directory mode
	if inputDir != "" {
		return runDirectoryTranslate(ctx, cfg)
	}

	var input io.Reader = os.Stdin
	if inputFile != "" {
		file, err := os.Open(inputFile)
		if err != nil {
			return fmt.Errorf("failed to open input file: %w", err)
		}
		defer file.Close()
		input = file
	} else {
		// Check if stdin is a terminal (no piped input)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return fmt.Errorf("no input provided. Use -i <file>, -d <dir> or pipe text to stdin")
		}
	}

	var output io.Writer = os.Stdout
	if outputFile != "" {
		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()
		output = file
	}

	inputText, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if len(inputText) == 0 {
		return fmt.Errorf("input is empty")
	}

	// Extract frontmatter if present
	frontmatter, content := extractFrontmatter(string(inputText))

	if verbose {
		logInfo("Provider: %s, Model: %s", cfg.DefaultProvider, getModelForProvider(cfg))
		logInfo("Source language: %s, Target language: %s", sourceLang, targetLang)
		logInfo("Input size: %d characters", len(content))
		if frontmatter != "" {
			logInfo("Frontmatter detected and will be preserved")
		}
	}

	if dryRun {
		logInfo("Dry run mode - showing request configuration:")
		logInfo("Provider: %s", cfg.DefaultProvider)
		logInfo("Model: %s", getModelForProvider(cfg))
		logInfo("Source: %s -> Target: %s", sourceLang, targetLang)
		logInfo("Temperature: %.2f", temperature)
		logInfo("Max tokens: %d", maxTokens)
		logInfo("Text preview (first 200 chars): %s", truncateText(content, 200))
		return nil
	}

	t := translator.New(cfg, verbose)

	req := translator.TranslateRequest{
		Text:           content,
		SourceLang:     sourceLang,
		TargetLang:     targetLang,
		Style:          style,
		Context:        contextStr,
		Temperature:    temperature,
		MaxTokens:      maxTokens,
		PreserveFormat: preserveFormat,
		StrongMode:     strongMode,
		StrongRetries:  strongRetries,
	}

	if glossaryFile != "" {
		glossary, err := loadGlossary(glossaryFile)
		if err != nil {
			return fmt.Errorf("failed to load glossary: %w", err)
		}
		req.Glossary = glossary
	}

	result, err := t.Translate(ctx, req)
	if err != nil {
		return fmt.Errorf("translation failed: %w", err)
	}

	// Collect frontmatter updates
	fmUpdates := make(map[string]interface{})

	if cfg.Settings.Sentiment {
		if verbose {
			logInfo("Analyzing sentiment...")
		}
		sentimentResult, err := t.AnalyzeSentiment(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Sentiment analysis failed: %v", err)
			}
		} else {
			fmUpdates["sentiment"] = sentimentResult.Sentiment
			fmUpdates["sentiment_score"] = sentimentResult.Score
		}
	}

	if cfg.Settings.TagsCount > 0 {
		if verbose {
			logInfo("Extracting %d tags...", cfg.Settings.TagsCount)
		}
		tagsResult, err := t.ExtractTags(ctx, result.Text, cfg.Settings.TagsCount)
		if err != nil {
			if verbose {
				logWarn("Tags extraction failed: %v", err)
			}
		} else {
			fmUpdates["tags"] = tagsResult.Tags
		}
	}

	if cfg.Settings.Classify {
		if verbose {
			logInfo("Classifying text...")
		}
		classifyResult, err := t.Classify(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Classification failed: %v", err)
			}
		} else {
			if len(classifyResult.Topics) > 0 {
				fmUpdates["topics"] = classifyResult.Topics
			}
			if len(classifyResult.Scope) > 0 {
				fmUpdates["scope"] = classifyResult.Scope
			}
			if len(classifyResult.NewsType) > 0 {
				fmUpdates["news_type"] = classifyResult.NewsType
			}
		}
	}

	if cfg.Settings.Emotions {
		if verbose {
			logInfo("Analyzing emotions...")
		}
		emotionsResult, err := t.AnalyzeEmotions(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Emotions analysis failed: %v", err)
			}
		} else {
			if len(emotionsResult.Emotions) > 0 {
				fmUpdates["emotions"] = emotionsResult.Emotions
			}
		}
	}

	if cfg.Settings.Factuality {
		if verbose {
			logInfo("Analyzing factuality...")
		}
		factualityResult, err := t.AnalyzeFactuality(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Factuality analysis failed: %v", err)
			}
		} else {
			fmUpdates["factuality"] = factualityResult.Type
			fmUpdates["factuality_confidence"] = factualityResult.Confidence
			if len(factualityResult.Evidence) > 0 {
				fmUpdates["factuality_evidence"] = factualityResult.Evidence
			}
		}
	}

	// Update frontmatter with analysis results if any
	if len(fmUpdates) > 0 {
		frontmatter = updateFrontmatter(frontmatter, fmUpdates)
	}

	// Combine frontmatter with translated content
	finalOutput := frontmatter + result.Text

	if _, err := output.Write([]byte(finalOutput)); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if verbose {
		logInfo("Translation complete. Output: %d characters", len(result.Text))
		if result.TokensUsed > 0 {
			logInfo("Tokens used: %d", result.TokensUsed)
		}
	}

	return nil
}

func applyCLIOverrides(cfg *config.Config) {
	if targetLang != "" && targetLang != "en" {
		cfg.DefaultTargetLanguage = targetLang
	}
	
	if provider != "" {
		cfg.DefaultProvider = provider
	}
	
	if temperature != 0.3 {
		cfg.Settings.Temperature = temperature
	}
	
	if maxTokens != 4096 {
		cfg.Settings.MaxTokens = maxTokens
	}
	
	if timeout != 60 {
		cfg.Settings.Timeout = timeout
	}
	
	if chunkSize != 3000 {
		cfg.Settings.ChunkSize = chunkSize
	}
	
	if preserveFormat {
		cfg.Settings.PreserveFormat = preserveFormat
	}
	
	if strongMode {
		cfg.StrongValidation.Enabled = strongMode
	}
	
	if strongRetries != 3 {
		cfg.StrongValidation.MaxRetries = strongRetries
	}
	
	if proxyURL != "" {
		cfg.Proxy.URL = proxyURL
	}
	
	if noProxy {
		cfg.Proxy.URL = ""
	}

	if sentiment {
		cfg.Settings.Sentiment = sentiment
	}

	if tagsCount > 0 {
		cfg.Settings.TagsCount = tagsCount
	}

	if classify {
		cfg.Settings.Classify = classify
	}

	if emotions {
		cfg.Settings.Emotions = emotions
	}

	if factuality {
		cfg.Settings.Factuality = factuality
	}

	providerCfg, ok := cfg.Providers[cfg.DefaultProvider]
	if !ok {
		providerCfg = config.ProviderConfig{}
	}
	
	if apiKey != "" {
		providerCfg.APIKey = apiKey
	}
	
	if baseURL != "" {
		providerCfg.BaseURL = baseURL
	}
	
	if model != "" {
		providerCfg.Model = model
	}
	
	cfg.Providers[cfg.DefaultProvider] = providerCfg
}

func getModelForProvider(cfg *config.Config) string {
	if provider, ok := cfg.Providers[cfg.DefaultProvider]; ok {
		return provider.Model
	}
	return "default"
}

func loadGlossary(path string) ([]config.GlossaryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var glossaryFile struct {
		Terms []config.GlossaryEntry `yaml:"terms"`
	}
	
	if err := yaml.Unmarshal(data, &glossaryFile); err != nil {
		return nil, err
	}
	
	return glossaryFile.Terms, nil
}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// extractFrontmatter extracts YAML frontmatter from markdown content.
// Returns frontmatter (with delimiters) and remaining content.
// If no frontmatter found, returns empty string and original content.
func extractFrontmatter(text string) (string, string) {
	if !strings.HasPrefix(text, "---") {
		return "", text
	}

	// Find the closing ---
	rest := text[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", text
	}

	// Include the closing --- and newline
	endIdx := 3 + idx + 4 // "---" + content + "\n---"

	// Check if there's a newline after closing ---
	if endIdx < len(text) && text[endIdx] == '\n' {
		endIdx++
	}

	frontmatter := text[:endIdx]
	content := text[endIdx:]

	return frontmatter, content
}

// parseFrontmatter parses frontmatter string into a map.
// Returns nil if parsing fails.
func parseFrontmatter(frontmatter string) map[string]interface{} {
	if frontmatter == "" {
		return nil
	}

	// Remove --- delimiters
	content := strings.TrimPrefix(frontmatter, "---")
	if idx := strings.Index(content, "\n---"); idx != -1 {
		content = content[:idx]
	}
	content = strings.TrimSpace(content)

	if content == "" {
		return make(map[string]interface{})
	}

	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		return nil
	}

	return data
}

// buildFrontmatter creates frontmatter string from a map.
func buildFrontmatter(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return ""
	}

	return "---\n" + string(out) + "---\n"
}

// updateFrontmatter adds or updates fields in frontmatter.
// If frontmatter is empty, creates a new one.
func updateFrontmatter(frontmatter string, updates map[string]interface{}) string {
	data := parseFrontmatter(frontmatter)
	if data == nil {
		data = make(map[string]interface{})
	}

	for k, v := range updates {
		data[k] = v
	}

	return buildFrontmatter(data)
}

func logInfo(format string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
	}
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

func logWarn(format string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
	}
}

func runDirectoryTranslate(ctx context.Context, cfg *config.Config) error {
	// Parse extensions
	extList := parseExtensions(extensions)
	if len(extList) == 0 {
		return fmt.Errorf("no valid extensions specified")
	}

	// Find files to translate
	files, err := findFiles(inputDir, extList)
	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no files found with extensions: %s", extensions)
	}

	// Filter out already translated files
	files = filterTranslatedFiles(files, outSuffix, outPrefix, targetLang)
	if len(files) == 0 {
		logInfo("All files already translated")
		return nil
	}

	logInfo("Found %d files to translate", len(files))

	// Load glossary once
	var glossary []config.GlossaryEntry
	if glossaryFile != "" {
		glossary, err = loadGlossary(glossaryFile)
		if err != nil {
			return fmt.Errorf("failed to load glossary: %w", err)
		}
	}

	t := translator.New(cfg, verbose)

	// Translate each file
	for i, inputPath := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		outputPath := generateOutputPath(inputPath, outSuffix, outPrefix, targetLang)
		logInfo("[%d/%d] %s -> %s", i+1, len(files), filepath.Base(inputPath), filepath.Base(outputPath))

		if err := translateFile(ctx, t, cfg, inputPath, outputPath, glossary); err != nil {
			logError("Failed to translate %s: %v", inputPath, err)
			continue
		}
	}

	logInfo("Translation complete")
	return nil
}

func parseExtensions(ext string) []string {
	parts := strings.Split(ext, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		result = append(result, strings.ToLower(p))
	}
	return result
}

func findFiles(dir string, extList []string) ([]string, error) {
	var files []string
	extMap := make(map[string]bool)
	for _, ext := range extList {
		extMap[ext] = true
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if extMap[ext] {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

func filterTranslatedFiles(files []string, suffix, prefix, lang string) []string {
	// Determine the suffix/prefix pattern to skip
	skipSuffix := suffix
	skipPrefix := prefix
	if skipSuffix == "" && skipPrefix == "" {
		skipSuffix = "_" + lang
	}

	var result []string
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))

		// Skip if file matches translated pattern
		if skipSuffix != "" && strings.HasSuffix(base, skipSuffix) {
			continue
		}
		if skipPrefix != "" && strings.HasPrefix(base, skipPrefix) {
			continue
		}
		result = append(result, f)
	}
	return result
}

func generateOutputPath(inputPath, suffix, prefix, lang string) string {
	dir := filepath.Dir(inputPath)
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), ext)

	// Default: use _<lang> suffix
	if suffix == "" && prefix == "" {
		suffix = "_" + lang
	}

	newName := prefix + base + suffix + ext
	return filepath.Join(dir, newName)
}

func translateFile(ctx context.Context, t *translator.Translator, cfg *config.Config, inputPath, outputPath string, glossary []config.GlossaryEntry) error {
	inputText, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if len(inputText) == 0 {
		return fmt.Errorf("file is empty")
	}

	// Extract frontmatter if present
	frontmatter, content := extractFrontmatter(string(inputText))

	req := translator.TranslateRequest{
		Text:           content,
		SourceLang:     sourceLang,
		TargetLang:     targetLang,
		Style:          style,
		Context:        contextStr,
		Temperature:    temperature,
		MaxTokens:      maxTokens,
		PreserveFormat: preserveFormat,
		StrongMode:     strongMode,
		StrongRetries:  strongRetries,
		Glossary:       glossary,
	}

	result, err := t.Translate(ctx, req)
	if err != nil {
		return err
	}

	// Collect frontmatter updates
	fmUpdates := make(map[string]interface{})

	if cfg.Settings.Sentiment {
		sentimentResult, err := t.AnalyzeSentiment(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Sentiment analysis failed: %v", err)
			}
		} else {
			fmUpdates["sentiment"] = sentimentResult.Sentiment
			fmUpdates["sentiment_score"] = sentimentResult.Score
		}
	}

	if cfg.Settings.TagsCount > 0 {
		tagsResult, err := t.ExtractTags(ctx, result.Text, cfg.Settings.TagsCount)
		if err != nil {
			if verbose {
				logWarn("Tags extraction failed: %v", err)
			}
		} else {
			fmUpdates["tags"] = tagsResult.Tags
		}
	}

	if cfg.Settings.Classify {
		classifyResult, err := t.Classify(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Classification failed: %v", err)
			}
		} else {
			if len(classifyResult.Topics) > 0 {
				fmUpdates["topics"] = classifyResult.Topics
			}
			if len(classifyResult.Scope) > 0 {
				fmUpdates["scope"] = classifyResult.Scope
			}
			if len(classifyResult.NewsType) > 0 {
				fmUpdates["news_type"] = classifyResult.NewsType
			}
		}
	}

	if cfg.Settings.Emotions {
		emotionsResult, err := t.AnalyzeEmotions(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Emotions analysis failed: %v", err)
			}
		} else {
			if len(emotionsResult.Emotions) > 0 {
				fmUpdates["emotions"] = emotionsResult.Emotions
			}
		}
	}

	if cfg.Settings.Factuality {
		factualityResult, err := t.AnalyzeFactuality(ctx, result.Text)
		if err != nil {
			if verbose {
				logWarn("Factuality analysis failed: %v", err)
			}
		} else {
			fmUpdates["factuality"] = factualityResult.Type
			fmUpdates["factuality_confidence"] = factualityResult.Confidence
			if len(factualityResult.Evidence) > 0 {
				fmUpdates["factuality_evidence"] = factualityResult.Evidence
			}
		}
	}

	// Update frontmatter with analysis results if any
	if len(fmUpdates) > 0 {
		frontmatter = updateFrontmatter(frontmatter, fmUpdates)
	}

	// Combine frontmatter with translated content
	finalOutput := frontmatter + result.Text

	if err := os.WriteFile(outputPath, []byte(finalOutput), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}