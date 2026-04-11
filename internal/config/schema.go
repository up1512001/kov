// Package config handles all configuration for kov.
// Configuration is loaded from (in priority order):
//  1. CLI flags
//  2. Environment variables (KOV_*)
//  3. Project-level .kov.yaml
//  4. User-level ~/.config/kov/config.yaml
//  5. Defaults
package config

import "time"

// Config is the top-level configuration struct for kov.
type Config struct {
	// Model is the default LLM model to use.
	Model string `mapstructure:"model" yaml:"model"`

	// Provider is the default provider (anthropic, openai, google, ollama).
	Provider string `mapstructure:"provider" yaml:"provider"`

	// Providers holds per-provider configuration.
	Providers ProvidersConfig `mapstructure:"providers" yaml:"providers"`

	// Modes holds per-mode overrides for model, thinkBudget, etc.
	Modes ModesConfig `mapstructure:"modes" yaml:"modes"`

	// Resilience configures the resilience engine.
	Resilience ResilienceConfig `mapstructure:"resilience" yaml:"resilience"`

	// Verify configures automatic test verification.
	Verify VerifyConfig `mapstructure:"verify" yaml:"verify"`

	// Permissions controls the permission model.
	// Values: "confirm" (default), "smart", "yolo", "chat"
	Permissions string `mapstructure:"permissions" yaml:"permissions"`

	// Git configures git integration.
	Git GitConfig `mapstructure:"git" yaml:"git"`

	// Cost configures budget limits.
	Cost CostConfig `mapstructure:"cost" yaml:"cost"`

	// MCP configures Model Context Protocol servers (v0.1: config only).
	MCPServers map[string]MCPServerConfig `mapstructure:"mcpServers" yaml:"mcpServers"`

	// Pipe configures pipe mode output.
	Pipe PipeConfig `mapstructure:"pipe" yaml:"pipe"`

	// UI configures the terminal interface.
	UI UIConfig `mapstructure:"ui" yaml:"ui"`

	// DataDir is where kov stores its data (~/.local/share/kov).
	DataDir string `mapstructure:"dataDir" yaml:"dataDir"`
}

// ProvidersConfig holds configuration for each LLM provider.
type ProvidersConfig struct {
	Anthropic *ProviderConfig `mapstructure:"anthropic" yaml:"anthropic,omitempty"`
	OpenAI    *ProviderConfig `mapstructure:"openai" yaml:"openai,omitempty"`
	Google    *ProviderConfig `mapstructure:"google" yaml:"google,omitempty"`
	Ollama    *OllamaConfig   `mapstructure:"ollama" yaml:"ollama,omitempty"`
}

// ProviderConfig is the config for a cloud LLM provider.
type ProviderConfig struct {
	APIKey  string `mapstructure:"apiKey" yaml:"apiKey,omitempty"`
	BaseURL string `mapstructure:"baseURL" yaml:"baseURL,omitempty"`
}

// OllamaConfig is the config for the local Ollama provider.
type OllamaConfig struct {
	URL   string `mapstructure:"url" yaml:"url"`
	Model string `mapstructure:"model" yaml:"model"`
}

// ModesConfig holds per-mode overrides.
type ModesConfig struct {
	Code      *ModeConfig      `mapstructure:"code" yaml:"code,omitempty"`
	Plan      *ModeConfig      `mapstructure:"plan" yaml:"plan,omitempty"`
	Think     *ModeConfig      `mapstructure:"think" yaml:"think,omitempty"`
	Ask       *ModeConfig      `mapstructure:"ask" yaml:"ask,omitempty"`
	Fast      *ModeConfig      `mapstructure:"fast" yaml:"fast,omitempty"`
	Research  *ModeConfig      `mapstructure:"research" yaml:"research,omitempty"`
	Architect *ArchitectConfig `mapstructure:"architect" yaml:"architect,omitempty"`
	Review    *ModeConfig      `mapstructure:"review" yaml:"review,omitempty"`
}

