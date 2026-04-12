// Package app provides the top-level application orchestrator for kov.
// It wires together all subsystems using lazy initialization to achieve
// sub-50ms cold start.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/up1512001/kov/internal/agent"
	"github.com/up1512001/kov/internal/bus"
	"github.com/up1512001/kov/internal/config"
	"github.com/up1512001/kov/internal/db"
	"github.com/up1512001/kov/internal/provider"
	"github.com/up1512001/kov/internal/resilience"
	"github.com/up1512001/kov/internal/session"
	"github.com/up1512001/kov/internal/tools"
	"github.com/up1512001/kov/internal/tmux"
	"github.com/up1512001/kov/internal/tui"
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
		a.buildSetupCmd(),
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
	flags.Bool("tmux", false, "Run session in an isolated tmux window")
	flags.String("profile", "", "Enable profiling: cpu, mem, trace")

	return cmd
}

// runRoot handles the main `kov [prompt]` command.
func (a *App) runRoot(cmd *cobra.Command, args []string) error {
	// Handle --tmux flag: re-exec inside an isolated tmux session
	useTmux, _ := cmd.Flags().GetBool("tmux")
	if useTmux && !tmux.IsInsideSession() {
		if !tmux.IsAvailable() {
			return fmt.Errorf("tmux is not installed. Install it with: brew install tmux")
		}
		sessionName := "kov-session"
		if len(args) > 0 {
			// Use a short hash of the prompt for the session name
			sessionName = fmt.Sprintf("kov-%s", truncate(strings.Join(args, "-"), 20))
		}

		if err := tmux.NewSession(sessionName); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}

		// Re-exec kov inside the tmux session without --tmux to avoid recursion
		reCmd := "kov"
		for _, arg := range args {
			reCmd += " " + fmt.Sprintf("%q", arg)
		}
		flagsToForward := []string{"mode", "model", "provider", "permissions"}
		for _, f := range flagsToForward {
			val, _ := cmd.Flags().GetString(f)
			if val != "" {
				reCmd += fmt.Sprintf(" --%s %s", f, val)
			}
		}
		if err := tmux.RunInSession(sessionName, reCmd); err != nil {
			return fmt.Errorf("running in tmux: %w", err)
		}

		fmt.Printf("Started kov in tmux session: %s\n", sessionName)
		fmt.Printf("Attach with: tmux attach -t %s\n", sessionName)
		return tmux.AttachSession(sessionName)
	}

	if len(args) == 0 {
		return a.runInteractive(cmd)
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

// runInteractive starts the interactive REPL mode with Bubble Tea TUI.
func (a *App) runInteractive(cmd *cobra.Command) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		a.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	cfg, err := a.config.Get()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	mode, _ := cmd.Flags().GetString("mode")
	modelFlag, _ := cmd.Flags().GetString("model")
	budgetFlag, _ := cmd.Flags().GetFloat64("budget")

	model := cfg.Model
	if modelFlag != "" {
		model = modelFlag
	}

	budget := cfg.Cost.BudgetPerSession
	if budgetFlag > 0 {
		budget = budgetFlag
	}

	// Build provider chain
	providers := buildProviders(cfg)
	if len(providers) == 0 {
		// No providers — run setup wizard
		wizardCfg, err := runSetupWizard()
		if err != nil {
			return nil
		}
		cfg = wizardCfg
		providers = buildProviders(cfg)
		if len(providers) == 0 {
			return nil
		}
	}

	// Initialize DB
	dbPath := cfg.DataDir + "/kov.db"
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	database, err := db.Open(dbPath, a.logger)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	router := provider.NewRouter(providers, a.bus, a.logger, provider.DefaultRouterConfig())
	defer router.Close()

	projectDir, _ := os.Getwd()
	toolRegistry := tools.DefaultRegistry(projectDir)

	engine := resilience.NewEngine(database, a.bus, a.logger, resilience.EngineConfig{
		CheckpointEnabled: cfg.Resilience.Checkpoint,
		LoopThreshold:     cfg.Resilience.LoopDetection.Threshold,
		BudgetPerSession:  budget,
		BudgetWarnAt:      cfg.Cost.WarnAt,
		MaxFixRetries:     cfg.Verify.MaxFixRetries,
	})

	// Create the wrapper that connects TUI to agent
	promptCh := make(chan agentRequest, 1)
	wrapper := &interactiveWrapper{
		model:    tui.NewInteractiveModel(mode, model, a.build.Version),
		promptCh: promptCh,
	}

	p := tea.NewProgram(wrapper, tea.WithAltScreen())

	// Background agent runner — processes prompts from the TUI
	go func() {
		for req := range promptCh {
			a.runAgentForPrompt(req, p, database, router, toolRegistry, engine, cfg, mode, model, projectDir)
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	close(promptCh)
	return nil
}

// agentRequest represents a prompt submitted from the interactive TUI.
type agentRequest struct {
	prompt string
	ctx    context.Context
	cancel context.CancelFunc
}

// runAgentForPrompt runs the agent loop for a single interactive prompt.
func (a *App) runAgentForPrompt(
	req agentRequest,
	p *tea.Program,
	database *db.DB,
	router *provider.Router,
	toolRegistry *tools.Registry,
	engine *resilience.Engine,
	cfg *config.Config,
	mode, model, projectDir string,
) {
	sessionID, err := database.CreateSession(req.ctx, &db.Session{
		ProjectDir: projectDir,
		Mode:       mode,
		Provider:   cfg.Provider,
		Model:      model,
		State:      "idle",
		Prompt:     req.prompt,
	})
	if err != nil {
		p.Send(tui.ErrorMsg{Error: fmt.Sprintf("creating session: %s", err)})
		return
	}

	agentCfg := agent.DefaultAgentConfig()
	agentCfg.Mode = mode
	agentCfg.Model = model
	agentCfg.Permissions = cfg.Permissions
	agentCfg.VerifyEnabled = cfg.Verify.Enabled
	agentCfg.VerifyCommand = cfg.Verify.Command
	agentCfg.ProjectDir = projectDir

	ag := agent.New(database, router, toolRegistry, engine, a.bus, a.logger, agentCfg)
	ag.SetCallbacks(
		func(toolName, desc string) bool {
			respCh := make(chan bool, 1)
			p.Send(tui.PermissionMsg{
				Tool:        toolName,
				Description: desc,
				ResponseCh:  respCh,
			})
			select {
			case resp := <-respCh:
				return resp
			case <-req.ctx.Done():
				return false
			}
		},
		func(token string) { p.Send(tui.TokenMsg{Token: token}) },
		func(token string) { p.Send(tui.ThinkingMsg{Token: token}) },
		func(name, args string) { p.Send(tui.ToolCallMsg{Name: name}) },
		func(name, result string, err error) {
			p.Send(tui.ToolResultMsg{
				Name:    name,
				Success: err == nil,
				Error:   fmt.Sprintf("%v", err),
			})
		},
		func(status string) { p.Send(tui.StatusMsg{Status: status}) },
	)

	if err := ag.Run(req.ctx, sessionID, req.prompt); err != nil {
		if req.ctx.Err() != nil {
			p.Send(tui.DoneMsg{Cost: 0}) // Cancelled — just return to input
		} else {
			p.Send(tui.ErrorMsg{Error: err.Error()})
		}
		return
	}

	sessionCost, _ := database.GetSessionCost(req.ctx, sessionID)
	p.Send(tui.DoneMsg{Cost: sessionCost})
}

// interactiveWrapper wraps the TUI model to intercept SubmitMsg and CancelMsg
// and route them to the agent goroutine.
type interactiveWrapper struct {
	model    tui.Model
	promptCh chan<- agentRequest
	cancelFn context.CancelFunc
}

func (w *interactiveWrapper) Init() tea.Cmd {
	return w.model.Init()
}

func (w *interactiveWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SubmitMsg:
		// Forward to the TUI model first (to update state)
		result, cmd := w.model.Update(msg)
		w.model = result.(tui.Model)

		// Cancel any running agent
		if w.cancelFn != nil {
			w.cancelFn()
		}

		// Start agent in background
		ctx, cancel := context.WithCancel(context.Background())
		w.cancelFn = cancel

		go func() {
			w.promptCh <- agentRequest{prompt: msg.Text, ctx: ctx, cancel: cancel}
		}()

		return w, cmd

	case tui.CancelMsg:
		if w.cancelFn != nil {
			w.cancelFn()
			w.cancelFn = nil
		}
		// Forward to model to update state
		result, cmd := w.model.Update(tui.ErrorMsg{Error: "Cancelled."})
		w.model = result.(tui.Model)
		return w, cmd
	}

	result, cmd := w.model.Update(msg)
	w.model = result.(tui.Model)
	return w, cmd
}

func (w *interactiveWrapper) View() string {
	return w.model.View()
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

			availableProviders := cfg.GetAvailableProviders()
			if len(availableProviders) == 0 {
				fmt.Println("No providers configured. Set an API key:")
				fmt.Println("  export ANTHROPIC_API_KEY=sk-ant-...  # console.anthropic.com/settings/keys")
				fmt.Println("  export OPENAI_API_KEY=sk-...          # platform.openai.com/api-keys")
				fmt.Println("  export GEMINI_API_KEY=...             # aistudio.google.com/apikey")
				if len(cfg.DetectedCLIs) > 0 {
					fmt.Println()
					fmt.Println("Detected CLI tools (but KOV needs its own API key):")
					for _, cli := range cfg.DetectedCLIs {
						fmt.Printf("  ✓ %s", cli.Name)
						if cli.Version != "" {
							fmt.Printf(" (%s)", cli.Version)
						}
						fmt.Println()
					}
				}
				return nil
			}

			// Show models grouped by provider
			providerModels := map[string][]provider.Model{
				"anthropic": provider.NewAnthropicProvider("", "").Models(),
				"openai":    provider.NewOpenAIProvider("", "").Models(),
				"google":    provider.NewGoogleProvider("", "").Models(),
			}

			fmt.Println("Available providers and models:")
			for _, pid := range availableProviders {
				fmt.Printf("\n  ✓ %s\n", pid)
				if models, ok := providerModels[pid]; ok {
					for _, m := range models {
						defaultMark := " "
						if m.ID == cfg.Model {
							defaultMark = "*"
						}
						fmt.Printf("   %s %-30s  %dk ctx  $%.2f/$%.2f per 1M tokens\n",
							defaultMark, m.ID, m.ContextWindow/1000,
							m.InputCostPer1M, m.OutputCostPer1M)
					}
				}
				if pid == "ollama" {
					if cfg.Providers.Ollama != nil {
						fmt.Printf("    %-30s  (local, free)\n", cfg.Providers.Ollama.Model)
					}
				}
			}
			fmt.Println()
			fmt.Println("  * = current default")
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
			if len(providers) > 0 {
				fmt.Printf("Providers:        %s\n", strings.Join(providers, ", "))
			} else {
				fmt.Printf("Providers:        (none configured)\n")
			}

			if len(cfg.DetectedCLIs) > 0 {
				fmt.Println()
				fmt.Println("Detected CLI tools:")
				for _, cli := range cfg.DetectedCLIs {
					fmt.Printf("  ✓ %s at %s", cli.Name, cli.Path)
					if cli.Version != "" {
						fmt.Printf(" (%s)", cli.Version)
					}
					fmt.Println()
				}
			}

			return nil
		},
	}
}

// buildSetupCmd creates the `kov setup` command for first-run provider configuration.
func (a *App) buildSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure providers interactively",
		Long: `Set up your AI provider for KOV. This guides you through
configuring an API key so KOV can connect to your preferred model.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := runSetupWizard()
			return err
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
