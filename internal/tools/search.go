package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GrepSearch searches file contents using string matching.
type GrepSearch struct {
	projectDir string
}

func NewGrepSearch(projectDir string) *GrepSearch {
	return &GrepSearch{projectDir: projectDir}
}

func (t *GrepSearch) Name() string        { return "grep_search" }
func (t *GrepSearch) NeedsPermission() bool { return false }
func (t *GrepSearch) Category() ToolCategory { return CategoryRead }
func (t *GrepSearch) Description() string {
	return "Search for a text pattern across files in the project. Returns matching file paths and line content. Useful for finding function definitions, imports, usage patterns."
}
func (t *GrepSearch) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Text pattern to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory or file to search in (default: '.')",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "File extension filter (e.g., '*.go', '*.ts')",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results (default: 50)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Include    string `json:"include"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	if input.Path == "" {
		input.Path = "."
	}
	if input.MaxResults <= 0 {
		input.MaxResults = 50
	}

	searchDir := resolvePath(t.projectDir, input.Path)
	if err := validatePath(t.projectDir, searchDir); err != nil {
		return "", err
	}

	var results []string
	count := 0

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if count >= input.MaxResults {
			return filepath.SkipAll
		}

		// Skip directories, hidden files, and common exclusions
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == ".venv" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip large or binary files
		if info.Size() > 1024*1024 { // 1MB max
			return nil
		}

		// Apply include filter
		if input.Include != "" {
			matched, _ := filepath.Match(input.Include, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		// Read and search file
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		lines := strings.Split(content, "\n")
		relPath, _ := filepath.Rel(t.projectDir, path)

		for i, line := range lines {
			if strings.Contains(line, input.Pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
				count++
				if count >= input.MaxResults {
					break
				}
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if len(results) == 0 {
		return "No matches found.", nil
	}

	return strings.Join(results, "\n"), nil
}

// GlobSearch finds files matching a glob pattern.
type GlobSearch struct {
	projectDir string
}

func NewGlobSearch(projectDir string) *GlobSearch {
	return &GlobSearch{projectDir: projectDir}
}

func (t *GlobSearch) Name() string        { return "glob_search" }
func (t *GlobSearch) NeedsPermission() bool { return false }
func (t *GlobSearch) Category() ToolCategory { return CategoryRead }
func (t *GlobSearch) Description() string {
	return "Find files matching a glob pattern (e.g., '**/*.go', 'src/**/*.ts'). Returns file paths."
}
func (t *GlobSearch) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern (e.g., '**/*.go', 'src/**/*.ts')",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results (default: 100)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Pattern    string `json:"pattern"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	if input.MaxResults <= 0 {
		input.MaxResults = 100
	}

	var results []string
	count := 0

	err := filepath.Walk(t.projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if count >= input.MaxResults {
			return filepath.SkipAll
		}

		// Skip common exclusions
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(t.projectDir, path)
		matched, _ := filepath.Match(input.Pattern, filepath.Base(path))
		if matched {
			results = append(results, relPath)
			count++
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if len(results) == 0 {
		return "No files found.", nil
	}

	return strings.Join(results, "\n"), nil
}
