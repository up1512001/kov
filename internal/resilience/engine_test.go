package resilience

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/utsavkovy/kov/internal/bus"
	"github.com/utsavkovy/kov/internal/db"
)

func newTestEngine(t *testing.T) (*Engine, *db.DB) {
	t.Helper()
	database := newTestDB(t)
	eventBus := bus.New()
	t.Cleanup(func() { eventBus.Close() })
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	engine := NewEngine(database, eventBus, logger, DefaultEngineConfig())
	return engine, database
}

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(dir+"/test.db", slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestEngine_StateTransitions(t *testing.T) {
	engine, database := newTestEngine(t)
	ctx := context.Background()

	// Create a session
	sessionID, _ := database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})
	engine.BindSession(sessionID)

	// Test transitions
	if engine.State() != StateIdle {
		t.Errorf("expected idle, got %s", engine.State())
	}

	engine.Transition(ctx, StatePlanning)
	if engine.State() != StatePlanning {
		t.Errorf("expected planning, got %s", engine.State())
	}

	engine.Transition(ctx, StateExecuting)
	if engine.State() != StateExecuting {
		t.Errorf("expected executing, got %s", engine.State())
	}

	engine.Transition(ctx, StateVerifying)
	engine.Transition(ctx, StateDone)
	if engine.State() != StateDone {
		t.Errorf("expected done, got %s", engine.State())
	}

	// Verify session state in DB
	session, err := database.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("getting session: %v", err)
	}
	if session.State != "done" {
		t.Errorf("expected DB state 'done', got %s", session.State)
	}
}

func TestEngine_Checkpoint(t *testing.T) {
	engine, database := newTestEngine(t)
	ctx := context.Background()

	sessionID, _ := database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})
	engine.BindSession(sessionID)

	// Transition creates checkpoint
	engine.Transition(ctx, StateExecuting)

	// Verify checkpoint was created
	cp, err := database.GetLatestCheckpoint(ctx, sessionID)
	if err != nil {
		t.Fatalf("getting checkpoint: %v", err)
	}
	if cp.State != "executing" {
		t.Errorf("expected checkpoint state 'executing', got %s", cp.State)
	}
}

func TestEngine_RecoverFromCheckpoint(t *testing.T) {
	engine1, database := newTestEngine(t)
	ctx := context.Background()

	sessionID, _ := database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})
	engine1.BindSession(sessionID)

	// Simulate work: transition + cost
	engine1.Transition(ctx, StateExecuting)
	engine1.RecordCost(ctx, 1.50, "anthropic", "claude-sonnet-4", 10000, 5000)

	// "Crash" — create a new engine and recover
	engine2, _ := newTestEngine(t)
	engine2.db = database
	err := engine2.Recover(ctx, sessionID)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}

	if engine2.State() != StateExecuting {
		t.Errorf("expected recovered state 'executing', got %s", engine2.State())
	}
}

func TestEngine_BudgetEnforcement(t *testing.T) {
	engine, database := newTestEngine(t)
	ctx := context.Background()

	engine.config.BudgetPerSession = 1.0
	engine.budget = NewBudgetMonitor(1.0, 0.5)

	sessionID, _ := database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})
	engine.BindSession(sessionID)
	engine.Transition(ctx, StateExecuting)

	// Under budget — should succeed
	err := engine.RecordCost(ctx, 0.50, "anthropic", "claude-sonnet-4", 5000, 2500)
	if err != nil {
		t.Fatalf("unexpected error under budget: %v", err)
	}

	// Over budget — should fail and pause
	err = engine.RecordCost(ctx, 0.60, "anthropic", "claude-sonnet-4", 5000, 2500)
	if err == nil {
		t.Fatal("expected budget error")
	}
	if engine.State() != StatePaused {
		t.Errorf("expected paused after budget exceeded, got %s", engine.State())
	}
}

func TestLoopDetector(t *testing.T) {
	det := NewLoopDetector(3)

	// Not a loop yet
	if det.Record("edit main.go") {
		t.Error("should not detect loop on first action")
	}
	if det.Record("edit main.go") {
		t.Error("should not detect loop on second action")
	}

	// Third repetition → loop!
	if !det.Record("edit main.go") {
		t.Error("should detect loop on third repetition")
	}

	// Reset and verify
	det.Reset()
	if det.Count() != 0 {
		t.Error("expected empty history after reset")
	}
}

func TestLoopDetector_DifferentActions(t *testing.T) {
	det := NewLoopDetector(3)

	det.Record("edit main.go")
	det.Record("edit utils.go")
	det.Record("edit main.go")
	det.Record("edit utils.go")
	det.Record("edit main.go")

	// Different actions alternating — should NOT trigger loop
	// because the pattern check looks at the last 'threshold' entries
	// not alternating patterns
}

func TestBudgetMonitor(t *testing.T) {
	b := NewBudgetMonitor(10.0, 5.0)

	if b.IsExceeded() {
		t.Error("should not be exceeded at start")
	}
	if b.Remaining() != 10.0 {
		t.Errorf("expected 10.0 remaining, got %f", b.Remaining())
	}

	b.Add(3.0)
	if b.Current() != 3.0 {
		t.Errorf("expected 3.0, got %f", b.Current())
	}

	// Warning threshold
	if b.ShouldWarn() {
		t.Error("should not warn at 3.0")
	}
	b.Add(3.0) // Now at 6.0
	if !b.ShouldWarn() {
		t.Error("should warn at 6.0 (warnAt=5.0)")
	}
	// Shouldn't warn again
	if b.ShouldWarn() {
		t.Error("should only warn once")
	}

	// Budget exceeded
	b.Add(5.0) // Now at 11.0
	if !b.IsExceeded() {
		t.Error("should be exceeded at 11.0")
	}
	if b.Remaining() != 0 {
		t.Errorf("expected 0 remaining, got %f", b.Remaining())
	}
}

func TestBudgetMonitor_Unlimited(t *testing.T) {
	b := NewBudgetMonitor(0, 0) // unlimited

	b.Add(1000.0)
	if b.IsExceeded() {
		t.Error("unlimited budget should never be exceeded")
	}
	if b.Remaining() != -1 {
		t.Errorf("expected -1 for unlimited, got %f", b.Remaining())
	}
}

func TestBudgetMonitor_Set(t *testing.T) {
	b := NewBudgetMonitor(10.0, 5.0)
	b.Set(7.5) // Recovery from checkpoint
	if b.Current() != 7.5 {
		t.Errorf("expected 7.5, got %f", b.Current())
	}
}
