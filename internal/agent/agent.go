// Package agent implements the core agent loop — the brain of kov.
// It orchestrates the plan→execute→verify cycle, managing tool calls,
// streaming responses, and resilience through the state machine.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/utsavkovy/kov/internal/bus"
	kovctx "github.com/utsavkovy/kov/internal/context"
	"github.com/utsavkovy/kov/internal/db"
	"github.com/utsavkovy/kov/internal/provider"
	"github.com/utsavkovy/kov/internal/resilience"
	"github.com/utsavkovy/kov/internal/tools"
)

// Agent runs the agentic coding loop for a session.
type Agent struct {
	db        *db.DB
	router    *provider.Router
	tools     *tools.Registry
	engine    *resilience.Engine
	bus       *bus.Bus
	logger    *slog.Logger
	config    AgentConfig
	sessionID string

	// Callbacks for permission requests and output display
	onPermission func(toolName, description string) bool
	onToken      func(token string)
	onThinking   func(token string)
	onToolCall   func(name string, args string)
	onToolResult func(name string, result string, err error)
	onStatus     func(status string)
}

// AgentConfig holds agent behavior settings.
type AgentConfig struct {
	Mode           string
	Model          string
	Provider       string
	MaxIterations  int
	MaxTokens      int
	Temperature    float64
	ThinkingBudget int
	Permissions    string // confirm, smart, yolo, chat
	ProjectDir     string // project root directory
	VerifyEnabled  bool
	VerifyCommand  string
	VerifyTimeout  time.Duration
	MaxFixRetries  int
}

// DefaultAgentConfig returns sensible defaults.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Mode:           "code",
		MaxIterations:  50,
		MaxTokens:      16384,
		Temperature:    0,
		ThinkingBudget: 10000,
		Permissions:    "confirm",
		VerifyEnabled:  true,
		VerifyTimeout:  120 * time.Second,
		MaxFixRetries:  2,
	}
}

// New creates a new agent.
func New(
	database *db.DB,
	router *provider.Router,
	toolRegistry *tools.Registry,
	engine *resilience.Engine,
	eventBus *bus.Bus,
	logger *slog.Logger,
	config AgentConfig,
) *Agent {
	return &Agent{
		db:     database,
		router: router,
		tools:  toolRegistry,
		engine: engine,
		bus:    eventBus,
		logger: logger,
		config: config,
	}
}

// SetCallbacks sets the UI callback functions.
func (a *Agent) SetCallbacks(
	onPermission func(string, string) bool,
	onToken func(string),
	onThinking func(string),
	onToolCall func(string, string),
	onToolResult func(string, string, error),
	onStatus func(string),
) {
	a.onPermission = onPermission
	a.onToken = onToken
	a.onThinking = onThinking
	a.onToolCall = onToolCall
	a.onToolResult = onToolResult
	a.onStatus = onStatus
}

// Run executes the agent loop for a given prompt.
func (a *Agent) Run(ctx context.Context, sessionID string, prompt string) error {
	a.sessionID = sessionID
	a.engine.BindSession(sessionID)

	// Emit session start
	a.bus.Publish(bus.SessionStarted{
		SessionID: sessionID,
		Mode:      a.config.Mode,
	})

	// Add user message to history
	a.db.AddMessage(ctx, &db.Message{
		SessionID:  sessionID,
		Role:       "user",
		Content:    prompt,
		TokenCount: estimateTokens(prompt),
	})

	// Transition to executing
	a.engine.Transition(ctx, resilience.StateExecuting)
	a.emit("Starting agent loop...")

	// Main agent loop
	for iteration := 0; iteration < a.config.MaxIterations; iteration++ {
		a.logger.Info("agent iteration",
			slog.Int("iteration", iteration+1),
			slog.Int("max", a.config.MaxIterations))

		// Build messages from DB
		messages, err := a.buildMessages(ctx)
		if err != nil {
			return fmt.Errorf("building messages: %w", err)
		}

		// Build provider tools list
		providerTools := a.buildProviderTools()

		// Stream LLM response
		response, err := a.streamLLMResponse(ctx, messages, providerTools)
		if err != nil {
			return fmt.Errorf("LLM request: %w", err)
		}

		// Save assistant message
		a.db.AddMessage(ctx, &db.Message{
			SessionID:  sessionID,
			Role:       "assistant",
			Content:    response.Content,
			TokenCount: response.OutputTokens,
			Cost:       a.calculateCost(response),
		})

		// Record cost
		cost := a.calculateCost(response)
		if err := a.engine.RecordCost(ctx, cost, "", response.Model, response.InputTokens, response.OutputTokens); err != nil {
			// Budget exceeded — gracefully stop
			a.emit("Budget exceeded. Session paused.")
			return err
		}

		// No tool calls → agent is done
		if len(response.ToolCalls) == 0 {
			a.logger.Info("agent completed — no more tool calls")
			break
		}

		// Execute tool calls
		for _, tc := range response.ToolCalls {
			toolResult, toolErr := a.executeTool(ctx, tc)

			// Record tool result as message
			resultContent := toolResult
			if toolErr != nil {
				resultContent = fmt.Sprintf("Error: %s\n\n%s", toolErr.Error(), toolResult)
			}

			a.db.AddMessage(ctx, &db.Message{
				SessionID:  sessionID,
				Role:       "tool",
				Content:    resultContent,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
				TokenCount: estimateTokens(resultContent),
			})

			// Loop detection
			if a.engine.RecordAction(tc.Name) {
				a.emit("⚠️ Loop detected — agent is repeating the same action. Pausing.")
				a.engine.Transition(ctx, resilience.StatePaused)
				return fmt.Errorf("loop detected: %s called %d times", tc.Name, a.config.MaxFixRetries)
			}
		}

		// Checkpoint after every tool execution round
		a.engine.Transition(ctx, resilience.StateExecuting)
	}

	// Verify if enabled
	if a.config.VerifyEnabled && a.config.VerifyCommand != "" {
		a.engine.Transition(ctx, resilience.StateVerifying)
		if err := a.verify(ctx); err != nil {
			a.logger.Warn("verification failed", slog.String("error", err.Error()))
			// Don't fail the whole session, just log it
		}
	}

	// Done
	a.engine.Transition(ctx, resilience.StateDone)

	// Emit session end
	total, done, _, _ := a.db.GetTaskStats(ctx, sessionID)
	sessionCost, _ := a.db.GetSessionCost(ctx, sessionID)
	a.bus.Publish(bus.SessionEnded{
		SessionID:  sessionID,
		TotalCost:  sessionCost,
		TasksTotal: total,
		TasksDone:  done,
	})

	return nil
}

