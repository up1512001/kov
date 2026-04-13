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

// CancelMsg signals the current agent run should be cancelled.
type CancelMsg struct{}

// WindowFillMsg updates the context window fill display.
type WindowFillMsg struct {
	FillPercent     float64
	UsedTokens      int
	BudgetTokens    int
	RemainingTokens int
}

// RateLimitMsg signals a rate limit was hit.
type RateLimitMsg struct {
	Provider   string
	RetryAfter int // seconds until retry
	Action     string // "waiting", "failover", "paused"
}

// ChatMessage represents a rendered conversation turn.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// --- Model ---

// Model is the Bubble Tea model for kov's TUI.
// It supports both single-shot mode (original) and interactive REPL mode.
type Model struct {
	renderer *Renderer
	width    int
	height   int
	mode     string
	model    string
	state    string // "input", "streaming", "tool", "done", "error", "idle"
	cost     float64
	iteration int
	version  string

	// Interactive mode
	interactive bool
	input       InputModel
	messages    []ChatMessage // conversation history

	// Current streaming content
	content  strings.Builder
	thinking strings.Builder
	toolCalls []string
	statusMsg string

	// Permission
	pendingPerm *PermissionMsg

	// Context window fill tracking
	windowFill  float64 // 0.0 to 1.0
	windowUsed  int     // tokens used
	windowTotal int     // total budget

	// Rate limit info
	rateLimitMsg string

	// Done
	done   bool
	errMsg string
}

// NewModel creates a new TUI model for single-shot mode.
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

