package tokens

import (
	"strings"
	"testing"
)

func TestCounter_EmptyString(t *testing.T) {
	c := NewCounter()
	if c.Count("") != 0 {
		t.Error("expected 0 for empty string")
	}
}

func TestCounter_SimpleProse(t *testing.T) {
	c := NewCounter()
	text := "Hello, world! This is a simple sentence."
	count := c.Count(text)

	// ~40 chars / 3.8 ≈ 11 tokens
	if count < 8 || count > 15 {
		t.Errorf("expected ~11 tokens, got %d", count)
	}
}

func TestCounter_CodeContent(t *testing.T) {
	c := NewCounter()
	code := `func main() {
		fmt.Println("hello")
		if err != nil {
			return err
		}
	}`
	count := c.Count(code)

	// Code should estimate more tokens due to symbols
	if count < 15 {
		t.Errorf("expected >15 tokens for code, got %d", count)
	}
}

func TestCounter_CountMessages(t *testing.T) {
	c := NewCounter()
	msgs := []MessageTokens{
		{Role: "system", Content: "You are helpful", Tokens: 10},
		{Role: "user", Content: "Hello", Tokens: 5},
		{Role: "assistant", Content: "Hi there!", Tokens: 8},
	}

	total := c.CountMessages(msgs)
	// 10 + 5 + 8 = 23 content + 12 overhead + 3 priming = 38
	if total != 38 {
		t.Errorf("expected 38, got %d", total)
	}
}

func TestCompactor_NeedsCompaction(t *testing.T) {
	comp := NewCompactor(100, 20) // budget = 80

	// Small conversation — no compaction
	small := []MessageTokens{
		{Role: "system", Tokens: 20},
		{Role: "user", Tokens: 10},
		{Role: "assistant", Tokens: 15},
	}
	if comp.NeedsCompaction(small) {
		t.Error("should not need compaction for small conversation")
	}

	// Large conversation — needs compaction
	large := []MessageTokens{
		{Role: "system", Tokens: 30},
		{Role: "user", Tokens: 20},
		{Role: "assistant", Tokens: 20},
		{Role: "user", Tokens: 20},
		{Role: "assistant", Tokens: 20},
	}
	if !comp.NeedsCompaction(large) {
		t.Error("should need compaction for large conversation")
	}
}

func TestCompactor_Compact(t *testing.T) {
	comp := NewCompactor(150, 20) // budget = 130

	msgs := []MessageTokens{
		{Role: "system", Content: "System prompt", Tokens: 15},
		{Role: "user", Content: "First question", Tokens: 10},
		{Role: "assistant", Content: "First answer", Tokens: 15},
		{Role: "user", Content: "Second question", Tokens: 15},
		{Role: "assistant", Content: "Second answer", Tokens: 15},
		{Role: "user", Content: "Third question", Tokens: 15},
		{Role: "assistant", Content: "Third answer", Tokens: 15},
		{Role: "user", Content: "Latest question", Tokens: 10},
		{Role: "assistant", Content: "Latest answer", Tokens: 10},
	}

	compacted := comp.Compact(msgs)

	// Should keep system + first user + compaction notice + recent
	if len(compacted) >= len(msgs) {
		t.Errorf("expected fewer messages after compaction, got %d vs %d", len(compacted), len(msgs))
	}

	// System message should always be preserved
	if compacted[0].Role != "system" {
		t.Error("first message should be system")
	}

	// Latest messages should be preserved
	last := compacted[len(compacted)-1]
	if last.Content != "Latest answer" {
		t.Errorf("expected last message 'Latest answer', got %q", last.Content)
	}
}

func TestCompactor_NoCompactionNeeded(t *testing.T) {
	comp := NewCompactor(10000, 1000) // huge budget

	msgs := []MessageTokens{
		{Role: "system", Tokens: 20},
		{Role: "user", Tokens: 10},
	}

	compacted := comp.Compact(msgs)
	if len(compacted) != len(msgs) {
		t.Error("should not compact when within budget")
	}
}

func TestCompactor_CompactionNotice(t *testing.T) {
	comp := NewCompactor(100, 20)

	msgs := []MessageTokens{
		{Role: "system", Content: "System", Tokens: 15},
		{Role: "user", Content: "Q1", Tokens: 10},
		{Role: "assistant", Content: "A1", Tokens: 20},
		{Role: "user", Content: "Q2", Tokens: 10},
		{Role: "assistant", Content: "A2", Tokens: 20},
		{Role: "user", Content: "Q3", Tokens: 10},
		{Role: "assistant", Content: "A3", Tokens: 20},
	}

	compacted := comp.Compact(msgs)

	// Find the compaction notice
	hasNotice := false
	for _, m := range compacted {
		if strings.Contains(m.Content, "compacted") {
			hasNotice = true
			break
		}
	}
	if !hasNotice {
		t.Error("expected compaction notice when messages are dropped")
	}
}

func TestEstimateModelWindow(t *testing.T) {
	tests := []struct {
		model    string
		expected int
	}{
		{"claude-sonnet-4-20250514", 200000},
		{"gpt-4o", 128000},
		{"gemini-2.5-pro", 1000000},
		{"unknown-model", 128000}, // default
	}

	for _, tc := range tests {
		got := EstimateModelWindow(tc.model)
		if got != tc.expected {
			t.Errorf("EstimateModelWindow(%q) = %d, want %d", tc.model, got, tc.expected)
		}
	}
}

func TestCompactor_Budget(t *testing.T) {
	comp := NewCompactor(200000, 16384)
	if comp.Budget() != 183616 {
		t.Errorf("expected budget 183616, got %d", comp.Budget())
	}
}
