package app

import (
	"github.com/up1512001/kov/internal/config"
	"github.com/up1512001/kov/internal/provider"
)

// buildProviders creates providers from config, in failover order.
func buildProviders(cfg *config.Config) []provider.Provider {
	var providers []provider.Provider

	// Priority: configured default first, then others by availability
	providerOrder := []string{"anthropic", "openai", "google", "ollama"}

	// Move the configured default to the front
	if cfg.Provider != "" {
		reordered := []string{cfg.Provider}
		for _, p := range providerOrder {
			if p != cfg.Provider {
				reordered = append(reordered, p)
			}
		}
		providerOrder = reordered
	}

	for _, name := range providerOrder {
		switch name {
		case "anthropic":
			if cfg.Providers.Anthropic != nil && cfg.Providers.Anthropic.APIKey != "" {
				providers = append(providers, provider.NewAnthropicProvider(
					cfg.Providers.Anthropic.APIKey,
					cfg.Providers.Anthropic.BaseURL,
				))
			}
		case "openai":
			if cfg.Providers.OpenAI != nil && cfg.Providers.OpenAI.APIKey != "" {
				providers = append(providers, provider.NewOpenAIProvider(
					cfg.Providers.OpenAI.APIKey,
					cfg.Providers.OpenAI.BaseURL,
				))
			}
		case "google":
			if cfg.Providers.Google != nil && cfg.Providers.Google.APIKey != "" {
				providers = append(providers, provider.NewGoogleProvider(
					cfg.Providers.Google.APIKey,
					cfg.Providers.Google.BaseURL,
				))
			}
		case "ollama":
			host := "http://localhost:11434"
			model := "qwen3:8b"
			if cfg.Providers.Ollama != nil {
				if cfg.Providers.Ollama.URL != "" {
					host = cfg.Providers.Ollama.URL
				}
				if cfg.Providers.Ollama.Model != "" {
					model = cfg.Providers.Ollama.Model
				}
			}
			// Ollama is always added as the final fallback
			providers = append(providers, provider.NewOllamaProvider(host, model))
		}
	}

	return providers
}
