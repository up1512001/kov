package verify

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/utsavkovy/kov/internal/tools"
)

func TestAutoDetect_GoProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), DefaultRunnerConfig())
	cmd := r.AutoDetect()

	if cmd != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", cmd)
	}
}

func TestAutoDetect_NodeProject(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"scripts": {"test": "jest", "lint": "eslint ."}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644)

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), DefaultRunnerConfig())
	cmd := r.AutoDetect()

	if cmd != "npm run test" {
		t.Errorf("expected 'npm run test', got %q", cmd)
	}
}

func TestAutoDetect_PythonProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.pytest]"), 0o644)

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), DefaultRunnerConfig())
	cmd := r.AutoDetect()

	if cmd != "python -m pytest" {
		t.Errorf("expected 'python -m pytest', got %q", cmd)
	}
}

func TestAutoDetect_RustProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0o644)

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), DefaultRunnerConfig())
	cmd := r.AutoDetect()

	if cmd != "cargo test" {
		t.Errorf("expected 'cargo test', got %q", cmd)
	}
}

func TestAutoDetect_NoProject(t *testing.T) {
	dir := t.TempDir()

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), DefaultRunnerConfig())
	cmd := r.AutoDetect()

	if cmd != "" {
		t.Errorf("expected empty command, got %q", cmd)
	}
}

func TestAutoDetect_Makefile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0o644)

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), DefaultRunnerConfig())
	cmd := r.AutoDetect()

	if cmd != "make test" {
		t.Errorf("expected 'make test', got %q", cmd)
	}
}

func TestRun_WithExplicitCommand(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultRunnerConfig()
	cfg.Command = "echo 'tests passed'"

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), cfg)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Error("expected verification to pass")
	}
}

func TestRun_FailingCommand(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultRunnerConfig()
	cfg.Command = "exit 1"

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), cfg)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed {
		t.Error("expected verification to fail")
	}
}

func TestVerifyFixLoop_PassesFirstTry(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultRunnerConfig()
	cfg.Command = "echo ok"

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), cfg)

	fixCalled := false
	result, err := r.VerifyFixLoop(context.Background(), func(ctx context.Context, output string) error {
		fixCalled = true
		return nil
	})

	if err != nil {
		t.Fatalf("VerifyFixLoop: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass")
	}
	if fixCalled {
		t.Error("fix should not be called when verification passes")
	}
}

func TestRun_NoCommand(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultRunnerConfig()
	cfg.Command = ""

	r := NewRunner(tools.DefaultRegistry(dir), dir, slog.Default(), cfg)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass when no command")
	}
}
