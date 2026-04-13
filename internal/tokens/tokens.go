// Package tokens implements token counting and context window management.
// It provides estimation-based counting (no CGO tiktoken dependency),
// predictive context fill tracking, and proactive compaction before overflow.
package tokens

import (
	"strings"
	"sync"
	"time"
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

// WindowFill represents the current state of context window usage.
type WindowFill struct {
	UsedTokens     int     // tokens currently used
	BudgetTokens   int     // total input budget (window - reserved output)
	FillPercent    float64 // 0.0 to 1.0
	RemainingTokens int    // tokens left before budget
	NeedsCompaction bool   // true if at or over budget
	ShouldCompact  bool   // true if proactive compaction recommended (>=90%)
	MessageCount   int     // total messages in conversation
}

// velocitySample records token usage at a point in time.
type velocitySample struct {
	tokens int
	time   time.Time
}

// Compactor manages context window budgets and proactive compaction.
// It tracks token velocity to predict when compaction will be needed
// and triggers it before hitting the wall.
type Compactor struct {
	counter    *Counter
	maxTokens  int
	reserveOut int // tokens reserved for output

	// Proactive compaction thresholds
	compactThreshold float64 // trigger compaction at this fill ratio (default: 0.90)
	warnThreshold    float64 // warn the user at this fill ratio (default: 0.80)

	// Velocity tracking for prediction
	mu       sync.Mutex
	samples  []velocitySample
	maxSamples int
}

// NewCompactor creates a compactor for a given context window.
func NewCompactor(maxContextTokens, reserveOutput int) *Compactor {
	return &Compactor{
		counter:          NewCounter(),
		maxTokens:        maxContextTokens,
		reserveOut:       reserveOutput,
		compactThreshold: 0.90,
		warnThreshold:    0.80,
		maxSamples:       20,
	}
}

// Budget returns the available token budget for input.
func (c *Compactor) Budget() int {
	return c.maxTokens - c.reserveOut
}

// WindowSize returns the total context window size.
func (c *Compactor) WindowSize() int {
	return c.maxTokens
}

// NeedsCompaction returns true if the messages exceed the budget.
func (c *Compactor) NeedsCompaction(messages []MessageTokens) bool {
	total := c.counter.CountMessages(messages)
	return total > c.Budget()
}

// CheckFill returns detailed context window fill information.
// This is the core of predictive token management — it tells the agent
// and TUI exactly how close we are to needing compaction.
func (c *Compactor) CheckFill(messages []MessageTokens) WindowFill {
	total := c.counter.CountMessages(messages)
	budget := c.Budget()
	fillPercent := 0.0
	if budget > 0 {
		fillPercent = float64(total) / float64(budget)
	}

	// Record sample for velocity tracking
	c.recordSample(total)

	return WindowFill{
		UsedTokens:      total,
		BudgetTokens:    budget,
		FillPercent:     fillPercent,
		RemainingTokens: budget - total,
		NeedsCompaction: total > budget,
		ShouldCompact:   fillPercent >= c.compactThreshold,
		MessageCount:    len(messages),
	}
}

// recordSample adds a velocity tracking data point.
func (c *Compactor) recordSample(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples = append(c.samples, velocitySample{tokens: tokens, time: time.Now()})
	if len(c.samples) > c.maxSamples {
		c.samples = c.samples[len(c.samples)-c.maxSamples:]
	}
}

// EstimateIterationsUntilFull estimates how many more agent iterations
// can run before the context window fills up, based on observed velocity.
// Returns -1 if not enough data to estimate.
func (c *Compactor) EstimateIterationsUntilFull() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.samples) < 2 {
		return -1
	}

	// Calculate average tokens per iteration (sample)
	first := c.samples[0]
	last := c.samples[len(c.samples)-1]
	totalGrowth := last.tokens - first.tokens
	iterations := len(c.samples) - 1

	if iterations == 0 || totalGrowth <= 0 {
		return -1
	}

	avgPerIteration := totalGrowth / iterations
	remaining := c.Budget() - last.tokens
	if remaining <= 0 || avgPerIteration == 0 {
		return 0
	}

	return remaining / avgPerIteration
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

// CompactProactive performs compaction when the fill percentage exceeds
// the proactive threshold (default 90%), even if not yet at 100%.
// Returns the compacted messages and true if compaction was performed.
func (c *Compactor) CompactProactive(messages []MessageTokens) ([]MessageTokens, bool) {
	fill := c.CheckFill(messages)
	if !fill.ShouldCompact {
		return messages, false
	}

	// Target: compact down to 70% of budget to leave headroom
	targetBudget := int(float64(c.Budget()) * 0.70)
	saved := c.compactToBudget(messages, targetBudget)
	return saved, true
}

// compactToBudget reduces messages to fit within the given target budget.
func (c *Compactor) compactToBudget(messages []MessageTokens, targetBudget int) []MessageTokens {
	if len(messages) <= 3 {
		return messages
	}

	headerTokens := 0
	headerEnd := 0
	for i := 0; i < len(messages) && i < 2; i++ {
		headerTokens += messages[i].Tokens + 4
		headerEnd = i + 1
	}

	remaining := targetBudget - headerTokens - 50
	var tail []MessageTokens
	for i := len(messages) - 1; i >= headerEnd; i-- {
		msgCost := messages[i].Tokens + 4
		if remaining-msgCost < 0 {
			break
		}
		remaining -= msgCost
		tail = append([]MessageTokens{messages[i]}, tail...)
	}

	result := make([]MessageTokens, 0, len(tail)+headerEnd+1)
	result = append(result, messages[:headerEnd]...)

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
	windows := map[string]int{
		// Claude 4.x / 3.5x
		"claude-sonnet-4-20250514":   200000,
		"claude-opus-4-20250514":     200000,
		"claude-3-5-sonnet-20241022": 200000,
		"claude-3-5-haiku-20241022":  200000,
		"claude-3-opus-20240229":     200000,
		// OpenAI
		"gpt-4o":       128000,
		"gpt-4o-mini":  128000,
		"gpt-4-turbo":  128000,
		"gpt-4.1":      1000000,
		"gpt-4.1-mini": 1000000,
		"o1":           200000,
		"o1-mini":      128000,
		"o3":           200000,
		"o3-mini":      200000,
		"o4-mini":      200000,
		// Gemini
		"gemini-2.5-pro":   1000000,
		"gemini-2.5-flash": 1000000,
		"gemini-2.0-flash": 1000000,
	}

	if w, ok := windows[model]; ok {
		return w
	}

	// Check prefix matches for versioned model IDs
	for prefix, w := range windows {
		if strings.HasPrefix(model, prefix) {
			return w
		}
	}

	// Default to 128K for unknown models
	return 128000
}
