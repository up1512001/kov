package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Messages ---

// TokenMsg delivers a streaming token from the LLM.
type TokenMsg struct{ Token string }

// ThinkingMsg delivers a thinking token.
type ThinkingMsg struct{ Token string }

// ToolCallMsg signals a tool call started.
type ToolCallMsg struct{ Name string }

// ToolResultMsg signals a tool call completed.
type ToolResultMsg struct {
	Name    string
	Success bool
	Error   string
}

// StatusMsg updates the status bar.
type StatusMsg struct{ Status string }

// CostMsg updates the cost display.
type CostMsg struct{ Total float64 }

// DoneMsg signals the agent is done.
type DoneMsg struct{ Cost float64 }

// ErrorMsg signals an error.
type ErrorMsg struct{ Error string }

// PermissionMsg requests user permission.
type PermissionMsg struct {
	Tool        string
	Description string
	ResponseCh  chan bool
}

// --- Model ---

// Model is the Bubble Tea model for kov's TUI.
type Model struct {
	renderer    *Renderer
	width       int
	height      int
	mode        string
	model       string
	state       string
	cost        float64
	iteration   int

	// Content
	content     strings.Builder
	thinking    strings.Builder
	toolCalls   []string
	statusMsg   string

	// Permission
	pendingPerm *PermissionMsg

	// Done
	done    bool
	errMsg  string
}

// NewModel creates a new TUI model.
func NewModel(mode, model string) Model {
	return Model{
		renderer: NewRenderer(100),
		width:    100,
		height:   40,
		mode:     mode,
		model:    model,
		state:    "idle",
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.pendingPerm != nil {
				m.pendingPerm.ResponseCh <- false
				m.pendingPerm = nil
			}
			return m, tea.Quit
		case "y", "Y":
			if m.pendingPerm != nil {
				m.pendingPerm.ResponseCh <- true
				m.pendingPerm = nil
			}
		case "n", "N":
			if m.pendingPerm != nil {
				m.pendingPerm.ResponseCh <- false
				m.pendingPerm = nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderer = NewRenderer(msg.Width)

	case TokenMsg:
		m.content.WriteString(msg.Token)
		m.state = "streaming"

	case ThinkingMsg:
		m.thinking.WriteString(msg.Token)

	case ToolCallMsg:
		m.toolCalls = append(m.toolCalls, m.renderer.RenderToolCall(msg.Name))
		m.state = "tool:" + msg.Name

	case ToolResultMsg:
		icon := "✅"
		if !msg.Success {
			icon = "❌"
		}
		m.toolCalls = append(m.toolCalls, fmt.Sprintf("  %s %s", icon, msg.Name))

	case StatusMsg:
		m.statusMsg = msg.Status

	case CostMsg:
		m.cost = msg.Total

	case DoneMsg:
		m.done = true
		m.cost = msg.Cost
		m.state = "done"
		return m, tea.Quit

	case ErrorMsg:
		m.errMsg = msg.Error
		m.done = true
		m.state = "error"
		return m, tea.Quit

	case PermissionMsg:
		m.pendingPerm = &msg
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	var sb strings.Builder

	// Header
	sb.WriteString(m.renderer.RenderHeader("", m.mode, m.model))
	sb.WriteString("\n")
	sb.WriteString(m.renderer.RenderDivider())
	sb.WriteString("\n\n")

	// Thinking
	if m.thinking.Len() > 0 {
		sb.WriteString(m.renderer.RenderThinking(truncate(m.thinking.String(), 200)))
		sb.WriteString("\n\n")
	}

	// Tool calls
	for _, tc := range m.toolCalls {
		sb.WriteString(tc)
		sb.WriteString("\n")
	}
	if len(m.toolCalls) > 0 {
		sb.WriteString("\n")
	}

	// Content (rendered as markdown)
	if m.content.Len() > 0 {
		rendered := m.renderer.RenderMarkdown(m.content.String())
		sb.WriteString(rendered)
		sb.WriteString("\n")
	}

	// Permission prompt
	if m.pendingPerm != nil {
		sb.WriteString("\n")
		sb.WriteString(m.renderer.RenderPermission(m.pendingPerm.Tool, m.pendingPerm.Description))
		sb.WriteString("\n")
	}

	// Error
	if m.errMsg != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true).Render("❌ " + m.errMsg))
		sb.WriteString("\n")
	}

	// Status bar at bottom
	sb.WriteString("\n")
	sb.WriteString(m.renderer.RenderDivider())
	sb.WriteString("\n")
	status := m.renderer.RenderStatus(m.mode, m.state, m.model, m.cost, m.iteration)
	sb.WriteString(status)

	return sb.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
