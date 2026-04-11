package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected default model claude-sonnet-4-20250514, got %s", cfg.Model)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("expected default provider anthropic, got %s", cfg.Provider)
	}
	if cfg.Permissions != "confirm" {
		t.Errorf("expected default permissions confirm, got %s", cfg.Permissions)
	}
	if cfg.Resilience.LoopDetection.Threshold != 3 {
		t.Errorf("expected loop threshold 3, got %d", cfg.Resilience.LoopDetection.Threshold)
	}
	if !cfg.Resilience.Checkpoint {
		t.Error("expected checkpoint enabled by default")
	}
	if !cfg.Resilience.Failover {
		t.Error("expected failover enabled by default")
	}
	if cfg.Resilience.MaxRetries != 3 {
		t.Errorf("expected 3 max retries, got %d", cfg.Resilience.MaxRetries)
	}
	if !cfg.Verify.Enabled {
		t.Error("expected verify enabled by default")
	}
	if cfg.Verify.MaxFixRetries != 2 {
		t.Errorf("expected 2 fix retries, got %d", cfg.Verify.MaxFixRetries)
	}
	if !cfg.Git.AutoCommit {
		t.Error("expected autoCommit enabled by default")
	}
	if cfg.Git.CommitPrefix != "[kov]" {
		t.Errorf("expected commit prefix [kov], got %s", cfg.Git.CommitPrefix)
	}
}

func TestResolveAPIKeys(t *testing.T) {
	// Set env vars
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	os.Setenv("OPENAI_API_KEY", "sk-oai-test")
	defer os.Unsetenv("OPENAI_API_KEY")

	cfg := Defaults()
	cfg.resolveAPIKeys()

	if cfg.Providers.Anthropic == nil {
		t.Fatal("expected anthropic provider to be created")
	}
	if cfg.Providers.Anthropic.APIKey != "sk-ant-test" {
		t.Errorf("expected anthropic key sk-ant-test, got %s", cfg.Providers.Anthropic.APIKey)
	}
	if cfg.Providers.OpenAI == nil {
		t.Fatal("expected openai provider to be created")
	}
	if cfg.Providers.OpenAI.APIKey != "sk-oai-test" {
		t.Errorf("expected openai key sk-oai-test, got %s", cfg.Providers.OpenAI.APIKey)
	}
}

func TestGetAvailableProviders(t *testing.T) {
	cfg := Defaults()
	cfg.Providers.Anthropic = &ProviderConfig{APIKey: "sk-test"}
	cfg.Providers.OpenAI = &ProviderConfig{APIKey: "sk-test"}
	// Google not set, Ollama has default URL

	providers := cfg.GetAvailableProviders()

	if len(providers) != 3 {
		t.Errorf("expected 3 providers (anthropic, openai, ollama), got %d: %v", len(providers), providers)
	}
}
