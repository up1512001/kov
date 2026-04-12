// Package permissions implements the tool permission system for kov.
// It controls which tool calls require user approval based on risk level.
//
// Permission modes:
//   - confirm: Ask for all write/exec operations (safest)
//   - smart: Auto-approve safe writes, ask for exec + destructive ops
//   - yolo: Auto-approve everything (dangerous)
//   - chat: No tool calls at all (conversation only)
package permissions

import (
	"path/filepath"
	"strings"
)

// Mode represents a permission mode.
type Mode string

const (
	ModeConfirm Mode = "confirm"
	ModeSmart   Mode = "smart"
	ModeYolo    Mode = "yolo"
	ModeChat    Mode = "chat"
)

// Category represents the risk category of a tool.
type Category int

const (
	CategoryRead    Category = iota // file_read, grep, glob, list_dir
	CategoryWrite                  // file_write, file_edit
	CategoryExecute                // shell_exec
)

// Checker evaluates whether a tool call needs user approval.
type Checker struct {
	mode        Mode
	projectRoot string
}

// NewChecker creates a permission checker for the given mode.
func NewChecker(mode string, projectRoot string) *Checker {
	m := Mode(mode)
	switch m {
	case ModeConfirm, ModeSmart, ModeYolo, ModeChat:
		// valid
	default:
		m = ModeConfirm
	}
	return &Checker{
		mode:        m,
		projectRoot: projectRoot,
	}
}

// NeedsApproval returns true if the tool call requires user confirmation.
func (c *Checker) NeedsApproval(toolName string, args string) bool {
	switch c.mode {
	case ModeYolo:
		return false // approve everything
	case ModeChat:
		return true // deny everything (tools shouldn't be called)
	case ModeConfirm:
		return c.confirmMode(toolName)
	case ModeSmart:
		return c.smartMode(toolName, args)
	default:
		return true
	}
}

// ToolsAllowed returns false if the mode disables all tools.
func (c *Checker) ToolsAllowed() bool {
	return c.mode != ModeChat
}

// GetMode returns the current permission mode.
func (c *Checker) GetMode() Mode {
	return c.mode
}

// confirmMode: approve reads, ask for writes and exec.
func (c *Checker) confirmMode(toolName string) bool {
	cat := categorize(toolName)
	return cat != CategoryRead // needs approval for write + execute
}

// smartMode: auto-approve reads + safe writes, ask for exec and destructive ops.
func (c *Checker) smartMode(toolName string, args string) bool {
	cat := categorize(toolName)

	switch cat {
	case CategoryRead:
		return false // always auto-approve reads

	case CategoryWrite:
		// Auto-approve writes within project directory
		if c.isWithinProject(args) {
			return false
		}
		return true // ask for writes outside project

	case CategoryExecute:
		// Auto-approve safe commands
		if isSafeCommand(args) {
			return false
		}
		return true // ask for potentially dangerous commands

	default:
		return true
	}
}

// categorize determines the risk category of a tool.
func categorize(toolName string) Category {
	switch toolName {
	case "file_read", "grep_search", "glob_search", "list_dir":
		return CategoryRead
	case "file_write", "file_edit":
		return CategoryWrite
	case "shell_exec":
		return CategoryExecute
	default:
		return CategoryExecute // unknown tools are high-risk
	}
}

// isWithinProject checks if the target path is within the project directory.
func (c *Checker) isWithinProject(args string) bool {
	if c.projectRoot == "" {
		return false
	}

	// Extract path from common tool argument patterns
	// This is a heuristic — we look for "path" field in args
	path := extractPath(args)
	if path == "" {
		return false
	}

	// Resolve to absolute and check containment
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absProject, err := filepath.Abs(c.projectRoot)
	if err != nil {
		return false
	}

	return strings.HasPrefix(absPath, absProject)
}

// isSafeCommand checks if a shell command is safe to auto-approve.
func isSafeCommand(args string) bool {
	safeCommands := []string{
		"go test",
		"go build",
		"go vet",
		"go fmt",
		"go mod tidy",
		"npm test",
		"npm run test",
		"npm run lint",
		"npm run build",
		"npx tsc",
		"cargo test",
		"cargo build",
		"cargo check",
		"cargo clippy",
		"python -m pytest",
		"python -m mypy",
		"make test",
		"make lint",
		"make check",
		"git diff",
		"git status",
		"git log",
		"git show",
	}

	lower := strings.ToLower(args)
	for _, safe := range safeCommands {
		if strings.Contains(lower, safe) {
			return true
		}
	}

	return false
}

// extractPath attempts to extract a file path from tool arguments.
// Handles common JSON arg patterns like {"path": "..."} or {"file_path": "..."}.
func extractPath(args string) string {
	// Simple extraction — look for common path keys
	for _, key := range []string{`"path"`, `"file_path"`, `"target"`} {
		idx := strings.Index(args, key)
		if idx < 0 {
			continue
		}

		// Find the value after the key
		rest := args[idx+len(key):]
		// Skip ': "' or ':"'
		rest = strings.TrimLeft(rest, `: "`)

		// Find end of string value
		endIdx := strings.IndexByte(rest, '"')
		if endIdx > 0 {
			return rest[:endIdx]
		}
	}

	return ""
}
