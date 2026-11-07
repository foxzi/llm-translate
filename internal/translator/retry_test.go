package translator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"testing"

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
