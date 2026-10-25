package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/foxzi/llm-translate/internal/config"
	"github.com/foxzi/llm-translate/internal/translator"
)

const maxRequestBody = 10 << 20

type apiRequest struct {
	Text string `json:"text"`
	From string `json:"from"`
	To   string `json:"to"`
}

type apiResponse struct {
	Text     string                 `json:"text"`
	Analysis map[string]interface{} `json:"analysis,omitempty"`
}

type apiError struct {
	Error string `json:"error"`
}

// runServer serves translation and analysis over HTTP until ctx is cancelled.
// Analyses enabled in cfg.Settings are applied to every request.
func runServer(ctx context.Context, cfg *config.Config, glossary []config.GlossaryEntry) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/translate", func(w http.ResponseWriter, r *http.Request) {
		handleTranslate(w, r, cfg, glossary)
	})

	srv := &http.Server{Addr: listenAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logInfo("API server listening on %s (provider: %s, model: %s)", listenAddr, cfg.DefaultProvider, getModelForProvider(cfg))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func handleTranslate(w http.ResponseWriter, r *http.Request, cfg *config.Config, glossary []config.GlossaryEntry) {
	var in apiRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON: " + err.Error()})
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "text is required"})
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

	// A Translator is not safe for concurrent use, so each request gets its own.
	t := translator.New(cfg, verbose)
	result, analysis, err := translateAndAnalyze(r.Context(), t, cfg, req, verbose)
	if err != nil {
		logError("translate %s->%s: %v", from, to, err)
		writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Text: result.Text, Analysis: analysis})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