// ModeConfig holds overrides for a single mode.
type ModeConfig struct {
	Model       string `mapstructure:"model" yaml:"model,omitempty"`
	ThinkBudget string `mapstructure:"thinkBudget" yaml:"thinkBudget,omitempty"` // low, medium, high, maximum
}

// ArchitectConfig holds the dual-model config for architect mode.
type ArchitectConfig struct {
	PlanModel string `mapstructure:"planModel" yaml:"planModel,omitempty"`
	EditModel string `mapstructure:"editModel" yaml:"editModel,omitempty"`
}

// ResilienceConfig controls the resilience engine.
type ResilienceConfig struct {
	Checkpoint       bool           `mapstructure:"checkpoint" yaml:"checkpoint"`
	Failover         bool           `mapstructure:"failover" yaml:"failover"`
	FallbackProvider string         `mapstructure:"fallbackProvider" yaml:"fallbackProvider"`
	MaxRetries       int            `mapstructure:"maxRetries" yaml:"maxRetries"`
	Backoff          BackoffConfig  `mapstructure:"backoff" yaml:"backoff"`
	PauseOnAuthError bool           `mapstructure:"pauseOnAuthError" yaml:"pauseOnAuthError"`
	LoopDetection    LoopConfig     `mapstructure:"loopDetection" yaml:"loopDetection"`
}

// BackoffConfig controls retry backoff behavior.
type BackoffConfig struct {
	Initial time.Duration `mapstructure:"initial" yaml:"initial"`
	Max     time.Duration `mapstructure:"max" yaml:"max"`
	Jitter  bool          `mapstructure:"jitter" yaml:"jitter"`
}

// LoopConfig controls doom-loop detection.
type LoopConfig struct {
	Enabled   bool `mapstructure:"enabled" yaml:"enabled"`
	Threshold int  `mapstructure:"threshold" yaml:"threshold"`
}

// VerifyConfig controls automatic verification after sub-tasks.
type VerifyConfig struct {
	Enabled        bool          `mapstructure:"enabled" yaml:"enabled"`
	Command        string        `mapstructure:"command" yaml:"command"`
	AfterEachTask  bool          `mapstructure:"afterEachTask" yaml:"afterEachTask"`
	Timeout        time.Duration `mapstructure:"timeout" yaml:"timeout"`
	MaxFixRetries  int           `mapstructure:"maxFixRetries" yaml:"maxFixRetries"`
}

// GitConfig controls git integration.
type GitConfig struct {
	AutoCommit         bool   `mapstructure:"autoCommit" yaml:"autoCommit"`
	CommitPrefix       string `mapstructure:"commitPrefix" yaml:"commitPrefix"`
	SnapshotBeforeEdit bool   `mapstructure:"snapshotBeforeEdit" yaml:"snapshotBeforeEdit"`
}

// CostConfig controls budget limits.
type CostConfig struct {
	BudgetPerSession float64 `mapstructure:"budgetPerSession" yaml:"budgetPerSession"`
	WarnAt           float64 `mapstructure:"warnAt" yaml:"warnAt"`
}

// MCPServerConfig describes an MCP server.
type MCPServerConfig struct {
	Command string            `mapstructure:"command" yaml:"command"`
	Args    []string          `mapstructure:"args" yaml:"args"`
	Env     map[string]string `mapstructure:"env" yaml:"env,omitempty"`
}

// PipeConfig controls pipe mode output.
type PipeConfig struct {
	OutputFormat string `mapstructure:"outputFormat" yaml:"outputFormat"` // json, text
}

// UIConfig controls the TUI.
type UIConfig struct {
	Theme            string `mapstructure:"theme" yaml:"theme"`
	ShowCost         bool   `mapstructure:"showCost" yaml:"showCost"`
	ShowTaskProgress bool   `mapstructure:"showTaskProgress" yaml:"showTaskProgress"`
}
