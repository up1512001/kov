// Package git provides git operations for kov sessions.
// It handles snapshots before edits, auto-commits after tasks,
// and stash management for clean recovery.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Git provides git operations for a project directory.
type Git struct {
	dir          string
	commitPrefix string
	autoCommit   bool
}

// New creates a new Git instance.
func New(dir, commitPrefix string, autoCommit bool) *Git {
	if commitPrefix == "" {
		commitPrefix = "[kov]"
	}
	return &Git{
		dir:          dir,
		commitPrefix: commitPrefix,
		autoCommit:   autoCommit,
	}
}

// IsRepo checks if the directory is a git repository.
func (g *Git) IsRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = g.dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// CurrentBranch returns the current branch name.
func (g *Git) CurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = g.dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CurrentHash returns the current commit hash (short).
func (g *Git) CurrentHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = g.dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Snapshot creates a lightweight snapshot commit before edits.
// This allows easy rollback if the agent makes mistakes.
func (g *Git) Snapshot(ctx context.Context, label string) (string, error) {
	if !g.IsRepo() {
		return "", nil
	}

	// Check if there's anything to snapshot
	if !g.HasChanges() {
		return g.CurrentHash(), nil
	}

	// Stage all changes
	if err := g.run(ctx, "add", "-A"); err != nil {
		return "", fmt.Errorf("staging: %w", err)
	}

	msg := fmt.Sprintf("%s snapshot: %s", g.commitPrefix, label)
	if err := g.run(ctx, "commit", "-m", msg, "--allow-empty"); err != nil {
		return "", fmt.Errorf("committing: %w", err)
	}

	return g.CurrentHash(), nil
}

// AutoCommit commits current changes with a descriptive message.
func (g *Git) AutoCommit(ctx context.Context, description string) error {
	if !g.autoCommit || !g.IsRepo() {
		return nil
	}

	if !g.HasChanges() {
		return nil
	}

	if err := g.run(ctx, "add", "-A"); err != nil {
		return fmt.Errorf("staging: %w", err)
	}

	msg := fmt.Sprintf("%s %s", g.commitPrefix, description)
	if err := g.run(ctx, "commit", "-m", msg); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// HasChanges returns true if there are uncommitted changes.
func (g *Git) HasChanges() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = g.dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// Diff returns the current diff (unstaged changes).
func (g *Git) Diff(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff")
	cmd.Dir = g.dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// DiffStaged returns the staged diff.
func (g *Git) DiffStaged(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--staged")
	cmd.Dir = g.dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Rollback reverts to a specific commit hash.
func (g *Git) Rollback(ctx context.Context, hash string) error {
	if hash == "" {
		return fmt.Errorf("no hash to rollback to")
	}
	return g.run(ctx, "reset", "--hard", hash)
}

// Log returns recent commit history.
func (g *Git) Log(ctx context.Context, count int) (string, error) {
	if count <= 0 {
		count = 10
	}
	cmd := exec.CommandContext(ctx, "git", "log",
		fmt.Sprintf("-n%d", count),
		"--oneline",
		"--no-decorate",
	)
	cmd.Dir = g.dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Stash saves current changes to stash (for clean recovery).
func (g *Git) Stash(ctx context.Context, message string) error {
	if message == "" {
		message = fmt.Sprintf("kov-stash-%d", time.Now().Unix())
	}
	return g.run(ctx, "stash", "push", "-m", message)
}

// StashPop restores the most recent stash.
func (g *Git) StashPop(ctx context.Context) error {
	return g.run(ctx, "stash", "pop")
}

// run executes a git command.
func (g *Git) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.dir
	cmd.Env = append(cmd.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_COMMITTER_NAME=kov",
		"GIT_COMMITTER_EMAIL=kov@trykov.dev",
		"GIT_AUTHOR_NAME=kov",
		"GIT_AUTHOR_EMAIL=kov@trykov.dev",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