// NewInteractiveModel creates a new TUI model for interactive REPL mode.
func NewInteractiveModel(mode, model, version string) Model {
	theme := DefaultTheme()
	return Model{
		renderer:    NewRenderer(100),
		width:       100,
		height:      40,
		mode:        mode,
		model:       model,
		version:     version,
		state:       "input",
		interactive: true,
		input:       NewInputModel(theme),
		messages:    make([]ChatMessage, 0),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.interactive {
		return m.input.Init()
	}
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderer = NewRenderer(msg.Width)
		if m.interactive {
			m.input.SetWidth(msg.Width)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case SubmitMsg:
		// User submitted a prompt from the input
		m.messages = append(m.messages, ChatMessage{Role: "user", Content: msg.Text})
		m.state = "streaming"
		m.content.Reset()
		m.thinking.Reset()
		m.toolCalls = nil
		m.input.Blur()
		return m, nil

	case SlashCommandMsg:
		return m.handleSlashCommand(msg.Command)

	case ClearScreenMsg:
		m.messages = nil
		m.content.Reset()
		m.thinking.Reset()
		m.toolCalls = nil
		return m, nil

	case TokenMsg:
		m.content.WriteString(msg.Token)
		m.state = "streaming"
		return m, nil

	case ThinkingMsg:
		m.thinking.WriteString(msg.Token)
		return m, nil

	case ToolCallMsg:
		m.toolCalls = append(m.toolCalls, m.renderer.RenderToolCall(msg.Name))
		m.state = "tool"
		return m, nil

	case ToolResultMsg:
		icon := "✅"
		if !msg.Success {
			icon = "❌"
		}
		m.toolCalls = append(m.toolCalls, fmt.Sprintf("  %s %s", icon, msg.Name))
		return m, nil

	case StatusMsg:
		m.statusMsg = msg.Status
		return m, nil

	case CostMsg:
		m.cost = msg.Total
		return m, nil

	case DoneMsg:
		m.cost = msg.Cost
		if m.interactive {
			// Save assistant response to conversation and return to input
			m.finishTurn()
			return m, m.input.Focus()
		}
		m.done = true
		m.state = "done"
		return m, tea.Quit

	case ErrorMsg:
		m.errMsg = msg.Error
		if m.interactive {
			m.finishTurn()
			m.messages = append(m.messages, ChatMessage{
				Role:    "assistant",
				Content: "Error: " + msg.Error,
			})
			return m, m.input.Focus()
		}
		m.done = true
		m.state = "error"
		return m, tea.Quit

	case PermissionMsg:
		m.pendingPerm = &msg
		return m, nil

	case WindowFillMsg:
		m.windowFill = msg.FillPercent
		m.windowUsed = msg.UsedTokens
		m.windowTotal = msg.BudgetTokens
		return m, nil

	case RateLimitMsg:
		m.rateLimitMsg = fmt.Sprintf("Rate limited on %s — %s (retry in %ds)", msg.Provider, msg.Action, msg.RetryAfter)
		return m, nil
	}

	return m, nil
}

// handleKey processes keyboard input based on current state.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Permission prompt state — handle y/n/a
	if m.pendingPerm != nil {
		switch key {
		case "y", "Y":
			m.pendingPerm.ResponseCh <- true
			m.pendingPerm = nil
		case "n", "N":
			m.pendingPerm.ResponseCh <- false
			m.pendingPerm = nil
		case "a":
			// Allow all for this session
			m.pendingPerm.ResponseCh <- true
			m.pendingPerm = nil
			m.statusMsg = "Auto-approving all permissions for this session"
		case "ctrl+c":
			if m.pendingPerm != nil {
				m.pendingPerm.ResponseCh <- false
				m.pendingPerm = nil
			}
			return m, tea.Quit
		}
		return m, nil
	}

	// Streaming state — only esc/ctrl+c to cancel
	if m.state == "streaming" || m.state == "tool" {
		switch key {
		case "esc", "ctrl+c":
			return m, func() tea.Msg { return CancelMsg{} }
		}
		return m, nil
	}

	// Non-interactive mode
	if !m.interactive {
		switch key {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
		return m, nil
	}

	// Interactive input state — delegate to input model
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleSlashCommand processes a slash command.
func (m Model) handleSlashCommand(cmd SlashCommand) (tea.Model, tea.Cmd) {
	switch cmd.Name {
	case "/quit":
		return m, tea.Quit

	case "/clear":
		m.messages = nil
		m.content.Reset()
		m.thinking.Reset()
		m.toolCalls = nil
		return m, nil

	case "/help":
		help := `Available commands:
  /help              Show this help
  /mode <mode>       Switch mode (code, plan, think, ask, fast, research, architect, review)
  /model <model>     Switch model
  /providers         List configured providers
  /cost              Show session cost
  /clear             Clear conversation
  /quit              Exit kov

Keyboard shortcuts:
  Enter              Submit prompt
  Ctrl+K             Clear input line
  Ctrl+L             Clear screen
  Ctrl+C             Quit (empty input) or clear input
  Ctrl+N             New session
  Up/Down            Browse command history
  Tab                Autocomplete slash commands
  Esc                Cancel current generation`
		m.messages = append(m.messages, ChatMessage{Role: "assistant", Content: help})
		return m, nil

	case "/mode":
		if cmd.Args != "" {
			m.mode = cmd.Args
			m.messages = append(m.messages, ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("Switched to %s mode.", m.mode),
			})
		} else {
			m.messages = append(m.messages, ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("Current mode: %s. Use /mode <mode> to switch.", m.mode),
			})
		}
		return m, nil

	case "/model":
		if cmd.Args != "" {
			m.model = cmd.Args
			m.messages = append(m.messages, ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("Switched to model: %s", m.model),
			})
		} else {
			m.messages = append(m.messages, ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("Current model: %s. Use /model <model> to switch.", m.model),
			})
		}
		return m, nil

	case "/cost":
		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: fmt.Sprintf("Session cost: $%.4f", m.cost),
		})
		return m, nil

	case "/providers":
		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: "Use `kov config` to see configured providers and detected CLI tools.",
		})
		return m, nil

	default:
		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: fmt.Sprintf("Unknown command: %s. Type /help for available commands.", cmd.Name),
		})
		return m, nil
	}
}

