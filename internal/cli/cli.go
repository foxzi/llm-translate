package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"github.com/user/llm-translate/internal/config"
	"github.com/user/llm-translate/internal/translator"
)

var (
	Version = "1.0.0"
	
	inputFile        string
	outputFile       string
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
			return fmt.Errorf("no input provided. Use -i <file> or pipe text to stdin")
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

	if verbose {
		logInfo("Provider: %s, Model: %s", cfg.DefaultProvider, getModelForProvider(cfg))
		logInfo("Source language: %s, Target language: %s", sourceLang, targetLang)
		logInfo("Input size: %d characters", len(inputText))
	}

	if dryRun {
		logInfo("Dry run mode - showing request configuration:")
		logInfo("Provider: %s", cfg.DefaultProvider)
		logInfo("Model: %s", getModelForProvider(cfg))
		logInfo("Source: %s -> Target: %s", sourceLang, targetLang)
		logInfo("Temperature: %.2f", temperature)
		logInfo("Max tokens: %d", maxTokens)
		logInfo("Text preview (first 200 chars): %s", truncateText(string(inputText), 200))
		return nil
	}

	t := translator.New(cfg, verbose)
	
	req := translator.TranslateRequest{
		Text:           string(inputText),
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

	if _, err := output.Write([]byte(result.Text)); err != nil {
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