package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "utils.go"), []byte("package main\n\nfunc helper() string {\n\treturn \"help\"\n}\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "other.go"), []byte("package sub\n\nfunc Other() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project\n\nThis is a test.\n"), 0o644)

	return dir
}

func TestFileRead(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewFileRead(dir)

	// Basic read
	args, _ := json.Marshal(map[string]interface{}{"path": "main.go"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "package main") {
		t.Error("expected file content")
	}

	// Read with line range
	args, _ = json.Marshal(map[string]interface{}{
		"path": "main.go", "start_line": 3, "end_line": 4,
	})
	result, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("line range: %v", err)
	}
	if !strings.Contains(result, "func main") {
		t.Errorf("expected func main in line range, got: %q", result)
	}
}

func TestFileRead_PathTraversal(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewFileRead(dir)

	args, _ := json.Marshal(map[string]interface{}{"path": "../../etc/passwd"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected path traversal error")
	}
	if !strings.Contains(err.Error(), "outside project") {
		t.Errorf("expected 'outside project' error, got: %v", err)
	}
}

func TestFileWrite(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewFileWrite(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"path":    "new_file.txt",
		"content": "hello world",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "11 bytes") {
		t.Errorf("expected byte count in result, got: %s", result)
	}

	// Verify file was written
	data, _ := os.ReadFile(filepath.Join(dir, "new_file.txt"))
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestFileWrite_CreateDirs(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewFileWrite(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"path":    "deep/nested/dir/file.txt",
		"content": "nested content",
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "deep/nested/dir/file.txt"))
	if string(data) != "nested content" {
		t.Error("expected nested file to be created")
	}
}

func TestFileEdit(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewFileEdit(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"path": "main.go",
		"edits": []map[string]string{
			{"old_text": "hello", "new_text": "world"},
		},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "1 edit") {
		t.Error("expected '1 edit' in result")
	}

	data, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(data), "world") {
		t.Error("expected edit to be applied")
	}
	if strings.Contains(string(data), "hello") {
		t.Error("expected 'hello' to be replaced")
	}
}

func TestFileEdit_NotFound(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewFileEdit(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"path": "main.go",
		"edits": []map[string]string{
			{"old_text": "nonexistent text", "new_text": "replacement"},
		},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for nonexistent text")
	}
}

func TestShellExec(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewShellExec(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'hello from shell'",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "hello from shell") {
		t.Errorf("expected 'hello from shell', got: %q", result)
	}
}

func TestShellExec_ExitCode(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewShellExec(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"command": "exit 42",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("expected exit code 42, got: %v", err)
	}
}

func TestGrepSearch(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewGrepSearch(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"pattern": "func",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Should find functions in main.go, utils.go, and sub/other.go
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 matches, got %d: %s", len(lines), result)
	}
}

func TestGrepSearch_WithFilter(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewGrepSearch(dir)

	// Search only .md files
	args, _ := json.Marshal(map[string]interface{}{
		"pattern": "Test",
		"include": "*.md",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "README.md") {
		t.Error("expected result from README.md")
	}
	if strings.Contains(result, ".go") {
		t.Error("should not include .go files when filtering by .md")
	}
}

func TestGlobSearch(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewGlobSearch(dir)

	args, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Error("expected main.go in results")
	}
	if !strings.Contains(result, "utils.go") {
		t.Error("expected utils.go in results")
	}
}

func TestListDir(t *testing.T) {
	dir := setupTestDir(t)
	tool := NewListDir(dir)

	args, _ := json.Marshal(map[string]interface{}{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected main.go in listing")
	}
	if !strings.Contains(result, "sub/") {
		t.Error("expected sub/ directory in listing")
	}
}

func TestDefaultRegistry(t *testing.T) {
	dir := setupTestDir(t)
	reg := DefaultRegistry(dir)

	tools := reg.All()
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}

	// Verify all tools exist
	for _, name := range []string{"file_read", "file_write", "file_edit", "shell_exec", "grep_search", "glob_search", "list_dir"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("missing tool: %s", name)
		}
	}

	// Verify ForProvider works
	providerTools := reg.ForProvider()
	if len(providerTools) != 7 {
		t.Errorf("expected 7 provider tools, got %d", len(providerTools))
	}
}

func TestRegistry_Execute(t *testing.T) {
	dir := setupTestDir(t)
	reg := DefaultRegistry(dir)

	args, _ := json.Marshal(map[string]interface{}{"path": "main.go"})
	result, err := reg.Execute(context.Background(), "file_read", args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "package main") {
		t.Error("expected file content")
	}
}

func TestRegistry_ExecuteNotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
