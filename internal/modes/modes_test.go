package modes

import (
	"testing"

	"github.com/utsavkovy/kov/internal/config"
)

func TestResolve_DefaultCode(t *testing.T) {
	cfg := config.Defaults()
	r := Resolve("code", "claude-sonnet-4-20250514", cfg)

	if r.Mode != "code" {
		t.Errorf("expected mode 'code', got %s", r.Mode)
	}
	if r.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected default model, got %s", r.Model)
	}
	if r.ReadOnly {
		t.Error("code mode should not be read-only")
	}
}

func TestResolve_PlanReadOnly(t *testing.T) {
	cfg := config.Defaults()
	r := Resolve("plan", "claude-sonnet-4-20250514", cfg)

	if !r.ReadOnly {
		t.Error("plan mode should be read-only")
	}
	if len(r.ToolFilter) != 4 {
		t.Errorf("expected 4 read tools, got %d", len(r.ToolFilter))
	}
}

func TestResolve_ThinkBudget(t *testing.T) {
	cfg := config.Defaults()
	r := Resolve("think", "claude-sonnet-4-20250514", cfg)

	if r.ThinkBudget != 20000 {
		t.Errorf("expected 20000 think budget, got %d", r.ThinkBudget)
	}
}

func TestResolve_FastNoThinking(t *testing.T) {
	cfg := config.Defaults()
	r := Resolve("fast", "claude-sonnet-4-20250514", cfg)

	if r.ThinkBudget != 0 {
		t.Errorf("expected 0 think budget for fast, got %d", r.ThinkBudget)
	}
}

func TestResolve_ModelOverride(t *testing.T) {
	cfg := config.Defaults()
	cfg.Modes.Plan = &config.ModeConfig{Model: "gemini-2.5-pro"}
	r := Resolve("plan", "claude-sonnet-4-20250514", cfg)

	if r.Model != "gemini-2.5-pro" {
		t.Errorf("expected override model 'gemini-2.5-pro', got %s", r.Model)
	}
}

func TestResolve_ReviewMode(t *testing.T) {
	cfg := config.Defaults()
	r := Resolve("review", "claude-sonnet-4-20250514", cfg)

	if !r.ReadOnly {
		t.Error("review should be read-only")
	}
	// review gets shell_exec too (for git diff)
	if len(r.ToolFilter) != 5 {
		t.Errorf("expected 5 tools for review, got %d", len(r.ToolFilter))
	}
}

func TestArchitectModels_Default(t *testing.T) {
	cfg := config.Defaults()
	plan, edit := ArchitectModels("claude-sonnet-4-20250514", cfg)

	if plan != "claude-sonnet-4-20250514" {
		t.Errorf("expected default plan model, got %s", plan)
	}
	if edit != "claude-sonnet-4-20250514" {
		t.Errorf("expected default edit model, got %s", edit)
	}
}

func TestArchitectModels_Override(t *testing.T) {
	cfg := config.Defaults()
	cfg.Modes.Architect = &config.ArchitectConfig{
		PlanModel: "claude-opus-4-20250514",
		EditModel: "claude-haiku",
	}

	plan, edit := ArchitectModels("claude-sonnet-4-20250514", cfg)
	if plan != "claude-opus-4-20250514" {
		t.Errorf("expected opus plan model, got %s", plan)
	}
	if edit != "claude-haiku" {
		t.Errorf("expected haiku edit model, got %s", edit)
	}
}

func TestParseBudget(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"low", 5000},
		{"medium", 10000},
		{"high", 20000},
		{"maximum", 50000},
		{"", 15000},
		{"invalid", 15000},
	}

	for _, tc := range tests {
		got := parseBudget(tc.input, 15000)
		if got != tc.expected {
			t.Errorf("parseBudget(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}
