package cli

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/foxzi/llm-translate/internal/config"
	"github.com/foxzi/llm-translate/internal/translator"
)

const maxRequestBody = 10 << 20

// Error codes returned in apiError.Code.
const (
	codeInvalidRequest = "invalid_request"
	codeUnauthorized   = "unauthorized"
	codeRateLimited    = "rate_limited"
	codeProviderError  = "provider_error"
	codeTimeout        = "timeout"
)

// apiRequest is the /v1/translate body. Only text is required; the other
// fields are optional and default to the server configuration.
type apiRequest struct {
	Text  string `json:"text"`
	From  string `json:"from"`
	To    string `json:"to"`
	Title string `json:"title"`
	// Translate=false skips translation and only analyzes the text.
	Translate *bool `json:"translate"`
	// Analysis, if set, replaces the analyses enabled in the config.
	Analysis *[]string `json:"analysis"`
	Tags     *int      `json:"tags"`
	Summary  *int      `json:"summary"`
}

type apiResponse struct {
	Text             string                 `json:"text"`
	Title            string                 `json:"title,omitempty"`
	Summary          string                 `json:"summary,omitempty"`
	DetectedLanguage string                 `json:"detected_language,omitempty"`
	Analysis         map[string]interface{} `json:"analysis,omitempty"`
	AnalysisErrors   []string               `json:"analysis_errors,omitempty"`
	Usage            apiUsage               `json:"usage"`
}

type apiUsage struct {
	PromptTokens     int64    `json:"prompt_tokens"`
	CompletionTokens int64    `json:"completion_tokens"`
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	Cost             *float64 `json:"cost,omitempty"`
}

