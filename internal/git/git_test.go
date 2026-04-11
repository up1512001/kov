package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Init git repo
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")

	// Create initial commit
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644)
	run("add", "-A")
	run("commit", "-m", "initial commit")

	return dir
}

func TestIsRepo(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", true)

	if !g.IsRepo() {
		t.Error("expected IsRepo to be true")
	}

	// Non-repo dir
	g2 := New(t.TempDir(), "[kov]", true)
	if g2.IsRepo() {
		t.Error("expected IsRepo to be false for non-repo")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", true)

	branch := g.CurrentBranch()
	if branch != "main" && branch != "master" {
		t.Errorf("expected main or master, got %q", branch)
	}
}

func TestHasChanges(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", true)

	if g.HasChanges() {
		t.Error("should have no changes after initial commit")
	}

	// Create a new file
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file"), 0o644)
	if !g.HasChanges() {
		t.Error("should detect new file as change")
	}
}

func TestSnapshot(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", true)
	ctx := context.Background()

	// Create a change
	os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main\n"), 0o644)

	hash, err := g.Snapshot(ctx, "before refactor")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}

	// Should have a clean state now
	if g.HasChanges() {
		t.Error("should have no changes after snapshot")
	}
}

func TestAutoCommit(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", true)
	ctx := context.Background()

	os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package auth\n"), 0o644)

	err := g.AutoCommit(ctx, "implement JWT auth")
	if err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}

	// Verify commit message
	log, _ := g.Log(ctx, 1)
	if log == "" {
		t.Error("expected commit in log")
	}
}

func TestAutoCommit_Disabled(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", false) // autoCommit disabled
	ctx := context.Background()

	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("test"), 0o644)

	err := g.AutoCommit(ctx, "should not commit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still have changes (not committed)
	if !g.HasChanges() {
		t.Error("changes should remain when autoCommit is disabled")
	}
}

func TestRollback(t *testing.T) {
	dir := setupGitRepo(t)
	g := New(dir, "[kov]", true)
	ctx := context.Background()

	// Get initial hash
	initialHash := g.CurrentHash()

	// Make changes and commit
	os.WriteFile(filepath.Join(dir, "bad.go"), []byte("bad code\n"), 0o644)
	g.AutoCommit(ctx, "bad change")

	// Verify bad file exists
	if _, err := os.Stat(filepath.Join(dir, "bad.go")); err != nil {
		t.Fatal("bad.go should exist after commit")
	}

	// Rollback
	err := g.Rollback(ctx, initialHash)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Verify bad file is gone
	if _, err := os.Stat(filepath.Join(dir, "bad.go")); !os.IsNotExist(err) {
		t.Error("bad.go should not exist after rollback")
	}
}
