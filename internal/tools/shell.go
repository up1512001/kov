package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ShellExec executes a shell command in the project directory.
// This is the most dangerous tool — always requires permission.
type ShellExec struct {
	projectDir string
}

func NewShellExec(projectDir string) *ShellExec {
	return &ShellExec{projectDir: projectDir}
}

func (t *ShellExec) Name() string        { return "shell_exec" }
func (t *ShellExec) NeedsPermission() bool { return true }
func (t *ShellExec) Category() ToolCategory { return CategoryExecute }
func (t *ShellExec) Description() string {
	return "Execute a shell command in the project directory. Use for running tests, linters, build tools, git commands, etc. Commands have a 120 second timeout."
}
func (t *ShellExec) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to execute (e.g., 'npm test', 'go build ./...')",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default: 120)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellExec) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	if input.Command == "" {
		return "", fmt.Errorf("command cannot be empty")
	}

	timeout := 120 * time.Second
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Second
	}
	if timeout > 300*time.Second {
		timeout = 300 * time.Second // Hard cap at 5 minutes
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
	cmd.Dir = t.projectDir
	cmd.Env = append(cmd.Environ(),
		"PAGER=cat",           // Don't page output
		"GIT_TERMINAL_PROMPT=0", // Don't prompt for git
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		// Truncate very long output
		out := stdout.String()
		if len(out) > 50000 {
			out = out[:25000] + "\n\n... (output truncated, showing first and last 25KB) ...\n\n" + out[len(out)-25000:]
		}
		result.WriteString(out)
	}
	if stderr.Len() > 0 {
		stderrStr := stderr.String()
		if len(stderrStr) > 10000 {
			stderrStr = stderrStr[:10000] + "\n... (stderr truncated)"
		}
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n")
		result.WriteString(stderrStr)
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result.String(), fmt.Errorf("command timed out after %v", timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return result.String(), fmt.Errorf("exit code %d", exitErr.ExitCode())
		}
		return result.String(), err
	}

	return result.String(), nil
}
