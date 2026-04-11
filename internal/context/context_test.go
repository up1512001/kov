package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create project structure
	os.MkdirAll(filepath.Join(dir, "cmd"), 0o755)
	os.MkdirAll(filepath.Join(dir, "internal", "api"), 0o755)
	os.MkdirAll(filepath.Join(dir, "node_modules", "foo"), 0o755) // should be ignored
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)     // should be ignored

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "cmd", "root.go"), []byte("package cmd\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "internal", "api", "handler.go"), []byte("package api\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "node_modules", "foo", "index.js"), []byte("// foo\n"), 0o644)

	return dir
}

func TestLoadProjectContext_NoKovFile(t *testing.T) {
	dir := setupTestProject(t)
	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if ctx.Instructions != "" {
		t.Error("expected empty instructions when no KOV.md")
	}
	if ctx.RepoMap == "" {
		t.Error("expected non-empty repo map")
	}
}

func TestLoadProjectContext_WithKovMd(t *testing.T) {
	dir := setupTestProject(t)
	os.WriteFile(filepath.Join(dir, "KOV.md"), []byte("# Project Rules\n\nAlways use Go modules."), 0o644)

	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if !strings.Contains(ctx.Instructions, "Project Rules") {
		t.Error("expected KOV.md content in instructions")
	}
}

func TestLoadProjectContext_ClaudeMdFallback(t *testing.T) {
	dir := setupTestProject(t)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Claude Instructions"), 0o644)

	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if !strings.Contains(ctx.Instructions, "Claude Instructions") {
		t.Error("expected CLAUDE.md fallback to work")
	}
}

func TestRepoMap_IgnoresGitAndNodeModules(t *testing.T) {
	dir := setupTestProject(t)
	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if strings.Contains(ctx.RepoMap, ".git") {
		t.Error("repo map should not contain .git directory")
	}
	if strings.Contains(ctx.RepoMap, "node_modules") {
		t.Error("repo map should not contain node_modules")
	}
}

func TestRepoMap_IncludesProjectFiles(t *testing.T) {
	dir := setupTestProject(t)
	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if !strings.Contains(ctx.RepoMap, "main.go") {
		t.Error("repo map should contain main.go")
	}
	if !strings.Contains(ctx.RepoMap, "handler.go") {
		t.Error("repo map should contain handler.go")
	}
	if !strings.Contains(ctx.RepoMap, "cmd") {
		t.Error("repo map should contain cmd directory")
	}
}

func TestKovIgnore_CustomPatterns(t *testing.T) {
	dir := setupTestProject(t)
	os.WriteFile(filepath.Join(dir, ".kovignore"), []byte("# Custom ignores\n*.log\ntmp\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "debug.log"), []byte("log content"), 0o644)

	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if strings.Contains(ctx.RepoMap, "debug.log") {
		t.Error("repo map should not contain *.log files when .kovignore excludes them")
	}
}

func TestToSystemContext_Format(t *testing.T) {
	dir := setupTestProject(t)
	os.WriteFile(filepath.Join(dir, "KOV.md"), []byte("Use Go."), 0o644)

	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	output := ctx.ToSystemContext()
	if !strings.Contains(output, "Project Instructions") {
		t.Error("expected 'Project Instructions' header")
	}
	if !strings.Contains(output, "Repository Structure") {
		t.Error("expected 'Repository Structure' header")
	}
}

func TestKovFile_SizeLimit(t *testing.T) {
	dir := setupTestProject(t)
	// Create a huge KOV.md (> 32KB)
	bigContent := strings.Repeat("x", 40000)
	os.WriteFile(filepath.Join(dir, "KOV.md"), []byte(bigContent), 0o644)

	ctx, err := LoadProjectContext(dir)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}

	if len(ctx.Instructions) > maxKovFileSize+100 {
		t.Errorf("KOV.md should be truncated, got %d bytes", len(ctx.Instructions))
	}
	if !strings.Contains(ctx.Instructions, "truncated") {
		t.Error("expected truncation notice")
	}
}
