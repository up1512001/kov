// Package tui implements the terminal user interface using Bubble Tea.
// It provides real-time streaming display, markdown rendering, tool call
// status, cost tracking, and interactive input.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the visual theme for the TUI.
type Theme struct {
	// Colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Muted     lipgloss.Color
	Error     lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	BG        lipgloss.Color

	// Styles
	Header      lipgloss.Style
	StatusBar   lipgloss.Style
	Prompt      lipgloss.Style
	ToolCall    lipgloss.Style
	ToolResult  lipgloss.Style
	Thinking    lipgloss.Style
	Cost        lipgloss.Style
	ErrorStyle  lipgloss.Style
	ModeStyle   lipgloss.Style
	Divider     lipgloss.Style
}

// DefaultTheme returns the default (dark) theme.
func DefaultTheme() Theme {
	t := Theme{
		Primary:   lipgloss.Color("#A78BFA"),  // violet-400
		Secondary: lipgloss.Color("#60A5FA"),  // blue-400
		Accent:    lipgloss.Color("#34D399"),   // emerald-400
		Muted:     lipgloss.Color("#6B7280"),   // gray-500
		Error:     lipgloss.Color("#F87171"),   // red-400
		Success:   lipgloss.Color("#34D399"),   // emerald-400
		Warning:   lipgloss.Color("#FBBF24"),   // amber-400
		BG:        lipgloss.Color("#1F2937"),   // gray-800
	}

	t.Header = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		MarginBottom(1)

	t.StatusBar = lipgloss.NewStyle().
		Foreground(t.Muted).
		PaddingLeft(1).
		PaddingRight(1)

	t.Prompt = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	t.ToolCall = lipgloss.NewStyle().
		Foreground(t.Secondary).
		PaddingLeft(2)

	t.ToolResult = lipgloss.NewStyle().
		Foreground(t.Accent).
		PaddingLeft(4)

	t.Thinking = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true)

	t.Cost = lipgloss.NewStyle().
		Foreground(t.Warning)

	t.ErrorStyle = lipgloss.NewStyle().
		Foreground(t.Error).
		Bold(true)

	t.ModeStyle = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)

	t.Divider = lipgloss.NewStyle().
		Foreground(t.Muted)

	return t
}

// Renderer handles markdown rendering and text formatting.
type Renderer struct {
	theme    Theme
	markdown *glamour.TermRenderer
	width    int
}

// NewRenderer creates a new renderer.
func NewRenderer(width int) *Renderer {
	md, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)

	return &Renderer{
		theme:    DefaultTheme(),
		markdown: md,
		width:    width,
	}
}

// RenderMarkdown converts markdown to styled terminal output.
func (r *Renderer) RenderMarkdown(content string) string {
	if r.markdown == nil {
		return content
	}
	out, err := r.markdown.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(out)
}

// RenderHeader renders the kov startup banner.
func (r *Renderer) RenderHeader(version, mode, model string) string {
	banner := r.theme.Header.Render("⚡ kov")
	info := fmt.Sprintf(" %s  |  %s  |  %s",
		r.theme.ModeStyle.Render(mode),
		lipgloss.NewStyle().Foreground(r.theme.Muted).Render(model),
		lipgloss.NewStyle().Foreground(r.theme.Muted).Render(version),
	)
	return banner + info
}

// RenderToolCall renders a tool call start.
func (r *Renderer) RenderToolCall(name string) string {
	icon := "⚙️"
	return r.theme.ToolCall.Render(fmt.Sprintf("%s %s", icon, name))
}

// RenderToolResult renders a tool call result.
func (r *Renderer) RenderToolResult(name string, duration time.Duration, err error) string {
	if err != nil {
		return r.theme.ErrorStyle.Render(fmt.Sprintf("  ❌ %s: %s", name, err))
	}
	return r.theme.ToolResult.Render(fmt.Sprintf("  ✅ %s (%dms)", name, duration.Milliseconds()))
}

// RenderCost renders the cost display.
func (r *Renderer) RenderCost(cost float64) string {
	return r.theme.Cost.Render(fmt.Sprintf("$%.4f", cost))
}

// RenderStatus renders the status bar.
func (r *Renderer) RenderStatus(mode, state, model string, cost float64, iteration int) string {
	parts := []string{
		r.theme.ModeStyle.Render(mode),
		lipgloss.NewStyle().Foreground(r.theme.Secondary).Render(state),
		lipgloss.NewStyle().Foreground(r.theme.Muted).Render(model),
		r.RenderCost(cost),
	}
	if iteration > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(r.theme.Muted).Render(fmt.Sprintf("iter:%d", iteration)))
	}
	return r.theme.StatusBar.Render(strings.Join(parts, "  │  "))
}

// RenderDivider renders a horizontal divider.
func (r *Renderer) RenderDivider() string {
	return r.theme.Divider.Render(strings.Repeat("─", r.width))
}

// RenderThinking renders thinking tokens.
func (r *Renderer) RenderThinking(content string) string {
	if content == "" {
		return ""
	}
	return r.theme.Thinking.Render("💭 " + content)
}

// RenderPermission renders a permission prompt.
func (r *Renderer) RenderPermission(tool, description string) string {
	return fmt.Sprintf("%s Allow %s?\n%s\n[y/N] ",
		lipgloss.NewStyle().Foreground(r.theme.Warning).Render("🔐"),
		r.theme.ToolCall.Render(tool),
		lipgloss.NewStyle().Foreground(r.theme.Muted).Render(description),
	)
}
