package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Load reads and merges configuration from all sources (CLI flags,
// env vars, project .kov.yaml, user config) and returns the final Config.
//
// Priority (highest to lowest):
//  1. CLI flags (set via Viper BindPFlag)
//  2. Environment variables (KOV_*)
//  3. Project-level .kov.yaml (current directory)
//  4. User-level ~/.config/kov/config.yaml
//  5. Compiled defaults
func Load() (*Config, error) {
	cfg := Defaults()

	v := viper.New()
	v.SetConfigType("yaml")

	// 1. Bind environment variables with KOV_ prefix
	v.SetEnvPrefix("KOV")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Also check common provider env vars without KOV_ prefix
	bindProviderEnvVars(v)

	// 2. User-level config (~/.config/kov/config.yaml)
	userConfigDir, err := os.UserConfigDir()
	if err == nil {
		userConfigPath := filepath.Join(userConfigDir, "kov", "config.yaml")
		if _, err := os.Stat(userConfigPath); err == nil {
			v.SetConfigFile(userConfigPath)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("reading user config: %w", err)
			}
		}
	}

	// 3. Project-level config (.kov.yaml in current directory)
	cwd, err := os.Getwd()
	if err == nil {
		projectConfig := filepath.Join(cwd, ".kov.yaml")
		if _, err := os.Stat(projectConfig); err == nil {
			v.SetConfigFile(projectConfig)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("reading project config: %w", err)
			}
		}
	}

	// Unmarshal into config struct
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Resolve data directory
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir()
	}

	// Inject API keys from environment if not set in config
	cfg.resolveAPIKeys()

	return cfg, nil
}

// bindProviderEnvVars maps standard provider env vars to config keys.
func bindProviderEnvVars(v *viper.Viper) {
	// These are the standard env var names that every tool uses.
	// Users shouldn't need to learn new env var names for kov.
	_ = v.BindEnv("providers.anthropic.apiKey", "ANTHROPIC_API_KEY")
	_ = v.BindEnv("providers.openai.apiKey", "OPENAI_API_KEY")
	_ = v.BindEnv("providers.google.apiKey", "GEMINI_API_KEY", "GOOGLE_API_KEY")
	_ = v.BindEnv("providers.ollama.url", "OLLAMA_HOST")
}

// resolveAPIKeys fills in provider configs from env vars when present.
func (c *Config) resolveAPIKeys() {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		if c.Providers.Anthropic == nil {
			c.Providers.Anthropic = &ProviderConfig{}
		}
		if c.Providers.Anthropic.APIKey == "" {
			c.Providers.Anthropic.APIKey = key
		}
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		if c.Providers.OpenAI == nil {
			c.Providers.OpenAI = &ProviderConfig{}
		}
		if c.Providers.OpenAI.APIKey == "" {
			c.Providers.OpenAI.APIKey = key
		}
	}

	if key := os.Getenv("GEMINI_API_KEY"); key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
		if key != "" {
			if c.Providers.Google == nil {
				c.Providers.Google = &ProviderConfig{}
			}
			if c.Providers.Google.APIKey == "" {
				c.Providers.Google.APIKey = key
			}
		}
	} else {
		if c.Providers.Google == nil {
			c.Providers.Google = &ProviderConfig{}
		}
		if c.Providers.Google.APIKey == "" {
			c.Providers.Google.APIKey = key
		}
	}

	if host := os.Getenv("OLLAMA_HOST"); host != "" {
		if c.Providers.Ollama == nil {
			c.Providers.Ollama = &OllamaConfig{Model: "qwen3:8b"}
		}
		if c.Providers.Ollama.URL == "" {
			c.Providers.Ollama.URL = host
		}
	}
}

// defaultDataDir returns the default data storage directory.
func defaultDataDir() string {
	// XDG standard: ~/.local/share/kov
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "kov")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kov" // fallback to project dir
	}
	return filepath.Join(home, ".local", "share", "kov")
}

// GetAvailableProviders returns the IDs of providers that are actually usable.
// For cloud providers, this means having an API key configured.
// For Ollama, this means the service is actually running (health check).
func (c *Config) GetAvailableProviders() []string {
	var providers []string
	if c.Providers.Anthropic != nil && c.Providers.Anthropic.APIKey != "" {
		providers = append(providers, "anthropic")
	}
	if c.Providers.OpenAI != nil && c.Providers.OpenAI.APIKey != "" {
		providers = append(providers, "openai")
	}
	if c.Providers.Google != nil && c.Providers.Google.APIKey != "" {
		providers = append(providers, "google")
	}
	// Ollama: only report as available if explicitly configured AND confirmed running
	if c.Providers.Ollama != nil && c.Providers.Ollama.URL != "" {
		if isOllamaRunning() {
			providers = append(providers, "ollama")
		}
	}
	return providers
}
