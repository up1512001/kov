package config

import (
	"os"
	"testing"
	"time"
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
	// Google not set, Ollama not in defaults anymore

	providers := cfg.GetAvailableProviders()

	if len(providers) != 2 {
		t.Errorf("expected 2 providers (anthropic, openai), got %d: %v", len(providers), providers)
	}
}

func TestDefaults_NoOllamaByDefault(t *testing.T) {
	cfg := Defaults()
	if cfg.Providers.Ollama != nil {
		t.Error("expected no Ollama provider in defaults")
	}
	if cfg.Resilience.FallbackProvider != "" {
		t.Errorf("expected empty fallback provider, got %s", cfg.Resilience.FallbackProvider)
	}
}

func TestDefaults_CostConfig(t *testing.T) {
	cfg := Defaults()

	if cfg.Cost.BudgetPerSession != 0 {
		t.Errorf("expected budget 0 (unlimited), got %f", cfg.Cost.BudgetPerSession)
	}
	if cfg.Cost.WarnAt != 5.00 {
		t.Errorf("expected warn at 5.00, got %f", cfg.Cost.WarnAt)
	}
}

func TestDefaults_GitConfig(t *testing.T) {
	cfg := Defaults()

	if !cfg.Git.AutoCommit {
		t.Error("expected autoCommit enabled")
	}
	if !cfg.Git.SnapshotBeforeEdit {
		t.Error("expected snapshotBeforeEdit enabled")
	}
	if cfg.Git.CommitPrefix != "[kov]" {
		t.Errorf("expected commit prefix '[kov]', got %s", cfg.Git.CommitPrefix)
	}
}

func TestDefaults_ResilienceBackoff(t *testing.T) {
	cfg := Defaults()

	if cfg.Resilience.Backoff.Initial != 2*time.Second {
		t.Errorf("expected 2s initial backoff, got %v", cfg.Resilience.Backoff.Initial)
	}
	if cfg.Resilience.Backoff.Max != 60*time.Second {
		t.Errorf("expected 60s max backoff, got %v", cfg.Resilience.Backoff.Max)
	}
	if !cfg.Resilience.Backoff.Jitter {
		t.Error("expected jitter enabled by default")
	}
}

func TestDefaults_VerifyConfig(t *testing.T) {
	cfg := Defaults()

	if !cfg.Verify.Enabled {
		t.Error("expected verify enabled")
	}
	if cfg.Verify.Timeout != 120*time.Second {
		t.Errorf("expected 120s timeout, got %v", cfg.Verify.Timeout)
	}
	if cfg.Verify.MaxFixRetries != 2 {
		t.Errorf("expected 2 retries, got %d", cfg.Verify.MaxFixRetries)
	}
}

func TestDefaults_UIConfig(t *testing.T) {
	cfg := Defaults()

	if cfg.UI.Theme != "dark" {
		t.Errorf("expected dark theme, got %s", cfg.UI.Theme)
	}
	if !cfg.UI.ShowCost {
		t.Error("expected ShowCost true")
	}
	if !cfg.UI.ShowTaskProgress {
		t.Error("expected ShowTaskProgress true")
	}
}

func TestDefaults_ModesConfig(t *testing.T) {
	cfg := Defaults()

	// Default: no mode overrides
	if cfg.Modes.Code != nil {
		t.Error("expected no code mode override by default")
	}
	if cfg.Modes.Plan != nil {
		t.Error("expected no plan mode override by default")
	}
}

func TestResolveAPIKeys_Google(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "gk-test")
	defer os.Unsetenv("GEMINI_API_KEY")

	cfg := Defaults()
	cfg.resolveAPIKeys()

	if cfg.Providers.Google == nil {
		t.Fatal("expected google provider to be created")
	}
	if cfg.Providers.Google.APIKey != "gk-test" {
		t.Errorf("expected google key gk-test, got %s", cfg.Providers.Google.APIKey)
	}
}

func TestResolveAPIKeys_OllamaHost(t *testing.T) {
	os.Setenv("OLLAMA_HOST", "http://custom:11435")
	defer os.Unsetenv("OLLAMA_HOST")

	cfg := Defaults()
	// Clear default ollama so resolveAPIKeys creates fresh from env
	cfg.Providers.Ollama = nil
	cfg.resolveAPIKeys()

	if cfg.Providers.Ollama == nil {
		t.Fatal("expected ollama config")
	}
	if cfg.Providers.Ollama.URL != "http://custom:11435" {
		t.Errorf("expected custom ollama URL, got %s", cfg.Providers.Ollama.URL)
	}
}

func TestGetAvailableProviders_Empty(t *testing.T) {
	cfg := Defaults()
	// Clear all providers
	cfg.Providers = ProvidersConfig{}

	providers := cfg.GetAvailableProviders()
	if len(providers) != 0 {
		t.Errorf("expected 0 providers when none configured, got %d", len(providers))
	}
}

func TestGetAvailableProviders_OllamaRequiresHealthCheck(t *testing.T) {
	cfg := Defaults()
	cfg.Providers = ProvidersConfig{
		Ollama: &OllamaConfig{URL: "http://localhost:11434"},
	}

	// Ollama is configured but not running — should NOT show as available
	providers := cfg.GetAvailableProviders()
	if isOllamaRunning() {
		// If Ollama happens to be running, it should show
		if len(providers) != 1 {
			t.Errorf("expected 1 provider (ollama running), got %d", len(providers))
		}
	} else {
		if len(providers) != 0 {
			t.Errorf("expected 0 providers (ollama not running), got %d", len(providers))
		}
	}
}

