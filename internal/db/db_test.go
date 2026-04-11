package db

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// newTestDB creates an in-memory SQLite database for testing.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGetSession(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	s := &Session{
		ProjectDir: "/tmp/test-project",
		Mode:       "code",
		Provider:   "anthropic",
		Model:      "claude-sonnet-4",
		State:      "idle",
		Prompt:     "refactor auth",
	}

	id, err := db.CreateSession(ctx, s)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty session ID")
	}

	got, err := db.GetSession(ctx, id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Mode != "code" {
		t.Errorf("expected mode 'code', got %q", got.Mode)
	}
	if got.Prompt != "refactor auth" {
		t.Errorf("expected prompt 'refactor auth', got %q", got.Prompt)
	}
	if got.State != "idle" {
		t.Errorf("expected state 'idle', got %q", got.State)
	}
}

func TestUpdateSessionState(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	id, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})

	err := db.UpdateSessionState(ctx, id, "executing", 1.50)
	if err != nil {
		t.Fatalf("UpdateSessionState: %v", err)
	}

	got, _ := db.GetSession(ctx, id)
	if got.State != "executing" {
		t.Errorf("expected state 'executing', got %q", got.State)
	}
	if got.Cost != 1.50 {
		t.Errorf("expected cost 1.50, got %f", got.Cost)
	}
}

func TestListSessions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		db.CreateSession(ctx, &Session{
			ProjectDir: "/tmp/test",
			Mode:       "code",
			State:      "idle",
		})
	}

	sessions, err := db.ListSessions(ctx, 3)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestAddAndGetMessages(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})

	// Add messages
	db.AddMessage(ctx, &Message{
		SessionID:  sessionID,
		Role:       "user",
		Content:    "refactor auth to JWT",
		TokenCount: 10,
	})
	db.AddMessage(ctx, &Message{
		SessionID:  sessionID,
		Role:       "assistant",
		Content:    "I'll help you refactor...",
		TokenCount: 50,
	})
	db.AddMessage(ctx, &Message{
		SessionID:  sessionID,
		Role:       "tool",
		Content:    "file contents...",
		ToolName:   "file_read",
		TokenCount: 200,
	})

	// Get all messages
	msgs, err := db.GetMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %q", msgs[0].Role)
	}
	if msgs[2].ToolName != "file_read" {
		t.Errorf("expected tool name 'file_read', got %q", msgs[2].ToolName)
	}
}

func TestGetRecentMessages_TokenBudget(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})

	// Add 5 messages with 100 tokens each
	for i := 0; i < 5; i++ {
		db.AddMessage(ctx, &Message{
			SessionID:  sessionID,
			Role:       "user",
			Content:    "test message",
			TokenCount: 100,
		})
	}

	// Get with budget of 250 tokens — should get 2 most recent
	msgs, err := db.GetRecentMessages(ctx, sessionID, 250)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages within 250 token budget, got %d", len(msgs))
	}
}

func TestGetMessageCount(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})

	db.AddMessage(ctx, &Message{SessionID: sessionID, Role: "user", Content: "hello", TokenCount: 5})
	db.AddMessage(ctx, &Message{SessionID: sessionID, Role: "assistant", Content: "hi", TokenCount: 3})

	count, err := db.GetMessageCount(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetMessageCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	total, err := db.GetTotalTokens(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTotalTokens: %v", err)
	}
	if total != 8 {
		t.Errorf("expected 8 tokens, got %d", total)
	}
}

func TestCreateAndGetTasks(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "planning",
	})

	// Create tasks in batch
	tasks := []*Task{
		{SessionID: sessionID, Seq: 1, Description: "Analyze current auth", Status: TaskStatusPending},
		{SessionID: sessionID, Seq: 2, Description: "Create JWT utils", Status: TaskStatusPending},
		{SessionID: sessionID, Seq: 3, Description: "Update middleware", Status: TaskStatusPending},
	}
	err := db.CreateTasks(ctx, tasks)
	if err != nil {
		t.Fatalf("CreateTasks: %v", err)
	}

	// Get all tasks
	got, err := db.GetTasks(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(got))
	}
	if got[0].Description != "Analyze current auth" {
		t.Errorf("expected first task to be 'Analyze current auth', got %q", got[0].Description)
	}

	// Update status
	err = db.UpdateTaskStatus(ctx, got[0].ID, TaskStatusDone, "completed analysis", "")
	if err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	// Check pending
	pending, err := db.GetPendingTasks(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetPendingTasks: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(pending))
	}

	// Check stats
	total, done, failed, err := db.GetTaskStats(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskStats: %v", err)
	}
	if total != 3 || done != 1 || failed != 0 {
		t.Errorf("expected stats 3/1/0, got %d/%d/%d", total, done, failed)
	}
}

