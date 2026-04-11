package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Message represents a single conversation message stored in the DB.
type Message struct {
	ID         int64
	SessionID  string
	Role       string // system, user, assistant, tool
	Content    string
	ToolName   string
	ToolCallID string
	ToolArgs   string
	TokenCount int
	Cost       float64
	CreatedAt  time.Time
}

// AddMessage inserts a message into the conversation history.
func (db *DB) AddMessage(ctx context.Context, m *Message) (int64, error) {
	var id int64
	err := db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO messages (session_id, role, content, tool_name, tool_call_id, tool_args, token_count, cost)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			m.SessionID, m.Role, m.Content, m.ToolName, m.ToolCallID, m.ToolArgs, m.TokenCount, m.Cost,
		)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("adding message: %w", err)
	}
	return id, nil
}

// GetMessages retrieves all messages for a session, ordered chronologically.
func (db *DB) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	rows, err := db.readDB.QueryContext(ctx,
		`SELECT id, session_id, role, content, tool_name, tool_call_id, tool_args, token_count, cost, created_at
		 FROM messages WHERE session_id = ? ORDER BY created_at ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetRecentMessages retrieves the most recent messages up to a token budget.
// This is the core context assembly function — keeps history in DB, loads on demand.
func (db *DB) GetRecentMessages(ctx context.Context, sessionID string, maxTokens int) ([]*Message, error) {
	// First get total count and tokens
	rows, err := db.readDB.QueryContext(ctx,
		`SELECT id, session_id, role, content, tool_name, tool_call_id, tool_args, token_count, cost, created_at
		 FROM messages WHERE session_id = ? ORDER BY created_at DESC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting recent messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	totalTokens := 0

	for rows.Next() {
		m := &Message{}
		var createdAt string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolName, &m.ToolCallID, &m.ToolArgs, &m.TokenCount, &m.Cost, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)

		totalTokens += m.TokenCount
		if maxTokens > 0 && totalTokens > maxTokens {
			break
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// GetMessageCount returns the number of messages in a session.
func (db *DB) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := db.readDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID,
	).Scan(&count)
	return count, err
}

// GetTotalTokens returns the total token count for a session.
func (db *DB) GetTotalTokens(ctx context.Context, sessionID string) (int, error) {
	var total int
	err := db.readDB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(token_count), 0) FROM messages WHERE session_id = ?`, sessionID,
	).Scan(&total)
	return total, err
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	var messages []*Message
	for rows.Next() {
		m := &Message{}
		var createdAt string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolName, &m.ToolCallID, &m.ToolArgs, &m.TokenCount, &m.Cost, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