// streamLLMResponse sends a streaming request and collects the response.
func (a *Agent) streamLLMResponse(ctx context.Context, messages []provider.Message, pTools []provider.Tool) (*provider.ChatResponse, error) {
	params := provider.ChatParams{
		Model:       a.config.Model,
		Messages:    messages,
		Tools:       pTools,
		MaxTokens:   a.config.MaxTokens,
		Temperature: a.config.Temperature,
	}

	// Enable thinking for appropriate modes
	if a.config.Mode == "think" || a.config.Mode == "architect" {
		params.ThinkingEnabled = true
		params.ThinkingBudget = a.config.ThinkingBudget
	}

	ch, err := a.router.Stream(ctx, params)
	if err != nil {
		return nil, err
	}

	// Collect streaming response
	resp := &provider.ChatResponse{}
	var contentBuilder strings.Builder
	var thinkingBuilder strings.Builder
	var currentToolCalls []provider.ToolCall

	for event := range ch {
		switch event.Type {
		case provider.EventContent:
			contentBuilder.WriteString(event.Content)
			if a.onToken != nil {
				a.onToken(event.Content)
			}
			a.bus.Publish(bus.TokenReceived{
				SessionID: a.sessionID,
				Token:     event.Content,
			})

		case provider.EventThinking:
			thinkingBuilder.WriteString(event.Thinking)
			if a.onThinking != nil {
				a.onThinking(event.Thinking)
			}

		case provider.EventToolCall:
			if event.ToolCall != nil {
				currentToolCalls = append(currentToolCalls, *event.ToolCall)
				if a.onToolCall != nil {
					a.onToolCall(event.ToolCall.Name, event.ToolCall.Arguments)
				}
			}

		case provider.EventDone:
			resp.InputTokens = event.InputTokens
			resp.OutputTokens = event.OutputTokens

		case provider.EventError:
			return nil, event.Error
		}
	}

	resp.Content = contentBuilder.String()
	resp.Thinking = thinkingBuilder.String()
	resp.ToolCalls = currentToolCalls

	return resp, nil
}

// executeTool runs a single tool call with permission checking.
func (a *Agent) executeTool(ctx context.Context, tc provider.ToolCall) (string, error) {
	tool, ok := a.tools.Get(tc.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}

	a.bus.Publish(bus.ToolCallStarted{
		SessionID: a.sessionID,
		ToolName:  tc.Name,
	})

	// Permission check
	if tool.NeedsPermission() && a.config.Permissions != "yolo" {
		if a.config.Permissions == "confirm" || a.config.Permissions == "smart" {
			if a.onPermission != nil {
				desc := fmt.Sprintf("%s(%s)", tc.Name, truncateArgs(tc.Arguments, 200))
				if !a.onPermission(tc.Name, desc) {
					return "Permission denied by user", nil
				}
			}
		}
	}

	start := time.Now()
	result, err := tool.Execute(ctx, json.RawMessage(tc.Arguments))
	duration := time.Since(start)

	if a.onToolResult != nil {
		a.onToolResult(tc.Name, truncateArgs(result, 500), err)
	}

	a.bus.Publish(bus.ToolCallCompleted{
		SessionID: a.sessionID,
		ToolName:  tc.Name,
		Duration:  duration.Milliseconds(),
	})

	a.logger.Info("tool executed",
		slog.String("tool", tc.Name),
		slog.Duration("duration", duration),
		slog.Bool("error", err != nil))

	return result, err
}

