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

	"github.com/up1512001/kov/internal/agent"
	"github.com/up1512001/kov/internal/bus"
	"github.com/up1512001/kov/internal/config"
	"github.com/up1512001/kov/internal/db"
	"github.com/up1512001/kov/internal/provider"
	"github.com/up1512001/kov/internal/resilience"
	"github.com/up1512001/kov/internal/session"
	"github.com/up1512001/kov/internal/tools"
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
	modelFlag, _ := cmd.Flags().GetString("model")
	budgetFlag, _ := cmd.Flags().GetFloat64("budget")
	verbose, _ := cmd.Flags().GetBool("verbose")

	if verbose {
		a.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	// Load config
	cfg, err := a.config.Get()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	model := cfg.Model
	if modelFlag != "" {
		model = modelFlag
	}

	budget := cfg.Cost.BudgetPerSession
	if budgetFlag > 0 {
		budget = budgetFlag
	}

	// Initialize DB
	dbPath := cfg.DataDir + "/kov.db"
	database, err := db.Open(dbPath, a.logger)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	// Build provider chain
	providers := buildProviders(cfg)
	if len(providers) == 0 {
		fmt.Println("❌ No providers configured. Set an API key:")
		fmt.Println("  export ANTHROPIC_API_KEY=sk-ant-...")
		fmt.Println("  export OPENAI_API_KEY=sk-...")
		fmt.Println("  export GEMINI_API_KEY=...")
		return fmt.Errorf("no providers available")
	}

	router := provider.NewRouter(providers, a.bus, a.logger, provider.DefaultRouterConfig())
	defer router.Close()

	// Get project directory
	projectDir, _ := os.Getwd()

	// Initialize tools
	toolRegistry := tools.DefaultRegistry(projectDir)

	// Initialize resilience engine
	engine := resilience.NewEngine(database, a.bus, a.logger, resilience.EngineConfig{
		CheckpointEnabled: cfg.Resilience.Checkpoint,
		LoopThreshold:     cfg.Resilience.LoopDetection.Threshold,
		BudgetPerSession:  budget,
		BudgetWarnAt:      cfg.Cost.WarnAt,
		MaxFixRetries:     cfg.Verify.MaxFixRetries,
	})

	// Create session
	sessionID, err := database.CreateSession(cmd.Context(), &db.Session{
		ProjectDir: projectDir,
		Mode:       mode,
		Provider:   cfg.Provider,
		Model:      model,
		State:      "idle",
		Prompt:     prompt,
	})
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	a.logger.Info("session created",
		slog.String("session", sessionID),
		slog.String("mode", mode),
		slog.String("model", model),
		slog.String("prompt", truncate(prompt, 80)),
	)

	// Configure agent
	agentCfg := agent.DefaultAgentConfig()
	agentCfg.Mode = mode
	agentCfg.Model = model
	agentCfg.Permissions = cfg.Permissions
	agentCfg.VerifyEnabled = cfg.Verify.Enabled
	agentCfg.VerifyCommand = cfg.Verify.Command
	agentCfg.ProjectDir = projectDir

	ag := agent.New(database, router, toolRegistry, engine, a.bus, a.logger, agentCfg)

	// Set callbacks for terminal output
	ag.SetCallbacks(
		func(toolName, desc string) bool {
			fmt.Printf("🔐 Allow %s? [y/N] ", desc)
			var response string
			fmt.Scanln(&response)
			return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
		},
		func(token string) { fmt.Print(token) },          // onToken
		func(token string) { /* thinking — hide for now */ }, // onThinking
		func(name, args string) {
			fmt.Printf("\n⚙️  %s\n", name)
		},
		func(name, result string, err error) {
			if err != nil {
				fmt.Printf("   ❌ %s: %s\n", name, err)
			} else {
				fmt.Printf("   ✅ %s\n", name)
			}
		},
		func(status string) { fmt.Println(status) },
	)

	// Run agent
	fmt.Printf("🔨 [%s mode] %s\n\n", mode, truncate(prompt, 120))
	if err := ag.Run(cmd.Context(), sessionID, prompt); err != nil {
		fmt.Printf("\n❌ %s\n", err)
		return err
	}

	// Print summary
	sessionCost, _ := database.GetSessionCost(cmd.Context(), sessionID)
	fmt.Printf("\n\n✅ Done (cost: $%.4f)\n", sessionCost)

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
			cfg, err := a.config.Get()
			if err != nil {
				return err
			}

			database, err := db.Open(cfg.DataDir+"/kov.db", a.logger)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			mgr := session.NewManager(database, a.bus, a.logger, "")

			var sessionID string
			if len(args) > 0 {
				sessionID = args[0]
				fmt.Printf("♻️  Resuming session: %s\n", sessionID)
			} else {
				last, err := mgr.GetLastInterrupted(cmd.Context())
				if err != nil {
					fmt.Println("No interrupted sessions found.")
					return nil
				}
				sessionID = last.ID
				fmt.Printf("♻️  Resuming session: %s (%s)\n", sessionID[:8], last.Prompt)
			}

			providers := buildProviders(cfg)
			router := provider.NewRouter(providers, a.bus, a.logger, provider.DefaultRouterConfig())
			defer router.Close()

			projectDir, _ := os.Getwd()
			toolRegistry := tools.DefaultRegistry(projectDir)

			agentCfg := agent.DefaultAgentConfig()
			agentCfg.Permissions = cfg.Permissions

			err = mgr.Resume(cmd.Context(), sessionID, router, toolRegistry, agentCfg, session.AgentCallbacks{
				OnPermission: func(name, desc string) bool {
					fmt.Printf("🔐 Allow %s? [y/N] ", desc)
					var r string
					fmt.Scanln(&r)
					return strings.ToLower(r) == "y"
				},
				OnToken:  func(t string) { fmt.Print(t) },
				OnStatus: func(s string) { fmt.Println(s) },
			})
			if err != nil {
				fmt.Printf("\n❌ %s\n", err)
				return err
			}

			sessionCost, _ := database.GetSessionCost(cmd.Context(), sessionID)
			fmt.Printf("\n✅ Resumed session complete (cost: $%.4f)\n", sessionCost)
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
			cfg, err := a.config.Get()
			if err != nil {
				return err
			}

			database, err := db.Open(cfg.DataDir+"/kov.db", a.logger)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			mgr := session.NewManager(database, a.bus, a.logger, "")
			sessions, err := mgr.List(cmd.Context(), 20)
			if err != nil {
				return err
			}

			if len(sessions) == 0 {
				fmt.Println("No sessions found. Start one with: kov \"your prompt\"")
				return nil
			}

			fmt.Printf("%-8s  %-8s  %-10s  %-6s  %s\n", "ID", "MODE", "STATE", "COST", "PROMPT")
			fmt.Println(strings.Repeat("─", 70))
			for _, s := range sessions {
				prompt := s.Prompt
				if len(prompt) > 40 {
					prompt = prompt[:37] + "..."
				}
				stateEmoji := "⚪"
				switch s.State {
				case "done":
					stateEmoji = "✅"
				case "executing", "planning", "verifying":
					stateEmoji = "🔄"
				case "error_wait", "paused":
					stateEmoji = "⏸️"
				}
				fmt.Printf("%-8s  %-8s  %s %-8s  $%.2f  %s\n",
					s.ID[:8], s.Mode, stateEmoji, s.State, s.Cost, prompt)
			}
			return nil
		},
	}

	// Add delete subcommand
	cmd.AddCommand(&cobra.Command{
		Use:   "delete [session-id]",
		Short: "Delete a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.config.Get()
			if err != nil {
				return err
			}
			database, err := db.Open(cfg.DataDir+"/kov.db", a.logger)
			if err != nil {
				return err
			}
			defer database.Close()

			mgr := session.NewManager(database, a.bus, a.logger, "")
			if err := mgr.Delete(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Printf("🗑️  Session %s deleted\n", args[0])
			return nil
		},
	})

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
