package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/foxzi/llm-translate/internal/config"
	"golang.org/x/net/proxy"
)

func NewHTTPClient(cfg config.ProxyConfig) (*http.Client, error) {
	if cfg.URL == "" {
		return &http.Client{
			Timeout: 60 * time.Second,
		}, nil
	}
	
	proxyURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	
	if cfg.Username != "" && cfg.Password != "" {
		proxyURL.User = url.UserPassword(cfg.Username, cfg.Password)
	}
	
	transport := &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	
	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = func(req *http.Request) (*url.URL, error) {
			if shouldBypassProxy(req.URL.Host, cfg.NoProxy) {
				return nil, nil
			}
			return proxyURL, nil
		}
		
	case "socks5", "socks5h":
		auth := &proxy.Auth{}
		if proxyURL.User != nil {
			auth.User = proxyURL.User.Username()
			auth.Password, _ = proxyURL.User.Password()
		}
		
		dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}
		
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			
			if shouldBypassProxy(host, cfg.NoProxy) {
				return (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext(ctx, network, addr)
			}
			
			return dialer.Dial(network, addr)
		}
		
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
	
	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}, nil
}

func shouldBypassProxy(host string, noProxyList []string) bool {
	if len(noProxyList) == 0 {
		return false
	}
	
	host = strings.ToLower(host)
	
	for _, pattern := range noProxyList {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		
		if pattern == "" {
			continue
		}
		
		if pattern == "*" {
			return true
		}
		
		if strings.HasPrefix(pattern, "*.") {
			domain := pattern[2:]
			if strings.HasSuffix(host, domain) {
				return true
			}
		}
		
		if strings.Contains(pattern, "*") {
			if matchPattern(host, pattern) {
				return true
			}
		} else {
			if host == pattern {
				return true
			}
			
			if net.ParseIP(host) != nil && net.ParseIP(pattern) != nil {
				if host == pattern {
					return true
				}
			}
		}
	}
	
	return false
}

func matchPattern(text, pattern string) bool {
	pattern = strings.ReplaceAll(pattern, ".", "\\.")
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	
	matched := strings.HasPrefix(text, pattern) || strings.HasSuffix(text, pattern)
	return matched
}