package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Usage accumulates tokens reported by OpenAI-compatible APIs for every
// request made with a context returned by WithUsage. It also records the
// cost reported by the API (OpenRouter usage.cost) and the last HTTP status.
type Usage struct {
	PromptTokens     atomic.Int64
	CompletionTokens atomic.Int64

	mu         sync.Mutex
	cost       float64
	hasCost    bool
	lastStatus int
	retryAfter int
}

// Cost returns the summed usage.cost of all responses; ok is false when no
// response reported a cost.
func (u *Usage) Cost() (cost float64, ok bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cost, u.hasCost
}

// RateLimited reports whether the last response had status 429, and its
// Retry-After in seconds (0 if unknown).
func (u *Usage) RateLimited() (limited bool, retryAfter int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastStatus == http.StatusTooManyRequests, u.retryAfter
}

type usageKey struct{}

// WithUsage returns a context whose LLM requests add their token usage to u.
func WithUsage(ctx context.Context, u *Usage) context.Context {
	return context.WithValue(ctx, usageKey{}, u)
}

// usageTransport reads the "usage" object of JSON responses and adds it to
// the Usage stored in the request context.
type usageTransport struct {
	base http.RoundTripper
}

func (t usageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	u, _ := req.Context().Value(usageKey{}).(*Usage)
	if err != nil || u == nil {
		return resp, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return resp, err
	}
	u.mu.Lock()
	u.lastStatus = resp.StatusCode
	u.retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	u.mu.Unlock()

	var parsed struct {
		Usage struct {
			PromptTokens     int64    `json:"prompt_tokens"`
			CompletionTokens int64    `json:"completion_tokens"`
			Cost             *float64 `json:"cost"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		u.PromptTokens.Add(parsed.Usage.PromptTokens)
		u.CompletionTokens.Add(parsed.Usage.CompletionTokens)
		if parsed.Usage.Cost != nil {
			u.mu.Lock()
			u.cost += *parsed.Usage.Cost
			u.hasCost = true
			u.mu.Unlock()
		}
	}
	return resp, nil
}

// parseRetryAfter converts a Retry-After header (seconds or HTTP date) to seconds.
func parseRetryAfter(v string) int {
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return int(d.Seconds() + 0.5)
		}
	}
	return 0
}

func withUsageTransport(c *http.Client) *http.Client {
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.Transport = usageTransport{base: base}
	return c
}
