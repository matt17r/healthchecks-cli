package main

import (
	"fmt"
	"os"
	"strings"
)

const defaultBaseURL = "https://healthchecks.io"

// Config is assembled from environment variables.
type Config struct {
	APIKey     string // HC_API_KEY
	BaseURL    string // HC_BASE_URL (default https://healthchecks.io)
	AllowWrite bool   // HC_ALLOW_WRITE
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		APIKey:     os.Getenv("HC_API_KEY"),
		BaseURL:    os.Getenv("HC_BASE_URL"),
		AllowWrite: truthy(os.Getenv("HC_ALLOW_WRITE")),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("HC_API_KEY is not set (export your healthchecks.io project API key)")
	}
	return cfg, nil
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
