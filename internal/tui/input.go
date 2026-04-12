package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SlashCommand represents a parsed slash command.
type SlashCommand struct {
	Name string
	Args string
}

// InputModel wraps a textinput with command history and slash command support.
type InputModel struct {
	textInput textinput.Model
	history   []string
	histIdx   int
	theme     Theme
}

// NewInputModel creates a new input model.
func NewInputModel(theme Theme) InputModel {
	ti := textinput.New()
	ti.Placeholder = "Type a prompt, or /help for commands..."
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 80
	ti.PromptStyle = lipgloss.NewStyle().Foreground(theme.Primary).Bold(true)
	ti.Prompt = "> "

	return InputModel{
		textInput: ti,
		history:   make([]string, 0),
		histIdx:   -1,
		theme:     theme,
	}
}

// Init returns the initial command (cursor blink).
func (m InputModel) Init() tea.Cmd {
	return textinput.Blink
}

// SubmitMsg is sent when the user presses Enter with non-empty input.
type SubmitMsg struct {
	Text string
}

// SlashCommandMsg is sent when a slash command is entered.
type SlashCommandMsg struct {
	Command SlashCommand
}

// ClearScreenMsg is sent when the user requests screen clear.
type ClearScreenMsg struct{}

// NewSessionMsg is sent when the user requests a new session.
type NewSessionMsg struct{}

// Update handles key events for the input.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := strings.TrimSpace(m.textInput.Value())
			if text == "" {
				return m, nil
			}

			// Add to history
			m.history = append(m.history, text)
			m.histIdx = len(m.history)
			m.textInput.SetValue("")

			// Check for slash command
			if strings.HasPrefix(text, "/") {
				cmd := parseSlashCommand(text)
				return m, func() tea.Msg {
					return SlashCommandMsg{Command: cmd}
				}
			}

			return m, func() tea.Msg {
				return SubmitMsg{Text: text}
			}

		case "up":
			if len(m.history) > 0 && m.histIdx > 0 {
				m.histIdx--
				m.textInput.SetValue(m.history[m.histIdx])
				m.textInput.CursorEnd()
			}
			return m, nil

		case "down":
			if m.histIdx < len(m.history)-1 {
				m.histIdx++
				m.textInput.SetValue(m.history[m.histIdx])
				m.textInput.CursorEnd()
			} else {
				m.histIdx = len(m.history)
				m.textInput.SetValue("")
			}
			return m, nil

		case "ctrl+k":
			m.textInput.SetValue("")
			return m, nil

		case "ctrl+l":
			m.textInput.SetValue("")
			return m, func() tea.Msg { return ClearScreenMsg{} }

		case "ctrl+n":
			return m, func() tea.Msg { return NewSessionMsg{} }

		case "ctrl+c":
			if m.textInput.Value() != "" {
				m.textInput.SetValue("")
				return m, nil
			}
			return m, tea.Quit

		case "tab":
			return m.handleTabComplete(), nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the input field.
func (m InputModel) View() string {
	return m.textInput.View()
}

// Focus sets focus to the text input.
func (m *InputModel) Focus() tea.Cmd {
	return m.textInput.Focus()
}

// Blur removes focus from the text input.
func (m *InputModel) Blur() {
	m.textInput.Blur()
}

// SetWidth updates the text input width.
func (m *InputModel) SetWidth(w int) {
	m.textInput.Width = w - 4 // account for prompt and padding
}

// Focused returns whether the input is focused.
func (m InputModel) Focused() bool {
	return m.textInput.Focused()
}

// slashCommands lists available slash commands for autocomplete.
var slashCommands = []string{
	"/help",
	"/mode",
	"/model",
	"/quit",
	"/clear",
	"/cost",
	"/providers",
	"/setup",
}

func (m InputModel) handleTabComplete() InputModel {
	val := m.textInput.Value()
	if !strings.HasPrefix(val, "/") {
		return m
	}

	var matches []string
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, val) {
			matches = append(matches, cmd)
		}
	}

	if len(matches) == 1 {
		m.textInput.SetValue(matches[0] + " ")
		m.textInput.CursorEnd()
	}

	return m
}

func parseSlashCommand(text string) SlashCommand {
	parts := strings.SplitN(text, " ", 2)
	cmd := SlashCommand{Name: parts[0]}
	if len(parts) > 1 {
		cmd.Args = strings.TrimSpace(parts[1])
	}
	return cmd
}
