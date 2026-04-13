// Package context handles assembling the context that gets sent to the LLM.
// This includes loading KOV.md (project instructions), .kovignore patterns,
// generating a repo map, and assembling the final prompt.
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultKovFile     = "KOV.md"
	defaultIgnoreFile  = ".kovignore"
	maxRepoMapEntries  = 500
	maxKovFileSize     = 32 * 1024 // 32KB
)

// ProjectContext holds all project-level context for the LLM.
type ProjectContext struct {
	ProjectDir         string
	Instructions       string   // contents of KOV.md or CLAUDE.md
	AgentsInstructions string   // contents of AGENTS.md (OpenAI standard)
	RepoMap            string   // file tree
	IgnorePatterns     []string // .kovignore patterns
}

// LoadProjectContext loads KOV.md, AGENTS.md, .kovignore, and generates the repo map.
func LoadProjectContext(projectDir string) (*ProjectContext, error) {
	ctx := &ProjectContext{
		ProjectDir: projectDir,
	}

	// Load .kovignore patterns
	ctx.IgnorePatterns = loadIgnorePatterns(projectDir)

	// Load KOV.md / CLAUDE.md (project-specific instructions)
	ctx.Instructions = loadKovFile(projectDir)

	// Load AGENTS.md (OpenAI-originated standard, adopted by 60K+ repos)
	ctx.AgentsInstructions = loadAgentsFile(projectDir)

	// Generate repo map (file tree)
	ctx.RepoMap = generateRepoMap(projectDir, ctx.IgnorePatterns)

	return ctx, nil
}

// ToSystemContext returns the formatted context string for the system prompt.
func (pc *ProjectContext) ToSystemContext() string {
	var parts []string

	if pc.Instructions != "" {
		parts = append(parts, fmt.Sprintf("## Project Instructions (KOV.md)\n\n%s", pc.Instructions))
	}

	if pc.AgentsInstructions != "" {
		parts = append(parts, fmt.Sprintf("## Agent Instructions (AGENTS.md)\n\n%s", pc.AgentsInstructions))
	}

	if pc.RepoMap != "" {
		parts = append(parts, fmt.Sprintf("## Repository Structure\n\n```\n%s```", pc.RepoMap))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// loadKovFile reads the KOV.md project instructions file.
func loadKovFile(projectDir string) string {
	// Try KOV.md, then CLAUDE.md (compatibility), then .kov/instructions.md
	candidates := []string{
		filepath.Join(projectDir, defaultKovFile),
		filepath.Join(projectDir, "CLAUDE.md"),
		filepath.Join(projectDir, ".kov", "instructions.md"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		// Enforce size limit
		if len(content) > maxKovFileSize {
			content = content[:maxKovFileSize] + "\n\n[... truncated at 32KB ...]"
		}
		return content
	}

	return ""
}

// loadAgentsFile reads the AGENTS.md file (OpenAI-originated standard, adopted by 60K+ repos).
// This provides AI agent-specific instructions for the project.
func loadAgentsFile(projectDir string) string {
	candidates := []string{
		filepath.Join(projectDir, "AGENTS.md"),
		filepath.Join(projectDir, ".github", "AGENTS.md"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxKovFileSize {
			content = content[:maxKovFileSize] + "\n\n[... truncated at 32KB ...]"
		}
		return content
	}

	return ""
}

// loadIgnorePatterns reads .kovignore patterns.
func loadIgnorePatterns(projectDir string) []string {
	// Start with defaults
	defaults := []string{
		".git",
		"node_modules",
		"vendor",
		".venv",
		"__pycache__",
		".DS_Store",
		"*.pyc",
		"*.o",
		"*.exe",
		"dist",
		"build",
		"coverage",
		".next",
		".nuxt",
		"target",
	}

	// Read .kovignore
	path := filepath.Join(projectDir, defaultIgnoreFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}

	lines := strings.Split(string(data), "\n")
	patterns := make([]string, 0, len(defaults)+len(lines))
	patterns = append(patterns, defaults...)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns
}

// generateRepoMap creates a file tree string of the project.
func generateRepoMap(projectDir string, ignorePatterns []string) string {
	var sb strings.Builder
	count := 0

	filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if count >= maxRepoMapEntries {
			return filepath.SkipAll
		}

		relPath, _ := filepath.Rel(projectDir, path)
		if relPath == "." {
			return nil
		}

		// Check ignore patterns
		base := filepath.Base(path)
		for _, pattern := range ignorePatterns {
			if matched, _ := filepath.Match(pattern, base); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Direct name match
			if base == pattern {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Build tree line
		depth := strings.Count(relPath, string(os.PathSeparator))
		indent := strings.Repeat("  ", depth)

		if info.IsDir() {
			fmt.Fprintf(&sb, "%s📁 %s/\n", indent, base)
		} else {
			size := formatSize(info.Size())
			fmt.Fprintf(&sb, "%s  %s (%s)\n", indent, base, size)
		}

		count++
		return nil
	})

	if count >= maxRepoMapEntries {
		sb.WriteString(fmt.Sprintf("\n... (%d+ files, showing first %d)\n", count, maxRepoMapEntries))
	}

	return sb.String()
}

// formatSize returns a human-readable file size.
func formatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

// shouldIgnore checks if a path should be ignored.
func shouldIgnore(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if name == pattern {
			return true
		}
	}
	return false
}