// verify runs the verification command (tests, lints, etc).
func (a *Agent) verify(ctx context.Context) error {
	if a.config.VerifyCommand == "" {
		return nil
	}

	a.emit(fmt.Sprintf("🔍 Verifying: %s", a.config.VerifyCommand))
	a.bus.Publish(bus.VerificationStarted{
		SessionID: a.sessionID,
		Command:   a.config.VerifyCommand,
	})

	args, _ := json.Marshal(map[string]interface{}{
		"command": a.config.VerifyCommand,
		"timeout": int(a.config.VerifyTimeout.Seconds()),
	})

	result, err := a.tools.Execute(ctx, "shell_exec", args)
	if err != nil {
		a.bus.Publish(bus.VerificationFailed{
			SessionID: a.sessionID,
			Error:     err,
			Output:    result,
		})
		return err
	}

	a.bus.Publish(bus.VerificationPassed{SessionID: a.sessionID})
	a.emit("✅ Verification passed")
	return nil
}

// buildMessages loads conversation history from DB and converts to provider format.
func (a *Agent) buildMessages(ctx context.Context) ([]provider.Message, error) {
	dbMessages, err := a.db.GetMessages(ctx, a.sessionID)
	if err != nil {
		return nil, err
	}

	// System prompt at the beginning
	messages := []provider.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
	}

	for _, m := range dbMessages {
		msg := provider.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.ToolName,
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// buildSystemPrompt creates the system prompt based on the mode.
func (a *Agent) buildSystemPrompt() string {
	base := `You are Kov, an expert AI coding assistant. You are working directly in the user's codebase.

Available tools: file_read, file_write, file_edit, shell_exec, grep_search, glob_search, list_dir

Rules:
- Always read files before editing them to understand the full context
- Make precise, minimal edits using file_edit when possible
- Use file_write only for new files or complete rewrites
- Run tests/lints after making changes when a verify command is available
- Explain what you're doing and why before making changes
- If something fails, analyze the error and try a different approach`

	// Add project context (KOV.md, repo map)
	if a.config.ProjectDir != "" {
		projCtx, err := kovctx.LoadProjectContext(a.config.ProjectDir)
		if err == nil {
			ctxStr := projCtx.ToSystemContext()
			if ctxStr != "" {
				base += "\n\n" + ctxStr
			}
		}
	}

	switch a.config.Mode {
	case "plan":
		return base + "\n\nMode: PLAN — Read-only analysis. Do NOT modify any files. Only use read tools (file_read, grep_search, glob_search, list_dir). Produce a detailed plan as text output."
	case "think":
		return base + "\n\nMode: THINK — Use extended thinking to deeply reason about the problem before acting. Show your reasoning process."
	case "ask":
		return base + "\n\nMode: ASK — Quick Q&A. Answer the user's question concisely. Use read tools to look up code if needed."
	case "fast":
		return base + "\n\nMode: FAST — Minimal overhead. Make direct edits without extensive analysis. Skip explanations."
	case "research":
		return base + "\n\nMode: RESEARCH — Multi-step investigation. Thoroughly explore the codebase to answer the question. Use grep_search and file_read extensively."
	case "review":
		return base + "\n\nMode: REVIEW — Review staged git changes. Run 'git diff --staged' first, then provide detailed feedback."
	case "architect":
		return base + "\n\nMode: ARCHITECT — Two-pass approach. First, deeply analyze and plan. Then implement the plan with precise edits."
	default: // "code"
		return base + "\n\nMode: CODE — Full agentic coding. Analyze, plan, implement, and verify."
	}
}

// buildProviderTools converts the tool registry to provider format.
func (a *Agent) buildProviderTools() []provider.Tool {
	regTools := a.tools.ForProvider()
	pTools := make([]provider.Tool, len(regTools))
	for i, t := range regTools {
		pTools[i] = provider.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	// In plan/ask mode, only expose read tools
	if a.config.Mode == "plan" || a.config.Mode == "ask" {
		filtered := make([]provider.Tool, 0)
		readTools := map[string]bool{"file_read": true, "grep_search": true, "glob_search": true, "list_dir": true}
		for _, t := range pTools {
			if readTools[t.Name] {
				filtered = append(filtered, t)
			}
		}
		return filtered
	}

	return pTools
}

// calculateCost computes cost using the known models table.
func (a *Agent) calculateCost(resp *provider.ChatResponse) float64 {
	model, ok := provider.GetModel(a.config.Model)
	if !ok {
		return 0
	}
	return model.CalculateCost(resp.InputTokens, resp.OutputTokens)
}

// emit sends a status update.
func (a *Agent) emit(status string) {
	if a.onStatus != nil {
		a.onStatus(status)
	}
}

// estimateTokens gives a rough token count (1 token ≈ 4 chars).
func estimateTokens(s string) int {
	return len(s) / 4
}

// truncateArgs shortens JSON args for display.
func truncateArgs(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
