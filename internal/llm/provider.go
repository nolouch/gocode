package llm

import (
	"fmt"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/google"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"
)

// BuildProvider creates a fantasy.Provider based on the config.
func BuildProvider(cfg Config) (fantasy.Provider, error) {
	switch cfg.ProviderName {
	case "openai", "":
		opts := []openai.Option{
			openai.WithAPIKey(cfg.APIKey),
			openai.WithUseResponsesAPI(),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
		}
		return openai.New(opts...)

	case "anthropic":
		opts := []anthropic.Option{
			anthropic.WithAPIKey(cfg.APIKey),
			anthropic.WithHeaders(map[string]string{
				"Authorization": "Bearer " + cfg.APIKey,
			}),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(cfg.BaseURL))
		}
		return anthropic.New(opts...)

	case "google":
		opts := []google.Option{google.WithGeminiAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, google.WithBaseURL(cfg.BaseURL))
		}
		return google.New(opts...)

	case "openrouter":
		return openrouter.New(openrouter.WithAPIKey(cfg.APIKey))

	case "openai-compat":
		opts := []openaicompat.Option{
			openaicompat.WithAPIKey(cfg.APIKey),
			openaicompat.WithUseResponsesAPI(),
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openaicompat.WithBaseURL(cfg.BaseURL))
		}
		return openaicompat.New(opts...)

	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.ProviderName)
	}
}
