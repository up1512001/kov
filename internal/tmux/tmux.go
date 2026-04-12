// Package tmux provides session isolation by running kov in dedicated tmux sessions.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsAvailable returns true if tmux is installed and accessible.
func IsAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// IsInsideSession returns true if currently running inside a tmux session.
func IsInsideSession() bool {
	return os.Getenv("TMUX") != ""
}

// NewSession creates a new detached tmux session with the given name.
func NewSession(name string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunInSession sends a command to be executed inside a tmux session.
func RunInSession(name, command string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", name, command, "Enter")
	return cmd.Run()
}

// AttachSession attaches to an existing tmux session, replacing the current process.
func AttachSession(name string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// Use syscall exec to replace the current process
	cmd := exec.Command(tmuxPath, "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// KillSession destroys a tmux session.
func KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	return cmd.Run()
}

// ListSessions returns active tmux session names that match a prefix.
func ListSessions(prefix string) ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" && strings.HasPrefix(line, prefix) {
			matches = append(matches, line)
		}
	}
	return matches, nil
}
