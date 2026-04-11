package session

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/utsavkovy/kov/internal/bus"
	"github.com/utsavkovy/kov/internal/db"
)

func newTestManager(t *testing.T) (*Manager, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	database, err := db.Open(dir+"/test.db", logger)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	eventBus := bus.New()
	t.Cleanup(func() { eventBus.Close() })

	return NewManager(database, eventBus, logger, "/tmp/test"), database
}

func TestList_Empty(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessions, err := mgr.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestList_WithSessions(t *testing.T) {
	mgr, database := newTestManager(t)
	ctx := context.Background()

	// Create sessions
	database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/a", Mode: "code", State: "done", Prompt: "fix bug",
	})
	database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/b", Mode: "plan", State: "executing", Prompt: "analyze code",
	})

	sessions, err := mgr.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestDelete(t *testing.T) {
	mgr, database := newTestManager(t)
	ctx := context.Background()

	sessionID, _ := database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test", Mode: "code", State: "done", Prompt: "test",
	})

	// Add some messages
	database.AddMessage(ctx, &db.Message{
		SessionID: sessionID, Role: "user", Content: "hello",
	})

	err := mgr.Delete(ctx, sessionID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	_, err = database.GetSession(ctx, sessionID)
	if err == nil {
		t.Error("session should be deleted")
	}
}

func TestGetLastInterrupted(t *testing.T) {
	mgr, database := newTestManager(t)
	ctx := context.Background()

	// Create a done session
	database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test", Mode: "code", State: "done", Prompt: "done task",
	})

	// Create an interrupted session (same project dir as manager)
	database.CreateSession(ctx, &db.Session{
		ProjectDir: "/tmp/test", Mode: "code", State: "executing", Prompt: "interrupted task",
	})

	session, err := mgr.GetLastInterrupted(ctx)
	if err != nil {
		t.Fatalf("GetLastInterrupted: %v", err)
	}
	if session.Prompt != "interrupted task" {
		t.Errorf("expected 'interrupted task', got %q", session.Prompt)
	}
}
