// Package resilience implements the resilience engine — the core differentiator
// of kov. It manages session state machine, checkpointing, loop detection,
// and budget enforcement.
package resilience

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/up1512001/kov/internal/bus"
	"github.com/up1512001/kov/internal/db"
)

// State represents a session's current state in the FSM.
type State string

const (
	StateIdle      State = "idle"
	StatePlanning  State = "planning"
	StateExecuting State = "executing"
	StateVerifying State = "verifying"
	StateErrorWait State = "error_wait"
	StateDone      State = "done"
	StatePaused    State = "paused"
)

// Engine manages the resilience lifecycle of a session.
type Engine struct {
	db        *db.DB
	bus       *bus.Bus
	logger    *slog.Logger
	loopDet   *LoopDetector
	budget    *BudgetMonitor

	sessionID string
	state     State
	mu        sync.RWMutex

	config EngineConfig
}

// EngineConfig holds resilience engine settings.
type EngineConfig struct {
	CheckpointEnabled  bool
	LoopThreshold      int
	BudgetPerSession   float64
	BudgetWarnAt       float64
	MaxFixRetries      int
	VerifyTimeout      time.Duration
}

// DefaultEngineConfig returns default resilience settings.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		CheckpointEnabled: true,
		LoopThreshold:     3,
		BudgetPerSession:  0, // unlimited
		BudgetWarnAt:      5.0,
		MaxFixRetries:     2,
		VerifyTimeout:     120 * time.Second,
	}
}

// NewEngine creates a new resilience engine for a session.
func NewEngine(database *db.DB, eventBus *bus.Bus, logger *slog.Logger, config EngineConfig) *Engine {
	return &Engine{
		db:      database,
		bus:     eventBus,
		logger:  logger,
		loopDet: NewLoopDetector(config.LoopThreshold),
		budget:  NewBudgetMonitor(config.BudgetPerSession, config.BudgetWarnAt),
		state:   StateIdle,
		config:  config,
	}
}

// BindSession binds the engine to a specific session.
func (e *Engine) BindSession(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionID = sessionID
}

// State returns the current session state.
func (e *Engine) State() State {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// Transition moves the session to a new state with a checkpoint.
func (e *Engine) Transition(ctx context.Context, newState State) error {
	e.mu.Lock()
	oldState := e.state
	e.state = newState
	e.mu.Unlock()

	e.logger.Info("state transition",
		slog.String("from", string(oldState)),
		slog.String("to", string(newState)),
		slog.String("session", e.sessionID))

	// Checkpoint on every state transition
	if e.config.CheckpointEnabled && e.sessionID != "" {
		if err := e.checkpoint(ctx, newState); err != nil {
			e.logger.Warn("checkpoint failed", slog.String("error", err.Error()))
			// Don't fail the transition — checkpoint failure is non-fatal
		}
	}

	// Update session state in DB
	if e.sessionID != "" {
		cost := e.budget.Current()
		e.db.UpdateSessionState(ctx, e.sessionID, string(newState), cost)
	}

	// Emit event
	if e.bus != nil {
		e.bus.Publish(bus.StateChanged{
			SessionID: e.sessionID,
			From:      string(oldState),
			To:        string(newState),
		})
	}

	return nil
}

// checkpoint saves the current state for crash recovery.
func (e *Engine) checkpoint(ctx context.Context, state State) error {
	cp := &db.Checkpoint{
		SessionID: e.sessionID,
		State:     string(state),
		Cost:      e.budget.Current(),
	}
	return e.db.SaveCheckpoint(ctx, cp)
}

// Recover loads the last checkpoint and restores session state.
func (e *Engine) Recover(ctx context.Context, sessionID string) error {
	e.BindSession(sessionID)

	cp, err := e.db.GetLatestCheckpoint(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("loading checkpoint: %w", err)
	}

	e.mu.Lock()
	e.state = State(cp.State)
	e.mu.Unlock()

	e.budget.Set(cp.Cost)

	e.logger.Info("recovered session",
		slog.String("session", sessionID),
		slog.String("state", cp.State),
		slog.Float64("cost", cp.Cost))

	return nil
}

// RecordCost tracks API call cost and checks budget.
func (e *Engine) RecordCost(ctx context.Context, cost float64, provider, model string, inputTokens, outputTokens int) error {
	e.budget.Add(cost)

	// Record in DB
	if e.sessionID != "" {
		e.db.RecordCost(ctx, &db.CostEvent{
			SessionID:    e.sessionID,
			Provider:     provider,
			Model:        model,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			Cost:         cost,
		})
	}

	// Emit cost event
	if e.bus != nil {
		e.bus.Publish(bus.CostUpdate{
			SessionID: e.sessionID,
			Total:     e.budget.Current(),
			Delta:     cost,
		})
	}

	// Check budget
	if e.budget.IsExceeded() {
		e.logger.Warn("budget exceeded",
			slog.Float64("total", e.budget.Current()),
			slog.Float64("budget", e.config.BudgetPerSession))
		e.Transition(ctx, StatePaused)
		return fmt.Errorf("budget exceeded: $%.2f / $%.2f", e.budget.Current(), e.config.BudgetPerSession)
	}

	if e.budget.ShouldWarn() {
		e.bus.Publish(bus.CostWarning{
			SessionID: e.sessionID,
			Total:     e.budget.Current(),
			Budget:    e.config.BudgetPerSession,
		})
	}

	return nil
}

// RecordAction records an action for loop detection.
func (e *Engine) RecordAction(action string) bool {
	isLoop := e.loopDet.Record(action)
	if isLoop {
		e.logger.Warn("loop detected",
			slog.String("action", action),
			slog.Int("threshold", e.config.LoopThreshold))
		if e.bus != nil {
			e.bus.Publish(bus.LoopDetected{
				SessionID:   e.sessionID,
				ToolName:    action,
				Repetitions: e.config.LoopThreshold,
			})
		}
	}
	return isLoop
}
