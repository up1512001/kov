package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme.Primary == "" {
		t.Error("expected Primary color to be set")
	}
	if theme.Success == "" {
		t.Error("expected Success color to be set")
	}
}

func TestNewRenderer(t *testing.T) {
	r := NewRenderer(80)
	if r.width != 80 {
		t.Errorf("expected width 80, got %d", r.width)
	}
	if r.markdown == nil {
		t.Error("expected markdown renderer to be initialized")
	}
}

func TestRenderMarkdown(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderMarkdown("# Hello\n\nThis is **bold** text.")

	if output == "" {
		t.Error("expected non-empty markdown output")
	}
	// The output should contain styled text (ANSI codes)
	if !strings.Contains(output, "Hello") {
		t.Error("expected 'Hello' in output")
	}
}

func TestRenderToolCall(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderToolCall("file_read")

	if !strings.Contains(output, "file_read") {
		t.Error("expected 'file_read' in tool call output")
	}
}

func TestRenderToolResult_Success(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderToolResult("file_read", 42*time.Millisecond, nil)

	if !strings.Contains(output, "✅") {
		t.Error("expected success icon")
	}
	if !strings.Contains(output, "42ms") {
		t.Error("expected duration in output")
	}
}

func TestRenderToolResult_Error(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderToolResult("shell_exec", 0, fmt.Errorf("exit code 1"))

	if !strings.Contains(output, "❌") {
		t.Error("expected error icon")
	}
}

func TestRenderCost(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderCost(0.0042)

	if !strings.Contains(output, "$0.0042") {
		t.Errorf("expected '$0.0042', got %q", output)
	}
}

func TestRenderStatus(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderStatus("code", "executing", "claude-sonnet", 0.05, 3)

	if !strings.Contains(output, "code") {
		t.Error("expected mode in status")
	}
	if !strings.Contains(output, "iter:3") {
		t.Error("expected iteration in status")
	}
}

func TestRenderHeader(t *testing.T) {
	r := NewRenderer(80)
	output := r.RenderHeader("v0.1.0", "code", "claude-sonnet")

	if !strings.Contains(output, "kov") {
		t.Error("expected 'kov' in header")
	}
	if !strings.Contains(output, "code") {
		t.Error("expected mode in header")
	}
}

func TestRenderDivider(t *testing.T) {
	r := NewRenderer(40)
	output := r.RenderDivider()

	if !strings.Contains(output, "─") {
		t.Error("expected divider character")
	}
}

func TestModel_Init(t *testing.T) {
	m := NewModel("code", "claude-sonnet")
	if m.mode != "code" {
		t.Errorf("expected mode 'code', got %s", m.mode)
	}
	if m.model != "claude-sonnet" {
		t.Errorf("expected model 'claude-sonnet', got %s", m.model)
	}
}

func TestModel_View(t *testing.T) {
	m := NewModel("code", "claude-sonnet")
	output := m.View()

	if output == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(output, "kov") {
		t.Error("expected 'kov' in view")
	}
}

func TestModel_TokenUpdate(t *testing.T) {
	m := NewModel("code", "claude-sonnet")
	updated, _ := m.Update(TokenMsg{Token: "Hello "})
	m = updated.(Model)
	updated, _ = m.Update(TokenMsg{Token: "world"})
	m = updated.(Model)

	if m.content.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", m.content.String())
	}
	if m.state != "streaming" {
		t.Errorf("expected state 'streaming', got %s", m.state)
	}
}