type apiError struct {
	Error      string `json:"error"`
	Code       string `json:"code,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

// defaultShutdownTimeout bounds the graceful shutdown when request_timeout is not set.
const defaultShutdownTimeout = 60 * time.Second

// runServer serves translation and analysis over HTTP until ctx is cancelled,
// then waits for in-flight requests. Analyses enabled in cfg.Settings are
// applied to every request.
func runServer(ctx context.Context, cfg *config.Config, glossary []config.GlossaryEntry) error {
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           newServerHandler(cfg, glossary),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	logInfo("API server listening on %s (provider: %s, model: %s)", listenAddr, cfg.DefaultProvider, getModelForProvider(cfg))
	if cfg.Settings.APIToken == "" {
		logWarn("API token is not set, the server accepts requests without authorization")
	}
	timeout := defaultShutdownTimeout
	if cfg.Settings.RequestTimeout > 0 {
		timeout = time.Duration(cfg.Settings.RequestTimeout) * time.Second
	}
	return serve(ctx, srv, ln, timeout)
}

// serve runs srv on ln until ctx is cancelled and returns after Shutdown has
// finished, i.e. after the in-flight requests are done or timeout has passed.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	shutdownErr := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		shutdownErr <- srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-shutdownErr
}

func newServerHandler(cfg *config.Config, glossary []config.GlossaryEntry) http.Handler {
	var slots chan struct{}
	if cfg.Settings.MaxConcurrent > 0 {
		slots = make(chan struct{}, cfg.Settings.MaxConcurrent)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
	})
	mux.HandleFunc("POST /v1/translate", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, cfg.Settings.APIToken) {
			writeJSON(w, http.StatusUnauthorized, apiError{Error: "unauthorized", Code: codeUnauthorized})
			return
		}
		// The body is read before a slot is taken so that slow clients do not hold one.
		in, ok := readRequest(w, r, cfg)
		if !ok {
			return
		}
		ctx := r.Context()
		if cfg.Settings.RequestTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.Settings.RequestTimeout)*time.Second)
			defer cancel()
		}
		if slots != nil {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				writeProcessError(w, ctx, ctx.Err(), nil)
				return
			}
		}
		handleTranslate(w, r.WithContext(ctx), in, cfg, glossary)
	})
	return mux
}

// readRequest decodes and validates the request body. It writes the error
// response and returns false if the request is invalid.
func readRequest(w http.ResponseWriter, r *http.Request, cfg *config.Config) (apiRequest, bool) {
	var in apiRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON: " + err.Error(), Code: codeInvalidRequest})
		return in, false
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "text is required", Code: codeInvalidRequest})
		return in, false
	}
	if n := utf8.RuneCountInString(in.Text); cfg.Settings.MaxTextLength > 0 && n > cfg.Settings.MaxTextLength {
		writeJSON(w, http.StatusBadRequest, apiError{
			Error: fmt.Sprintf("text is too long: %d characters, limit %d", n, cfg.Settings.MaxTextLength),
			Code:  codeInvalidRequest,
		})
		return in, false
	}
	return in, true
}

func handleTranslate(w http.ResponseWriter, r *http.Request, in apiRequest, cfg *config.Config, glossary []config.GlossaryEntry) {
	// Per-request settings; Config is copied by value, its maps are only read.
	reqCfg := *cfg
	if err := applyRequestAnalyses(&reqCfg.Settings, in); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: err.Error(), Code: codeInvalidRequest})
		return
	}

	from := in.From
	if from == "" {
		from = sourceLang
	}
	to := in.To
	if to == "" {
		to = cfg.DefaultTargetLanguage
	}
	if to == "" {
		to = targetLang
	}

	req := translator.TranslateRequest{
		Text:           in.Text,
		SourceLang:     from,
		TargetLang:     to,
		Style:          style,
		Context:        contextStr,
		Glossary:       glossary,
		Temperature:    cfg.Settings.Temperature,
		MaxTokens:      cfg.Settings.MaxTokens,
		PreserveFormat: cfg.Settings.PreserveFormat,
		StrongMode:     strongMode,
		StrongRetries:  strongRetries,
	}
	doTranslate := in.Translate == nil || *in.Translate
	if cfg.Settings.SkipSameLanguage && strings.EqualFold(from, to) {
		doTranslate = false
	}

	var usage translator.Usage
	out, err := processRequest(translator.WithUsage(r.Context(), &usage), &reqCfg, req, in.Title, doTranslate)
	if err != nil {
		logError("translate %s->%s: %v", from, to, err)
		writeProcessError(w, r.Context(), err, &usage)
		return
	}

	out.Usage = apiUsage{
		PromptTokens:     usage.PromptTokens.Load(),
		CompletionTokens: usage.CompletionTokens.Load(),
		Provider:         cfg.DefaultProvider,
		Model:            getModelForProvider(cfg),
	}
	if cost, ok := usage.Cost(); ok {
		out.Usage.Cost = &cost
	}
	writeJSON(w, http.StatusOK, out)
}

// analysisNames are the names accepted in apiRequest.Analysis and returned
// in analysis_errors.
var analysisNames = []string{
	"sentiment", "tags", "classify", "emotions", "factuality", "impact",
	"sensationalism", "entities", "events", "usefulness", "time_focus", "ad_detect",
}

// defaultTagsCount is used when "tags" is requested without a count and the
// config has none.
const defaultTagsCount = 5

// applyRequestAnalyses overrides the analysis settings with those of the request.
func applyRequestAnalyses(s *config.Settings, in apiRequest) error {
	if in.Analysis != nil {
		set := map[string]bool{}
		for _, name := range *in.Analysis {
			name = strings.ToLower(strings.TrimSpace(name))
			known := false
			for _, n := range analysisNames {
				known = known || n == name
			}
			if !known {
				return fmt.Errorf("unknown analysis %q, allowed: %s", name, strings.Join(analysisNames, ", "))
			}
			set[name] = true
		}
		tags := s.TagsCount
		if tags <= 0 {
			tags = defaultTagsCount
		}
		s.Sentiment, s.Classify, s.Emotions = set["sentiment"], set["classify"], set["emotions"]
		s.Factuality, s.Impact, s.Sensationalism = set["factuality"], set["impact"], set["sensationalism"]
		s.Entities, s.Events, s.Usefulness = set["entities"], set["events"], set["usefulness"]
		s.TimeFocus, s.AdDetect = set["time_focus"], set["ad_detect"]
		s.TagsCount = 0
		if set["tags"] {
			s.TagsCount = tags
		}
	}
	if in.Tags != nil {
		if *in.Tags < 0 {
			return errors.New("tags must not be negative")
		}
		s.TagsCount = *in.Tags
	}
	if in.Summary != nil {
		if *in.Summary < 0 {
			return errors.New("summary must not be negative")
		}
		s.Summary = *in.Summary
	}
	return nil
}

// processRequest translates the title and text and runs the analyses enabled
// in cfg. Without a title, with translation on and no new options it behaves
// like translateAndAnalyze. The language is detected whenever the original
// text is analyzed and the source language is not given.
func processRequest(ctx context.Context, cfg *config.Config, req translator.TranslateRequest, title string, doTranslate bool) (apiResponse, error) {
	var out apiResponse
	detect := req.SourceLang == "" || req.SourceLang == "auto"

	analyze := func(text, lang string, detect bool) {
		// A Translator is not safe for concurrent use, so each goroutine gets its own.
		fm, failed := runAnalysis(ctx, translator.New(cfg, verbose), cfg, joinTitle(title, text), lang, detect, verbose)
		out.Summary, _ = fm["summary"].(string)
		out.DetectedLanguage, _ = fm["language"].(string)
		delete(fm, "summary")
		delete(fm, "language")
		if cfg.Settings.StrictAnalysis {
			fm = sanitizeAnalysis(fm)
		}
		out.Analysis = fm
		out.AnalysisErrors = failed
	}

	if !doTranslate {
		analyze(req.Text, req.TargetLang, detect)
		out.Text, out.Title = req.Text, title
		return out, nil
	}

	var wg sync.WaitGroup
	var titleErr error
	if title != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			titleReq := req
			titleReq.Text = title
			var res translator.TranslateResponse
			res, titleErr = translator.New(cfg, verbose).Translate(ctx, titleReq)
			out.Title = res.Text
		}()
	}

	parallel := cfg.Settings.ParallelAnalysis
	if parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			analyze(req.Text, req.TargetLang, detect)
		}()
	}
	res, err := translator.New(cfg, verbose).Translate(ctx, req)
	wg.Wait()
	if err == nil {
		err = titleErr
	}
	if err != nil {
		return out, err
	}
	out.Text = res.Text
	if !parallel {
		// The translation is analyzed; title is now the translated one.
		title = out.Title
		analyze(res.Text, "", false)
	}
	return out, nil
}

// joinTitle prepends title to text as a paragraph for analysis.
func joinTitle(title, text string) string {
	if title == "" {
		return text
	}
	return title + "\n\n" + text
}

var urlPattern = regexp.MustCompile(`https?://\S+`)

// publicMessage is err's message without URLs, so that provider base URLs
// and keys in query strings do not reach API clients. The logs keep the full error.
func publicMessage(err error) string {
	return urlPattern.ReplaceAllString(err.Error(), "<url>")
}

// writeProcessError maps a processing error to an HTTP status and code.
// Errors other than the request timeout and a provider 429 stay 502.
func writeProcessError(w http.ResponseWriter, ctx context.Context, err error, usage *translator.Usage) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		writeJSON(w, http.StatusGatewayTimeout, apiError{Error: "request timeout: " + publicMessage(err), Code: codeTimeout})
		return
	}
	if usage != nil {
		if limited, after := usage.RateLimited(); limited {
			if after > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(after))
			}
			writeJSON(w, http.StatusTooManyRequests, apiError{Error: publicMessage(err), Code: codeRateLimited, RetryAfter: after})
			return
		}
	}
	writeJSON(w, http.StatusBadGateway, apiError{Error: publicMessage(err), Code: codeProviderError})
}

// authorized reports whether r carries "Authorization: Bearer <token>".
// An empty token disables authorization.
func authorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
