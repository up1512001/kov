package config

import "time"

// Defaults returns a Config with all default values populated.
// These match the documented defaults in the architecture plan.
func Defaults() *Config {
	return &Config{
		Model:       "claude-sonnet-4-20250514",
		Provider:    "anthropic",
		Permissions: "confirm",
		Providers: ProvidersConfig{
			// All providers are nil by default.
			// They are populated from env vars, config files, or auto-detection.
			// Ollama is NOT hardcoded — it's only added when confirmed running.
		},
		Resilience: ResilienceConfig{
			Checkpoint:       true,
			Failover:         true,
			FallbackProvider: "ollama",
			MaxRetries:       3,
			Backoff: BackoffConfig{
				Initial: 2 * time.Second,
				Max:     60 * time.Second,
				Jitter:  true,
			},
			PauseOnAuthError: true,
			LoopDetection: LoopConfig{
				Enabled:   true,
				Threshold: 3,
			},
		},
		Verify: VerifyConfig{
			Enabled:       true,
			Command:       "", // auto-detect
			AfterEachTask: true,
			Timeout:       120 * time.Second,
			MaxFixRetries: 2,
		},
		Git: GitConfig{
			AutoCommit:         true,
			CommitPrefix:       "[kov]",
			SnapshotBeforeEdit: true,
		},
		Cost: CostConfig{
			BudgetPerSession: 0, // unlimited
			WarnAt:           5.00,
		},
		Pipe: PipeConfig{
			OutputFormat: "json",
		},
		UI: UIConfig{
			Theme:            "dark",
			ShowCost:         true,
			ShowTaskProgress: true,
		},
	}
}
