// Package verify implements the verification runner — auto-detection of
// test/lint commands and the verify→fix→re-verify loop.
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/up1512001/kov/internal/tools"
)

// Runner executes verification commands and manages fix loops.
type Runner struct {
	tools      *tools.Registry
	projectDir string
	logger     *slog.Logger
	config     RunnerConfig
}

// RunnerConfig configures the verification runner.
type RunnerConfig struct {
	Command       string        // explicit verify command (overrides auto-detect)
	Timeout       time.Duration
	MaxFixRetries int
	Enabled       bool
}

// DefaultRunnerConfig returns sensible defaults.
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		Timeout:       120 * time.Second,
		MaxFixRetries: 2,
		Enabled:       true,
	}
}

// NewRunner creates a verification runner.
func NewRunner(toolRegistry *tools.Registry, projectDir string, logger *slog.Logger, config RunnerConfig) *Runner {
	return &Runner{
		tools:      toolRegistry,
		projectDir: projectDir,
		logger:     logger,
		config:     config,
	}
}

// Result holds the outcome of a verification run.
type Result struct {
	Passed  bool
	Command string
	Output  string
	Error   error
}

// Run executes the verification command.
func (r *Runner) Run(ctx context.Context) (*Result, error) {
	cmd := r.config.Command
	if cmd == "" {
		cmd = r.AutoDetect()
	}
	if cmd == "" {
		return &Result{Passed: true, Command: "(none)"}, nil
	}

	r.logger.Info("running verification", slog.String("command", cmd))

	args, _ := json.Marshal(map[string]interface{}{
		"command": cmd,
		"timeout": int(r.config.Timeout.Seconds()),
	})

	output, err := r.tools.Execute(ctx, "shell_exec", args)

	return &Result{
		Passed:  err == nil,
		Command: cmd,
		Output:  output,
		Error:   err,
	}, nil
}

// AutoDetect guesses the test command based on project files.
func (r *Runner) AutoDetect() string {
	checks := []struct {
		file    string
		command string
	}{
		// Go
		{"go.mod", "go test ./..."},
		// Node.js
		{"package.json", r.detectNodeTestCmd()},
		// Python
		{"pyproject.toml", "python -m pytest"},
		{"setup.py", "python -m pytest"},
		{"pytest.ini", "python -m pytest"},
		// Rust
		{"Cargo.toml", "cargo test"},
		// Ruby
		{"Gemfile", "bundle exec rspec"},
		// Java/Kotlin
		{"build.gradle", "./gradlew test"},
		{"build.gradle.kts", "./gradlew test"},
		{"pom.xml", "mvn test"},
		// Elixir
		{"mix.exs", "mix test"},
		// Makefile
		{"Makefile", r.detectMakeTarget()},
	}

	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(r.projectDir, c.file)); err == nil {
			if c.command != "" {
				return c.command
			}
		}
	}

	return ""
}

// detectNodeTestCmd reads package.json to find the test script.
func (r *Runner) detectNodeTestCmd() string {
	data, err := os.ReadFile(filepath.Join(r.projectDir, "package.json"))
	if err != nil {
		return ""
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	// Prefer test, then lint, then check
	for _, name := range []string{"test", "lint", "check", "typecheck"} {
		if _, ok := pkg.Scripts[name]; ok {
			return "npm run " + name
		}
	}

	return ""
}

// detectMakeTarget checks if Makefile has test/check targets.
func (r *Runner) detectMakeTarget() string {
	data, err := os.ReadFile(filepath.Join(r.projectDir, "Makefile"))
	if err != nil {
		return ""
	}
	content := string(data)

	for _, target := range []string{"test", "check", "lint"} {
		if strings.Contains(content, target+":") {
			return "make " + target
		}
	}

	return ""
}

// VerifyFixLoop runs verify→fix→re-verify up to maxRetries.
// fixFn is called with the error output and should attempt to fix it.
// Returns the final result.
func (r *Runner) VerifyFixLoop(ctx context.Context, fixFn func(ctx context.Context, errorOutput string) error) (*Result, error) {
	for attempt := 0; attempt <= r.config.MaxFixRetries; attempt++ {
		result, err := r.Run(ctx)
		if err != nil {
			return nil, fmt.Errorf("verification execution error: %w", err)
		}

		if result.Passed {
			r.logger.Info("verification passed",
				slog.String("command", result.Command),
				slog.Int("attempt", attempt))
			return result, nil
		}

		// Last attempt — don't try to fix
		if attempt >= r.config.MaxFixRetries {
			r.logger.Warn("verification failed after max retries",
				slog.Int("attempts", attempt+1))
			return result, nil
		}

		r.logger.Info("verification failed, attempting fix",
			slog.Int("attempt", attempt+1),
			slog.Int("max", r.config.MaxFixRetries))

		// Let the agent fix it
		if fixFn != nil {
			if err := fixFn(ctx, result.Output); err != nil {
				return result, fmt.Errorf("fix attempt failed: %w", err)
			}
		}
	}

	// Should not reach here
	return &Result{Passed: false}, nil
}
