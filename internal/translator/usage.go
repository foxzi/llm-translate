package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
)

// Usage accumulates tokens reported by OpenAI-compatible APIs for every
// request made with a context returned by WithUsage.
type Usage struct {
	PromptTokens     atomic.Int64
	CompletionTokens atomic.Int64
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
	var parsed struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		u.PromptTokens.Add(parsed.Usage.PromptTokens)
		u.CompletionTokens.Add(parsed.Usage.CompletionTokens)
	}
	return resp, nil
}

func withUsageTransport(c *http.Client) *http.Client {
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.Transport = usageTransport{base: base}
	return c
}
