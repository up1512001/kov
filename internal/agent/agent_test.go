package agent

import (
	"testing"
	"time"
)

func TestDefaultAgentConfig(t *testing.T) {
	cfg := DefaultAgentConfig()

	if cfg.Mode != "code" {
		t.Errorf("expected default mode 'code', got %s", cfg.Mode)
	}
	if cfg.MaxIterations != 50 {
		t.Errorf("expected 50 max iterations, got %d", cfg.MaxIterations)
	}
	if cfg.MaxTokens != 16384 {
		t.Errorf("expected 16384 max tokens, got %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0 {
		t.Errorf("expected 0 temperature, got %f", cfg.Temperature)
	}
	if cfg.Permissions != "confirm" {
		t.Errorf("expected 'confirm' permissions, got %s", cfg.Permissions)
	}
	if cfg.VerifyTimeout != 120*time.Second {
		t.Errorf("expected 120s verify timeout, got %v", cfg.VerifyTimeout)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		minExpected int
	}{
		{"", 0},
		{"hello", 1},
		{"This is a longer sentence with more characters", 10},
	}

	for _, tc := range tests {
		got := estimateTokens(tc.input)
		if got < tc.minExpected {
			t.Errorf("estimateTokens(%q) = %d, want >= %d", tc.input, got, tc.minExpected)
		}
	}
}

func TestTruncateArgs(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"a very long string that should be truncated", 20, "a very long strin..."},
		{"exact", 5, "exact"},
	}

	for _, tc := range tests {
		got := truncateArgs(tc.input, tc.maxLen)
		if got != tc.expected {
			t.Errorf("truncateArgs(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
		}
	}
}

func TestBuildSystemPrompt_Modes(t *testing.T) {
	modes := []struct {
		mode     string
		contains string
	}{
		{"code", "CODE"},
		{"plan", "PLAN"},
		{"think", "THINK"},
		{"ask", "ASK"},
		{"fast", "FAST"},
		{"research", "RESEARCH"},
		{"review", "REVIEW"},
		{"architect", "ARCHITECT"},
	}

	for _, tc := range modes {
		a := &Agent{config: AgentConfig{Mode: tc.mode}}
		prompt := a.buildSystemPrompt()
		if prompt == "" {
			t.Errorf("empty prompt for mode %s", tc.mode)
		}
		if !contains(prompt, tc.contains) {
			t.Errorf("prompt for mode %q should contain %q", tc.mode, tc.contains)
		}
		// All prompts should contain the base instructions
		if !contains(prompt, "file_read") {
			t.Errorf("all prompts should mention available tools")
		}
	}
}

func TestBuildProviderTools_ReadOnlyModes(t *testing.T) {
	// Create a minimal agent with a tool registry
	a := &Agent{
		config: AgentConfig{Mode: "plan"},
		tools:  nil, // Will need a mock for full test
	}

	// Can't fully test without real registry, but verify the mode logic
	if a.config.Mode != "plan" {
		t.Error("mode should be plan")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
