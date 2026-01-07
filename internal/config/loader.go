package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()
	
	paths := []string{}
	if configPath != "" {
		paths = append(paths, configPath)
	} else {
		paths = GetConfigPaths()
	}

	var configFound bool
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if err := loadFromFile(path, cfg); err != nil {
				return nil, fmt.Errorf("failed to load config from %s: %w", path, err)
			}
			configFound = true
			break
		}
	}

	if !configFound && configPath != "" {
		return nil, fmt.Errorf("config file not found at %s", configPath)
	}

	expandEnvVarsInConfig(cfg)
	applyEnvironmentOverrides(cfg)
	
	return cfg, nil
}

func loadFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return err
	}

	return nil
}

func expandEnvVarsInConfig(cfg *Config) {
	for name, provider := range cfg.Providers {
		provider.APIKey = ExpandEnvVars(provider.APIKey)
		provider.BaseURL = ExpandEnvVars(provider.BaseURL)
		provider.Model = ExpandEnvVars(provider.Model)
		provider.Proxy.URL = ExpandEnvVars(provider.Proxy.URL)
		provider.Proxy.Username = ExpandEnvVars(provider.Proxy.Username)
		provider.Proxy.Password = ExpandEnvVars(provider.Proxy.Password)
		cfg.Providers[name] = provider
	}

	cfg.Proxy.URL = ExpandEnvVars(cfg.Proxy.URL)
	cfg.Proxy.Username = ExpandEnvVars(cfg.Proxy.Username)
	cfg.Proxy.Password = ExpandEnvVars(cfg.Proxy.Password)
}

func applyEnvironmentOverrides(cfg *Config) {
	if provider := os.Getenv("LLM_TRANSLATE_PROVIDER"); provider != "" {
		cfg.DefaultProvider = provider
	}
	
	if model := os.Getenv("LLM_TRANSLATE_MODEL"); model != "" {
		if provider, ok := cfg.Providers[cfg.DefaultProvider]; ok {
			provider.Model = model
			cfg.Providers[cfg.DefaultProvider] = provider
		}
	}
	
	if proxy := os.Getenv("LLM_TRANSLATE_PROXY"); proxy != "" {
		cfg.Proxy.URL = proxy
	} else if proxy := os.Getenv("HTTPS_PROXY"); proxy != "" {
		cfg.Proxy.URL = proxy
	} else if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
		cfg.Proxy.URL = proxy
	} else if proxy := os.Getenv("ALL_PROXY"); proxy != "" {
		cfg.Proxy.URL = proxy
	}
	
	if noProxy := os.Getenv("NO_PROXY"); noProxy != "" {
		cfg.Proxy.NoProxy = strings.Split(noProxy, ",")
	}
	
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		if provider, ok := cfg.Providers["openai"]; ok {
			provider.APIKey = apiKey
			cfg.Providers["openai"] = provider
		} else {
			cfg.Providers["openai"] = ProviderConfig{
				APIKey:  apiKey,
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4o-mini",
			}
		}
	}
	
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		if provider, ok := cfg.Providers["anthropic"]; ok {
			provider.APIKey = apiKey
			cfg.Providers["anthropic"] = provider
		} else {
			cfg.Providers["anthropic"] = ProviderConfig{
				APIKey:  apiKey,
				BaseURL: "https://api.anthropic.com",
				Model:   "claude-3-5-sonnet-20241022",
			}
		}
	}
	
	if apiKey := os.Getenv("GOOGLE_API_KEY"); apiKey != "" {
		if provider, ok := cfg.Providers["google"]; ok {
			provider.APIKey = apiKey
			cfg.Providers["google"] = provider
		} else {
			cfg.Providers["google"] = ProviderConfig{
				APIKey:  apiKey,
				BaseURL: "https://generativelanguage.googleapis.com/v1beta",
				Model:   "gemini-2.0-flash",
			}
		}
	}
	
	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		if provider, ok := cfg.Providers["openrouter"]; ok {
			provider.APIKey = apiKey
			cfg.Providers["openrouter"] = provider
		} else {
			cfg.Providers["openrouter"] = ProviderConfig{
				APIKey:  apiKey,
				BaseURL: "https://openrouter.ai/api/v1",
				Model:   "anthropic/claude-3.5-sonnet",
			}
		}
	}
}