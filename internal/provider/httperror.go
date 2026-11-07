package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// maxErrorBody is the longest body excerpt used as an error message.
const maxErrorBody = 200

// HTTPError is returned by HTTP providers for a non-2xx response.
type HTTPError struct {
	StatusCode int
	Message    string
	// RetryAfter is the delay requested by the Retry-After header, 0 if absent.
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("API error: status %d", e.StatusCode)
	}
	return fmt.Sprintf("API error: status %d: %s", e.StatusCode, e.Message)
}

// readResponse reads the body of resp and returns an *HTTPError for a non-2xx status.
func readResponse(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}
	return nil, &HTTPError{
		StatusCode: resp.StatusCode,
		Message:    errorMessage(body),
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}
}

// errorMessage extracts the message of a JSON error body ({"error":{"message":..}},
// {"error":".."} or {"message":".."}) and falls back to a short body excerpt.
func errorMessage(body []byte) string {
	var parsed struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		var s string
		switch {
		case json.Unmarshal(parsed.Error, &nested) == nil && nested.Message != "":
			return nested.Message
		case json.Unmarshal(parsed.Error, &s) == nil && s != "":
			return s
		case parsed.Message != "":
			return parsed.Message
		}
	}
	text := strings.TrimSpace(string(body))
	if utf8.RuneCountInString(text) > maxErrorBody {
		text = string([]rune(text)[:maxErrorBody]) + "..."
	}
	return text
}

// parseRetryAfter parses a Retry-After header given in seconds or as an HTTP date.
func parseRetryAfter(v string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
