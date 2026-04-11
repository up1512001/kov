// Package session manages the session lifecycle — listing,
// resuming, and deleting sessions stored in SQLite.
package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/up1512001/kov/internal/agent"
	"github.com/up1512001/kov/internal/bus"
	"github.com/up1512001/kov/internal/db"
	"github.com/up1512001/kov/internal/provider"
	"github.com/up1512001/kov/internal/resilience"
	"github.com/up1512001/kov/internal/tools"
)

// Manager handles session operations.
type Manager struct {
	db         *db.DB
	bus        *bus.Bus
	logger     *slog.Logger
	projectDir string
}

// NewManager creates a session manager.
func NewManager(database *db.DB, eventBus *bus.Bus, logger *slog.Logger, projectDir string) *Manager {
	return &Manager{
		db:         database,
		bus:        eventBus,
		logger:     logger,
		projectDir: projectDir,
	}
}

// SessionInfo is a display-friendly session summary.
type SessionInfo struct {
	ID        string
	Mode      string
	State     string
	Prompt    string
	Provider  string
	Model     string
	Cost      float64
	CreatedAt time.Time
	UpdatedAt time.Time
	Messages  int
}

// List returns all sessions, most recent first.
func (m *Manager) List(ctx context.Context, limit int) ([]SessionInfo, error) {
	sessions, err := m.db.ListSessions(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	infos := make([]SessionInfo, len(sessions))
	for i, s := range sessions {
		msgCount, _ := m.db.GetMessageCount(ctx, s.ID)
		infos[i] = SessionInfo{
			ID:        s.ID,
			Mode:      s.Mode,
			State:     s.State,
			Prompt:    s.Prompt,
			Provider:  s.Provider,
			Model:     s.Model,
			Cost:      s.Cost,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
			Messages:  msgCount,
		}
	}

	return infos, nil
}

// Resume recovers an interrupted session and continues the agent loop.
func (m *Manager) Resume(
	ctx context.Context,
	sessionID string,
	router *provider.Router,
	toolRegistry *tools.Registry,
	agentCfg agent.AgentConfig,
	callbacks AgentCallbacks,
) error {
	// If no session ID, find the last interrupted one
	if sessionID == "" {
		session, err := m.db.GetLastInterruptedSession(ctx, m.projectDir)
		if err != nil {
			return fmt.Errorf("no interrupted sessions found")
		}
		sessionID = session.ID
	}

	// Load session
	session, err := m.db.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	m.logger.Info("resuming session",
		slog.String("session", sessionID),
		slog.String("state", session.State),
		slog.String("prompt", session.Prompt))

	// Initialize resilience engine and recover from checkpoint
	engine := resilience.NewEngine(m.db, m.bus, m.logger, resilience.EngineConfig{
		CheckpointEnabled: true,
		LoopThreshold:     3,
	})

	if err := engine.Recover(ctx, sessionID); err != nil {
		m.logger.Warn("checkpoint recovery failed, starting fresh",
			slog.String("error", err.Error()))
		engine.BindSession(sessionID)
	}

	// Emit resume event
	m.bus.Publish(bus.SessionResumed{
		SessionID: sessionID,
	})

	// Update agent config from session
	agentCfg.Mode = session.Mode
	agentCfg.Model = session.Model

	// Create and run agent
	ag := agent.New(m.db, router, toolRegistry, engine, m.bus, m.logger, agentCfg)
	ag.SetCallbacks(
		callbacks.OnPermission,
		callbacks.OnToken,
		callbacks.OnThinking,
		callbacks.OnToolCall,
		callbacks.OnToolResult,
		callbacks.OnStatus,
	)

	// Resume with the original prompt — the agent will pick up
	// the existing conversation history from the DB
	return ag.Run(ctx, sessionID, session.Prompt)
}

// Delete removes a session and all its data (cascading).
func (m *Manager) Delete(ctx context.Context, sessionID string) error {
	if err := m.db.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	m.logger.Info("session deleted", slog.String("session", sessionID))
	return nil
}

// GetLastInterrupted returns the most recent interrupted session.
func (m *Manager) GetLastInterrupted(ctx context.Context) (*db.Session, error) {
	return m.db.GetLastInterruptedSession(ctx, m.projectDir)
}

// AgentCallbacks groups the UI callback functions for the session manager.
type AgentCallbacks struct {
	OnPermission func(toolName, description string) bool
	OnToken      func(token string)
	OnThinking   func(token string)
	OnToolCall   func(name string, args string)
	OnToolResult func(name string, result string, err error)
	OnStatus     func(status string)
}