// finishTurn saves the current streaming content as a conversation turn and resets.
func (m *Model) finishTurn() {
	if m.content.Len() > 0 {
		// Build the full response including tool calls
		var response strings.Builder
		for _, tc := range m.toolCalls {
			response.WriteString(tc)
			response.WriteString("\n")
		}
		if len(m.toolCalls) > 0 {
			response.WriteString("\n")
		}
		response.WriteString(m.content.String())

		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: response.String(),
		})
	}

	m.content.Reset()
	m.thinking.Reset()
	m.toolCalls = nil
	m.state = "input"
	m.errMsg = ""
}

// View implements tea.Model.
func (m Model) View() string {
	if m.interactive {
		return m.viewInteractive()
	}
	return m.viewSingleShot()
}

// viewInteractive renders the multi-turn REPL interface.
func (m Model) viewInteractive() string {
	var sb strings.Builder

	// Header
	sb.WriteString(m.renderer.RenderHeader(m.version, m.mode, m.model))
	sb.WriteString("\n")
	sb.WriteString(m.renderer.RenderDivider())
	sb.WriteString("\n\n")

	// Conversation history
	for _, msg := range m.messages {
		if msg.Role == "user" {
			prompt := lipgloss.NewStyle().
				Foreground(m.renderer.theme.Primary).
				Bold(true).
				Render("> " + msg.Content)
			sb.WriteString(prompt)
			sb.WriteString("\n\n")
		} else {
			rendered := m.renderer.RenderMarkdown(msg.Content)
			sb.WriteString(rendered)
			sb.WriteString("\n\n")
		}
	}

	// Current streaming content
	if m.state == "streaming" || m.state == "tool" {
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

		// Streaming content
		if m.content.Len() > 0 {
			rendered := m.renderer.RenderMarkdown(m.content.String())
			sb.WriteString(rendered)
			sb.WriteString("\n")
		}

		// Cancel hint
		cancelHint := lipgloss.NewStyle().Foreground(m.renderer.theme.Muted).
			Render("  (press Esc to cancel)")
		sb.WriteString(cancelHint)
		sb.WriteString("\n\n")
	}

	// Permission prompt
	if m.pendingPerm != nil {
		sb.WriteString("\n")
		sb.WriteString(m.renderer.RenderPermission(m.pendingPerm.Tool, m.pendingPerm.Description))
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Foreground(m.renderer.theme.Muted).Render("(y)es (n)o (a)ll"))
		sb.WriteString("\n\n")
	}

	// Error
	if m.errMsg != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true).Render("Error: " + m.errMsg))
		sb.WriteString("\n\n")
	}

	// Input field (only when in input state)
	if m.state == "input" {
		sb.WriteString(m.input.View())
		sb.WriteString("\n")
	}

	// Status bar
	sb.WriteString("\n")
	sb.WriteString(m.renderer.RenderDivider())
	sb.WriteString("\n")

	// Build status bar with context window fill and shortcuts
	statusParts := []string{
		m.renderer.theme.ModeStyle.Render(m.mode),
		lipgloss.NewStyle().Foreground(m.renderer.theme.Muted).Render(m.model),
		m.renderer.RenderCost(m.cost),
	}

	// Context window fill indicator
	if m.windowTotal > 0 {
		fillPct := m.windowFill * 100
		fillColor := m.renderer.theme.Success
		if fillPct >= 90 {
			fillColor = lipgloss.Color("#F87171") // red
		} else if fillPct >= 70 {
			fillColor = lipgloss.Color("#FBBF24") // yellow
		}
		windowStr := fmt.Sprintf("ctx: %.0f%%", fillPct)
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(fillColor).Render(windowStr))
	}

	// Rate limit warning
	if m.rateLimitMsg != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Render(m.rateLimitMsg))
		sb.WriteString("\n")
	}

	shortcuts := lipgloss.NewStyle().Foreground(m.renderer.theme.Muted).
		Render("ctrl+c quit | /help commands | esc cancel")

	sb.WriteString(m.renderer.theme.StatusBar.Render(
		strings.Join(statusParts, "  |  ") + "    " + shortcuts,
	))

	return sb.String()
}

// viewSingleShot renders the original single-shot mode.
func (m Model) viewSingleShot() string {
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
