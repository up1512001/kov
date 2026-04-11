// Package app provides the top-level application orchestrator for kov.
// It wires together all subsystems using lazy initialization to achieve
// sub-50ms cold start.
package app

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/utsavkovy/kov/internal/bus"
	"github.com/utsavkovy/kov/internal/config"
)

// BuildInfo contains version metadata injected at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// App is the top-level application orchestrator.
// All subsystems are lazily initialized on first use.
type App struct {
	build  BuildInfo
	bus    *bus.Bus
	config *Lazy[*config.Config]
	logger *slog.Logger

	// Root cobra command
	rootCmd *cobra.Command
}

// New creates a new App instance with lazy subsystem initialization.
func New(build BuildInfo) *App {
	a := &App{
		build: build,
		bus:   bus.New(),
	}

	// Config loads lazily on first access
	a.config = NewLazy(func() (*config.Config, error) {
		return config.Load()
	})

	// Structured logger
	a.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Build CLI commands
	a.rootCmd = a.buildRootCmd()
	a.rootCmd.AddCommand(
		a.buildResumeCmd(),
		a.buildSessionsCmd(),
		a.buildVersionCmd(),
		a.buildModelsCmd(),
		a.buildConfigCmd(),
	)

	return a
}

// RootCmd returns the root cobra command for execution.
func (a *App) RootCmd() *cobra.Command {
	return a.rootCmd
}

// buildRootCmd creates the root `kov` command.
func (a *App) buildRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kov [prompt]",
		Short: "Indestructible AI coding in your terminal",
		Long: `Kov is an open-source AI coding CLI that makes your coding sessions indestructible.

Auto-recovers from any failure — rate limits, auth expiry, network drops, crashes.
Works with all frontier models. Sub-50ms startup. Zero dependencies.

  kov "refactor auth to JWT"          # code mode (default)
  kov --mode plan "analyze this repo" # read-only analysis
  kov --mode think "debug this test"  # extended reasoning
  kov --mode fast "fix the typo"      # minimal overhead
  kov resume                          # resume last interrupted session

Documentation: https://trykov.dev`,
		Args:                  cobra.ArbitraryArgs,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		RunE:                  a.runRoot,
	}

	// Global flags
	flags := cmd.PersistentFlags()
	flags.StringP("mode", "m", "code", "Agent mode: code, plan, think, ask, fast, research, architect, review, pipe")
	flags.StringP("model", "M", "", "Override the default model")
	flags.StringP("provider", "p", "", "Override the default provider")
	flags.Float64("budget", 0, "Max cost (USD) for this session")
	flags.String("permissions", "", "Permission mode: confirm, smart, yolo, chat")
	flags.Bool("verbose", false, "Enable verbose logging")
	flags.String("profile", "", "Enable profiling: cpu, mem, trace")

	return cmd
}

// runRoot handles the main `kov [prompt]` command.
func (a *App) runRoot(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// No prompt — start interactive mode
		fmt.Println(a.banner())
		fmt.Println("  Type your prompt, or use --help for options.")
		fmt.Println()
		// TODO: Start interactive TUI (Day 10)
		return nil
	}

	prompt := strings.Join(args, " ")
	mode, _ := cmd.Flags().GetString("mode")

	a.logger.Info("starting session",
		slog.String("mode", mode),
		slog.String("prompt", truncate(prompt, 80)),
	)

	// TODO: Initialize agent and run (Day 7)
	fmt.Printf("🔨 [%s mode] %s\n", mode, prompt)
	fmt.Println("⚡ Agent loop not yet implemented — coming Day 7")

	return nil
}

// buildResumeCmd creates the `kov resume` command.
func (a *App) buildResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [session-id]",
		Short: "Resume an interrupted session",
		Long: `Resume the most recent interrupted session, or a specific
session by ID. Kov recovers the exact state from its checkpoint
and continues from where it left off.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				fmt.Printf("♻️  Resuming session: %s\n", args[0])
			} else {
				fmt.Println("♻️  Resuming last interrupted session...")
			}
			// TODO: Implement resume (Day 11)
			fmt.Println("⚡ Resume not yet implemented — coming Day 11")
			return nil
		},
	}
}

// buildSessionsCmd creates the `kov sessions` command.
func (a *App) buildSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List and manage sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: List sessions from SQLite (Day 11)
			fmt.Println("📋 Sessions list not yet implemented — coming Day 11")
			return nil
		},
	}
	return cmd
}

// buildVersionCmd creates the `kov version` command.
func (a *App) buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kov %s\n", a.build.Version)
			fmt.Printf("  commit: %s\n", a.build.Commit)
			fmt.Printf("  built:  %s\n", a.build.Date)
			fmt.Printf("  site:   https://trykov.dev\n")
		},
	}
}

// buildModelsCmd creates the `kov models` command.
func (a *App) buildModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available models from configured providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.config.Get()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			providers := cfg.GetAvailableProviders()
			if len(providers) == 0 {
				fmt.Println("No providers configured. Set an API key:")
				fmt.Println("  export ANTHROPIC_API_KEY=sk-...")
				fmt.Println("  export OPENAI_API_KEY=sk-...")
				fmt.Println("  export GEMINI_API_KEY=...")
				return nil
			}

			fmt.Println("Available providers:")
			for _, p := range providers {
				fmt.Printf("  ✓ %s\n", p)
			}
			return nil
		},
	}
}

// buildConfigCmd creates the `kov config` command.
func (a *App) buildConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.config.Get()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			fmt.Printf("Default provider: %s\n", cfg.Provider)
			fmt.Printf("Default model:    %s\n", cfg.Model)
			fmt.Printf("Permissions:      %s\n", cfg.Permissions)
			fmt.Printf("Data directory:   %s\n", cfg.DataDir)
			fmt.Printf("Resilience:       checkpoint=%t failover=%t\n", cfg.Resilience.Checkpoint, cfg.Resilience.Failover)
			fmt.Printf("Verification:     enabled=%t command=%q\n", cfg.Verify.Enabled, cfg.Verify.Command)
			fmt.Printf("Loop detection:   threshold=%d\n", cfg.Resilience.LoopDetection.Threshold)
			fmt.Printf("Cost budget:      $%.2f/session\n", cfg.Cost.BudgetPerSession)

			providers := cfg.GetAvailableProviders()
			fmt.Printf("Providers:        %s\n", strings.Join(providers, ", "))

			return nil
		},
	}
}

// banner returns the startup banner for interactive mode.
func (a *App) banner() string {
	return fmt.Sprintf(`
  ██╗  ██╗ ██████╗ ██╗   ██╗
  ██║ ██╔╝██╔═══██╗██║   ██║
  █████╔╝ ██║   ██║██║   ██║
  ██╔═██╗ ██║   ██║╚██╗ ██╔╝
  ██║  ██╗╚██████╔╝ ╚████╔╝
  ╚═╝  ╚═╝ ╚═════╝   ╚═══╝  %s
  Indestructible AI coding   trykov.dev
`, a.build.Version)
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
