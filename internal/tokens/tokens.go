// Package tokens implements token counting and context window management.
// It provides estimation-based counting (no CGO tiktoken dependency) and
// automatic context compaction when conversations exceed the window.
package tokens

import (
	"strings"
	"unicode/utf8"
)

// Counter estimates token counts for text content.
// Uses a hybrid approach: character-based estimation for speed,
// with adjustments for code vs prose.
type Counter struct {
	// charsPerToken is the average characters per token.
	// English prose ≈ 4.0, code ≈ 3.5, CJK ≈ 1.5
	charsPerToken float64
}

// NewCounter creates a new token counter with default settings.
func NewCounter() *Counter {
	return &Counter{
		charsPerToken: 3.8, // weighted average for code + prose
	}
}

// Count estimates the token count for a string.
func (c *Counter) Count(text string) int {
	if text == "" {
		return 0
	}

	charCount := utf8.RuneCountInString(text)
	estimate := float64(charCount) / c.charsPerToken

	// Adjust for special patterns
	// Code has more short tokens (symbols, brackets, etc)
	codeIndicators := strings.Count(text, "{") + strings.Count(text, "}") +
		strings.Count(text, "(") + strings.Count(text, ")") +
		strings.Count(text, ";") + strings.Count(text, "//")

	if codeIndicators > 5 {
		// Code-heavy content has ~3.5 chars per token
		estimate = float64(charCount) / 3.5
	}

	// Round up
	return int(estimate) + 1
}

// CountMessages estimates total tokens for a message array.
func (c *Counter) CountMessages(messages []MessageTokens) int {
	total := 0
	for _, m := range messages {
		total += m.Tokens
		total += 4 // per-message overhead (~4 tokens for role, separators)
	}
	return total + 3 // final assistant priming
}

// MessageTokens holds token info for a single message.
type MessageTokens struct {
	Role    string
	Content string
	Tokens  int
}

// Compactor manages context window budgets and auto-compaction.
type Compactor struct {
	counter    *Counter
	maxTokens  int
	reserveOut int // tokens reserved for output
}

// NewCompactor creates a compactor for a given context window.
func NewCompactor(maxContextTokens, reserveOutput int) *Compactor {
	return &Compactor{
		counter:    NewCounter(),
		maxTokens:  maxContextTokens,
		reserveOut: reserveOutput,
	}
}

// Budget returns the available token budget for input.
func (c *Compactor) Budget() int {
	return c.maxTokens - c.reserveOut
}

// NeedsCompaction returns true if the messages exceed the budget.
func (c *Compactor) NeedsCompaction(messages []MessageTokens) bool {
	total := c.counter.CountMessages(messages)
	return total > c.Budget()
}

// Compact reduces messages to fit within the token budget.
// Strategy: keep system + first user message + last N messages.
func (c *Compactor) Compact(messages []MessageTokens) []MessageTokens {
	budget := c.Budget()
	total := c.counter.CountMessages(messages)

	if total <= budget {
		return messages // No compaction needed
	}

	if len(messages) <= 3 {
		return messages // Can't compact further
	}

	// Keep system message (index 0) and first user message (index 1)
	// Then keep as many recent messages as fit
	headerTokens := 0
	headerEnd := 0
	for i := 0; i < len(messages) && i < 2; i++ {
		headerTokens += messages[i].Tokens + 4
		headerEnd = i + 1
	}

	// Fill from the end
	remaining := budget - headerTokens - 50 // 50 token buffer for compaction notice
	var tail []MessageTokens
	for i := len(messages) - 1; i >= headerEnd; i-- {
		msgCost := messages[i].Tokens + 4
		if remaining-msgCost < 0 {
			break
		}
		remaining -= msgCost
		tail = append([]MessageTokens{messages[i]}, tail...)
	}

	// Build compacted result
	result := make([]MessageTokens, 0, len(tail)+headerEnd+1)
	result = append(result, messages[:headerEnd]...)

	// Add compaction notice
	droppedCount := len(messages) - headerEnd - len(tail)
	if droppedCount > 0 {
		result = append(result, MessageTokens{
			Role:    "system",
			Content: "[Note: Earlier conversation messages were compacted to fit context window. The most recent messages are preserved.]",
			Tokens:  25,
		})
	}

	result = append(result, tail...)
	return result
}

// EstimateModelWindow returns the known context window for a model.
func EstimateModelWindow(model string) int {
	// Known context windows (as of 2025)
	windows := map[string]int{
		"claude-sonnet-4-20250514":   200000,
		"claude-3-5-sonnet-20241022": 200000,
		"claude-3-5-haiku-20241022":  200000,
		"claude-3-opus-20240229":     200000,
		"gpt-4o":                     128000,
		"gpt-4o-mini":                128000,
		"gpt-4-turbo":                128000,
		"o1":                         200000,
		"o1-mini":                    128000,
		"o3-mini":                    200000,
		"gemini-2.5-pro":             1000000,
		"gemini-2.5-flash":           1000000,
		"gemini-2.0-flash":           1000000,
	}

	if w, ok := windows[model]; ok {
		return w
	}

	// Default to 128K for unknown models
	return 128000
}
