package translator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/foxzi/llm-translate/internal/config"
	"github.com/foxzi/llm-translate/internal/provider"
)

func TestIsRetryableError(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"429", context.Background(), &provider.HTTPError{StatusCode: 429}, true},
		{"500", context.Background(), &provider.HTTPError{StatusCode: 500}, true},
		{"502", context.Background(), &provider.HTTPError{StatusCode: 502}, true},
		{"503", context.Background(), &provider.HTTPError{StatusCode: 503}, true},
		{"504", context.Background(), &provider.HTTPError{StatusCode: 504}, true},
		{"wrapped 503", context.Background(), fmt.Errorf("x: %w", &provider.HTTPError{StatusCode: 503}), true},
		{"400", context.Background(), &provider.HTTPError{StatusCode: 400}, false},
		{"401", context.Background(), &provider.HTTPError{StatusCode: 401}, false},
		{"400 with timeout text", context.Background(), &provider.HTTPError{StatusCode: 400, Message: "timeout 500"}, false},
		{"url error", context.Background(), &url.Error{Op: "Post", URL: "http://x", Err: errors.New("refused")}, true},
		{"unexpected EOF", context.Background(), fmt.Errorf("failed to read response: %w", io.ErrUnexpectedEOF), true},
		{"client timeout", context.Background(), fmt.Errorf("failed to send request: %w", context.DeadlineExceeded), true},
		{"cancelled parent", canceled, &provider.HTTPError{StatusCode: 503}, false},
		{"cancelled parent timeout", canceled, context.DeadlineExceeded, false},
		{"cli substring", context.Background(), errors.New("codex: connection reset"), true},
		{"cli other", context.Background(), errors.New("invalid argument"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableError(tc.ctx, tc.err); got != tc.want {
				t.Errorf("isRetryableError = %v, want %v", got, tc.want)
			}
		})
	}
}

type flakyProvider struct {
	provider.Provider
	errs  []error
	calls int
}

func (f *flakyProvider) Translate(ctx context.Context, req provider.TranslateRequest) (provider.TranslateResponse, error) {
	f.calls++
	if f.calls <= len(f.errs) {
		return provider.TranslateResponse{}, f.errs[f.calls-1]
	}
	return provider.TranslateResponse{Text: "ok"}, nil
}

func newTestTranslator(p provider.Provider, retryCount int) *Translator {
	cfg := config.DefaultConfig()
	cfg.Settings.RetryCount = retryCount
	return &Translator{config: cfg, provider: p}
}

func TestRetryCountZeroMeansNoRetries(t *testing.T) {
	f := &flakyProvider{errs: []error{&provider.HTTPError{StatusCode: 503}}}
	_, err := newTestTranslator(f, 0).translateWithRetry(context.Background(), provider.TranslateRequest{})
	if err == nil || f.calls != 1 {
		t.Errorf("err = %v, calls = %d, want error after 1 call", err, f.calls)
	}
}

func TestRetryNonRetryableStops(t *testing.T) {
	f := &flakyProvider{errs: []error{&provider.HTTPError{StatusCode: 400}}}
	_, err := newTestTranslator(f, 3).translateWithRetry(context.Background(), provider.TranslateRequest{})
	if err == nil || f.calls != 1 {
		t.Errorf("err = %v, calls = %d, want error after 1 call", err, f.calls)
	}
}

func TestRetryRecovers(t *testing.T) {
	f := &flakyProvider{errs: []error{&provider.HTTPError{StatusCode: 503}}}
	resp, err := newTestTranslator(f, 3).translateWithRetry(context.Background(), provider.TranslateRequest{})
	if err != nil || resp.Text != "ok" || f.calls != 2 {
		t.Errorf("resp = %q, err = %v, calls = %d", resp.Text, err, f.calls)
	}
}

func TestRetryStopsWaitingOnCancel(t *testing.T) {
	f := &flakyProvider{errs: []error{&provider.HTTPError{StatusCode: 503}, &provider.HTTPError{StatusCode: 503}}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := newTestTranslator(f, 3).translateWithRetry(ctx, provider.TranslateRequest{})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 500*time.Millisecond {
		t.Errorf("err = %v after %v", err, time.Since(start))
	}
}

func TestRetryDelay(t *testing.T) {
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second} {
		got := retryDelay(time.Second, attempt, 0)
		if got < want || got > want+want/4 {
			t.Errorf("attempt %d: delay %v, want %v..+25%%", attempt, got, want)
		}
	}
	if got := retryDelay(time.Second, 30, 0); got != maxBackoff {
		t.Errorf("large attempt: delay %v, want cap %v", got, maxBackoff)
	}
	if got := retryDelay(time.Second, 1, 5*time.Second); got != 5*time.Second {
		t.Errorf("Retry-After 5s: delay %v", got)
	}
	if got := retryDelay(time.Second, 1, time.Minute); got > 2*time.Second {
		t.Errorf("Retry-After above max must be ignored: delay %v", got)
	}
}

func (f *flakyProvider) AnalyzeSentiment(ctx context.Context, text string) (provider.SentimentResponse, error) {
	f.calls++
	if f.calls <= len(f.errs) {
		return provider.SentimentResponse{}, f.errs[f.calls-1]
	}
	return provider.SentimentResponse{Sentiment: "positive"}, nil
}

func TestAnalysisIsRetried(t *testing.T) {
	f := &flakyProvider{errs: []error{&provider.HTTPError{StatusCode: 502}}}
	resp, err := newTestTranslator(f, 3).AnalyzeSentiment(context.Background(), "x")
	if err != nil || resp.Sentiment != "positive" || f.calls != 2 {
		t.Errorf("resp = %+v, err = %v, calls = %d", resp, err, f.calls)
	}

	f = &flakyProvider{errs: []error{&provider.HTTPError{StatusCode: 401}}}
	if _, err := newTestTranslator(f, 3).AnalyzeSentiment(context.Background(), "x"); err == nil || f.calls != 1 {
		t.Errorf("non-retryable: err = %v, calls = %d", err, f.calls)
	}
}