func TestCheckpointSaveAndRestore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "executing",
	})

	// Save checkpoint
	cp := &Checkpoint{
		SessionID:      sessionID,
		State:          "executing",
		TaskID:         "task-123",
		ContextSummary: `{"summary": "working on JWT"}`,
		ProviderID:     "anthropic",
		Model:          "claude-sonnet-4",
		RetryCount:     0,
		Cost:           2.50,
	}
	err := db.SaveCheckpoint(ctx, cp)
	if err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	// Restore checkpoint
	got, err := db.GetLatestCheckpoint(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint: %v", err)
	}
	if got.State != "executing" {
		t.Errorf("expected state 'executing', got %q", got.State)
	}
	if got.TaskID != "task-123" {
		t.Errorf("expected task ID 'task-123', got %q", got.TaskID)
	}
	if got.ProviderID != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", got.ProviderID)
	}
	if got.Cost != 2.50 {
		t.Errorf("expected cost 2.50, got %f", got.Cost)
	}
}

func TestRecordAndGetCost(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "executing",
	})

	// Record costs
	db.RecordCost(ctx, &CostEvent{
		SessionID:    sessionID,
		Provider:     "anthropic",
		Model:        "claude-sonnet-4",
		InputTokens:  1000,
		OutputTokens: 500,
		Cost:         0.015,
	})
	db.RecordCost(ctx, &CostEvent{
		SessionID:    sessionID,
		Provider:     "anthropic",
		Model:        "claude-sonnet-4",
		InputTokens:  2000,
		OutputTokens: 800,
		Cost:         0.025,
	})

	// Check total
	total, err := db.GetSessionCost(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSessionCost: %v", err)
	}
	if total < 0.039 || total > 0.041 { // float comparison
		t.Errorf("expected cost ~0.04, got %f", total)
	}

	// Verify session cost was updated
	session, _ := db.GetSession(ctx, sessionID)
	if session.Cost < 0.039 || session.Cost > 0.041 {
		t.Errorf("expected session cost ~0.04, got %f", session.Cost)
	}

	// Check events list
	events, err := db.GetCostEvents(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetCostEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 cost events, got %d", len(events))
	}
}

func TestDeleteSession_Cascade(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sessionID, _ := db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "idle",
	})

	// Add associated data
	db.AddMessage(ctx, &Message{SessionID: sessionID, Role: "user", Content: "test", TokenCount: 5})
	db.CreateTask(ctx, &Task{SessionID: sessionID, Seq: 1, Description: "task 1", Status: TaskStatusPending})
	db.SaveCheckpoint(ctx, &Checkpoint{SessionID: sessionID, State: "executing"})
	db.RecordCost(ctx, &CostEvent{SessionID: sessionID, Provider: "anthropic", Model: "test", Cost: 0.01})

	// Delete session
	err := db.DeleteSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// Verify all cascade-deleted
	msgs, _ := db.GetMessages(ctx, sessionID)
	if len(msgs) != 0 {
		t.Error("expected messages to be cascade deleted")
	}

	tasks, _ := db.GetTasks(ctx, sessionID)
	if len(tasks) != 0 {
		t.Error("expected tasks to be cascade deleted")
	}
}

func TestGetLastInterruptedSession(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Create some sessions
	db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "done",
		Prompt:     "completed task",
	})

	db.CreateSession(ctx, &Session{
		ProjectDir: "/tmp/test",
		Mode:       "code",
		State:      "executing",
		Prompt:     "interrupted task",
	})

	s, err := db.GetLastInterruptedSession(ctx, "/tmp/test")
	if err != nil {
		t.Fatalf("GetLastInterruptedSession: %v", err)
	}
	if s.State != "executing" {
		t.Errorf("expected state 'executing', got %q", s.State)
	}
	if s.Prompt != "interrupted task" {
		t.Errorf("expected prompt 'interrupted task', got %q", s.Prompt)
	}
}
