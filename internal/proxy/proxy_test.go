package proxy

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/foxzi/llm-translate/internal/config"
)

func proxyFor(t *testing.T, cfg config.ProxyConfig, target string) *url.URL {
	t.Helper()
	c, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("transport = %#v, want *http.Transport with Proxy", c.Transport)
	}
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestHTTPProxyApplied(t *testing.T) {
	cfg := config.ProxyConfig{URL: "http://proxy.local:3128", Username: "u", Password: "p"}
	u := proxyFor(t, cfg, "https://api.example.com/v1")
	if u == nil || u.Host != "proxy.local:3128" {
		t.Fatalf("proxy = %v", u)
	}
	if pw, _ := u.User.Password(); u.User.Username() != "u" || pw != "p" {
		t.Errorf("credentials not applied: %v", u.User)
	}
}

func TestNoProxyBypass(t *testing.T) {
	cfg := config.ProxyConfig{URL: "http://proxy.local:3128", NoProxy: []string{"localhost", "*.internal", "10.*"}}
	for _, target := range []string{"http://localhost:8000/x", "http://llm.internal/x", "http://10.1.2.3:8000/x"} {
		if u := proxyFor(t, cfg, target); u != nil {
			t.Errorf("%s went through proxy %v", target, u)
		}
	}
	if u := proxyFor(t, cfg, "https://api.example.com"); u == nil {
		t.Error("external host bypassed the proxy")
	}
}

func TestShouldBypassProxy(t *testing.T) {
	tests := []struct {
		host string
		list []string
		want bool
	}{
		{"example.com", nil, false},
		{"example.com", []string{"*"}, true},
		{"EXAMPLE.com", []string{"example.com"}, true},
		{"a.example.com", []string{"*.example.com"}, true},
		{"example.com", []string{"*.example.com"}, true},
		{"badexample.com", []string{"*.example.com"}, false},
		{"192.168.1.5", []string{"192.168.*"}, true},
		{"example.org", []string{"", " "}, false},
	}
	for _, tc := range tests {
		if got := shouldBypassProxy(tc.host, tc.list); got != tc.want {
			t.Errorf("shouldBypassProxy(%q, %q) = %v, want %v", tc.host, tc.list, got, tc.want)
		}
	}
}

func TestNewHTTPClientVariants(t *testing.T) {
	c, err := NewHTTPClient(config.ProxyConfig{}, 5)
	if err != nil || c.Transport != nil || c.Timeout != 5*time.Second {
		t.Errorf("no proxy: %+v, %v", c, err)
	}
	if c, err = NewHTTPClient(config.ProxyConfig{URL: "socks5://127.0.0.1:1080"}); err != nil || c.Transport == nil || c.Timeout != 60*time.Second {
		t.Errorf("socks5: %+v, %v", c, err)
	}
	if _, err = NewHTTPClient(config.ProxyConfig{URL: "ftp://x"}); err == nil {
		t.Error("unsupported scheme accepted")
	}
	if _, err = NewHTTPClient(config.ProxyConfig{URL: "http://a b"}); err == nil {
		t.Error("invalid URL accepted")
	}
}
