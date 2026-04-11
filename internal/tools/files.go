package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileRead reads file contents (safe, read-only).
type FileRead struct {
	projectDir string
}

func NewFileRead(projectDir string) *FileRead {
	return &FileRead{projectDir: projectDir}
}

func (t *FileRead) Name() string        { return "file_read" }
func (t *FileRead) NeedsPermission() bool { return false }
func (t *FileRead) Category() ToolCategory { return CategoryRead }
func (t *FileRead) Description() string {
	return "Read the contents of a file. Returns the file content as text."
}
func (t *FileRead) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file (relative to project root)",
			},
			"start_line": map[string]interface{}{
				"type":        "integer",
				"description": "Start line number (1-indexed, optional)",
			},
			"end_line": map[string]interface{}{
				"type":        "integer",
				"description": "End line number (1-indexed, inclusive, optional)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FileRead) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	fullPath := resolvePath(t.projectDir, input.Path)
	if err := validatePath(t.projectDir, fullPath); err != nil {
		return "", err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	content := string(data)

	// Apply line range if specified
	if input.StartLine > 0 || input.EndLine > 0 {
		lines := strings.Split(content, "\n")
		start := max(1, input.StartLine) - 1
		end := len(lines)
		if input.EndLine > 0 {
			end = min(input.EndLine, len(lines))
		}
		if start >= len(lines) {
			return "", fmt.Errorf("start_line %d exceeds file length (%d lines)", input.StartLine, len(lines))
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return content, nil
}

// FileWrite creates or overwrites a file.
type FileWrite struct {
	projectDir string
}

func NewFileWrite(projectDir string) *FileWrite {
	return &FileWrite{projectDir: projectDir}
}

func (t *FileWrite) Name() string        { return "file_write" }
func (t *FileWrite) NeedsPermission() bool { return true }
func (t *FileWrite) Category() ToolCategory { return CategoryWrite }
func (t *FileWrite) Description() string {
	return "Create or overwrite a file with the given content. Creates parent directories if needed."
}
func (t *FileWrite) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file (relative to project root)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *FileWrite) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	fullPath := resolvePath(t.projectDir, input.Path)
	if err := validatePath(t.projectDir, fullPath); err != nil {
		return "", err
	}

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating directories: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(input.Content), 0o644); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(input.Content), input.Path), nil
}

// FileEdit applies a search-and-replace edit to a file.
type FileEdit struct {
	projectDir string
}

func NewFileEdit(projectDir string) *FileEdit {
	return &FileEdit{projectDir: projectDir}
}

func (t *FileEdit) Name() string        { return "file_edit" }
func (t *FileEdit) NeedsPermission() bool { return true }
func (t *FileEdit) Category() ToolCategory { return CategoryWrite }
func (t *FileEdit) Description() string {
	return "Edit a file by replacing exact text matches. Provide the old text and new text. Multiple replacements can be done at once."
}
func (t *FileEdit) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file (relative to project root)",
			},
			"edits": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"old_text": map[string]interface{}{
							"type":        "string",
							"description": "Exact text to find (must match exactly)",
						},
						"new_text": map[string]interface{}{
							"type":        "string",
							"description": "Text to replace it with",
						},
					},
					"required": []string{"old_text", "new_text"},
				},
				"description": "List of search-and-replace edits to apply",
			},
		},
		"required": []string{"path", "edits"},
	}
}

func (t *FileEdit) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path  string `json:"path"`
		Edits []struct {
			OldText string `json:"old_text"`
			NewText string `json:"new_text"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	fullPath := resolvePath(t.projectDir, input.Path)
	if err := validatePath(t.projectDir, fullPath); err != nil {
		return "", err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	content := string(data)
	applied := 0

	for _, edit := range input.Edits {
		if !strings.Contains(content, edit.OldText) {
			return "", fmt.Errorf("old_text not found in file: %q", truncateStr(edit.OldText, 80))
		}
		content = strings.Replace(content, edit.OldText, edit.NewText, 1)
		applied++
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return fmt.Sprintf("Applied %d edit(s) to %s", applied, input.Path), nil
}

// ListDir lists directory contents.
type ListDir struct {
	projectDir string
}

func NewListDir(projectDir string) *ListDir {
	return &ListDir{projectDir: projectDir}
}

func (t *ListDir) Name() string        { return "list_dir" }
func (t *ListDir) NeedsPermission() bool { return false }
func (t *ListDir) Category() ToolCategory { return CategoryRead }
func (t *ListDir) Description() string {
	return "List the contents of a directory. Returns file names, sizes, and types."
}
func (t *ListDir) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path (relative to project root, default '.')",
			},
		},
	}
}

func (t *ListDir) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	json.Unmarshal(args, &input)
	if input.Path == "" {
		input.Path = "."
	}

	fullPath := resolvePath(t.projectDir, input.Path)
	if err := validatePath(t.projectDir, fullPath); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("reading directory: %w", err)
	}

	var sb strings.Builder
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if entry.IsDir() {
			fmt.Fprintf(&sb, "📁 %s/\n", entry.Name())
		} else {
			fmt.Fprintf(&sb, "   %s (%d bytes)\n", entry.Name(), info.Size())
		}
	}

	return sb.String(), nil
}

// Path safety helpers

func resolvePath(projectDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(projectDir, path))
}

func validatePath(projectDir, fullPath string) error {
	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("resolving project dir: %w", err)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Prevent path traversal outside project
	if !strings.HasPrefix(absPath, absProject) {
		return fmt.Errorf("path %q is outside project directory", fullPath)
	}
	return nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
