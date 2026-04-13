package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/up1512001/kov/internal/bus"
	"github.com/up1512001/kov/internal/db"
	"github.com/up1512001/kov/internal/provider"
	"github.com/up1512001/kov/internal/resilience"
	"github.com/up1512001/kov/internal/tokens"
	"github.com/up1512001/kov/internal/tools"
)

// ArchitectRunner implements the two-phase architect mode:
//   Phase 1: Planning — uses the plan model (read-only tools, extended thinking)
//            to deeply analyze the codebase and produce a structured plan.
//   Phase 2: Editing — uses the edit model (full tool access) to execute
//            the plan step by step.
//
// This is what differentiates kov from competitors — none of them implement
// actual two-phase execution. Claude Code's "plan mode" is read-only only.
// Cursor's agent mode is single-pass. Aider has no plan→execute pipeline.
type ArchitectRunner struct {
	db        *db.DB
	router    *provider.Router
	tools     *tools.Registry
	engine    *resilience.Engine
	bus       *bus.Bus
	logger    *slog.Logger
	sessionID string

	planModel  string
	editModel  string
	config     AgentConfig

	// Callbacks
	onToken    func(string)
	onThinking func(string)
	onToolCall func(string, string)
	onToolResult func(string, string, error)
	onStatus   func(string)
	onPermission func(string, string) bool
	onWindowFill func(tokens.WindowFill)
}

// NewArchitectRunner creates a two-phase architect agent.
func NewArchitectRunner(
	database *db.DB,
	router *provider.Router,
	toolRegistry *tools.Registry,
	engine *resilience.Engine,
	eventBus *bus.Bus,
	logger *slog.Logger,
	config AgentConfig,
	planModel, editModel string,
) *ArchitectRunner {
	return &ArchitectRunner{
		db:        database,
		router:    router,
		tools:     toolRegistry,
		engine:    engine,
		bus:       eventBus,
		logger:    logger,
		config:    config,
		planModel: planModel,
		editModel: editModel,
	}
}

// SetCallbacks sets the UI callbacks (same signature as Agent).
func (ar *ArchitectRunner) SetCallbacks(
	onPermission func(string, string) bool,
	onToken func(string),
	onThinking func(string),
	onToolCall func(string, string),
	onToolResult func(string, string, error),
	onStatus func(string),
) {
	ar.onPermission = onPermission
	ar.onToken = onToken
	ar.onThinking = onThinking
	ar.onToolCall = onToolCall
	ar.onToolResult = onToolResult
	ar.onStatus = onStatus
}

// SetWindowFillCallback sets the context window fill callback.
func (ar *ArchitectRunner) SetWindowFillCallback(fn func(tokens.WindowFill)) {
	ar.onWindowFill = fn
}

// Run executes the two-phase architect workflow.
func (ar *ArchitectRunner) Run(ctx context.Context, sessionID string, prompt string) error {
	ar.sessionID = sessionID

	// ── Phase 1: Planning ──────────────────────────────────────
	ar.emit("Phase 1: Planning (read-only analysis)...")
	ar.bus.Publish(bus.SessionStarted{SessionID: sessionID, Mode: "architect-plan"})

	planCfg := ar.config
	planCfg.Mode = "plan" // read-only tools only
	planCfg.Model = ar.planModel
	planCfg.MaxIterations = 20 // planning shouldn't need 50 iterations

	planAgent := New(ar.db, ar.router, ar.tools, ar.engine, ar.bus, ar.logger, planCfg)

	// Collect the plan output
	var planBuilder strings.Builder
	planAgent.SetCallbacks(
		ar.onPermission,
		func(token string) {
			planBuilder.WriteString(token)
			if ar.onToken != nil {
				ar.onToken(token)
			}
		},
		ar.onThinking,
		ar.onToolCall,
		ar.onToolResult,
		ar.onStatus,
	)
	if ar.onWindowFill != nil {
		planAgent.SetWindowFillCallback(ar.onWindowFill)
	}

	planPrompt := fmt.Sprintf(`Analyze the following request and create a detailed implementation plan.
Do NOT make any changes — only read files and analyze.

Your output must be a structured plan with:
1. A summary of what needs to change
2. A list of specific files to modify/create, with descriptions of changes
3. The order of operations (which changes depend on others)
4. Any risks or edge cases to watch for

Request: %s`, prompt)

	if err := planAgent.Run(ctx, sessionID, planPrompt); err != nil {
		return fmt.Errorf("architect plan phase: %w", err)
	}

	plan := planBuilder.String()
	if plan == "" {
		return fmt.Errorf("architect plan phase produced no output")
	}

	ar.logger.Info("architect plan complete",
		slog.Int("plan_length", len(plan)),
		slog.String("plan_model", ar.planModel))

	// ── Phase 2: Editing ──────────────────────────────────────
	ar.emit("\nPhase 2: Implementing the plan...")

	editCfg := ar.config
	editCfg.Mode = "code" // full tool access
	editCfg.Model = ar.editModel
	editCfg.MaxIterations = ar.config.MaxIterations

	editAgent := New(ar.db, ar.router, ar.tools, ar.engine, ar.bus, ar.logger, editCfg)
	editAgent.SetCallbacks(
		ar.onPermission,
		ar.onToken,
		ar.onThinking,
		ar.onToolCall,
		ar.onToolResult,
		ar.onStatus,
	)
	if ar.onWindowFill != nil {
		editAgent.SetWindowFillCallback(ar.onWindowFill)
	}

	editPrompt := fmt.Sprintf(`Execute the following implementation plan precisely. Follow each step in order.

## Original Request
%s

## Implementation Plan
%s

Execute each step of the plan. After making changes, verify they work.`, prompt, plan)

	if err := editAgent.Run(ctx, sessionID+"-edit", editPrompt); err != nil {
		return fmt.Errorf("architect edit phase: %w", err)
	}

	ar.emit("Architect mode complete: plan executed successfully.")
	return nil
}

func (ar *ArchitectRunner) emit(status string) {
	if ar.onStatus != nil {
		ar.onStatus(status)
	}
}
