package config

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DetectedProvider represents a provider found on the system.
type DetectedProvider struct {
	Name   string // anthropic, openai, google, ollama
	Source string // e.g. "env:ANTHROPIC_API_KEY", "claude-code-cli", "local:11434"
	APIKey string // populated for cloud providers
}

// DetectedCLI represents an AI coding CLI tool found on the system.
type DetectedCLI struct {
	Name    string // "claude", "codex"
	Path    string // absolute path to binary
	Version string // version string from --version
}

// DetectSystemProviders scans the system for available AI providers.
// It checks env vars, CLI tool configs, and running local services.
func DetectSystemProviders() []DetectedProvider {
	var detected []DetectedProvider

	// 1. Environment variables (most common)
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		detected = append(detected, DetectedProvider{
			Name: "anthropic", Source: "env:ANTHROPIC_API_KEY", APIKey: key,
		})
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		detected = append(detected, DetectedProvider{
			Name: "openai", Source: "env:OPENAI_API_KEY", APIKey: key,
		})
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		detected = append(detected, DetectedProvider{
			Name: "google", Source: "env:GEMINI_API_KEY", APIKey: key,
		})
	} else if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		detected = append(detected, DetectedProvider{
			Name: "google", Source: "env:GOOGLE_API_KEY", APIKey: key,
		})
	}

	// 2. Claude Code CLI — check if installed and has config
	if !hasProviderNamed(detected, "anthropic") {
		if key := detectClaudeCodeCLI(); key != "" {
			detected = append(detected, DetectedProvider{
				Name: "anthropic", Source: "claude-code-cli", APIKey: key,
			})
		}
	}

	// 3. Codex CLI — check for OpenAI key from codex config
	if !hasProviderNamed(detected, "openai") {
		if key := detectCodexCLI(); key != "" {
			detected = append(detected, DetectedProvider{
				Name: "openai", Source: "codex-cli", APIKey: key,
			})
		}
	}

	// 4. Ollama — only if actually running
	if isOllamaRunning() {
		detected = append(detected, DetectedProvider{
			Name: "ollama", Source: "local:11434",
		})
	}

	return detected
}

// DetectCLITools scans the system for known AI coding CLI tools.
// This is for display purposes — shows what's installed regardless of API key status.
func DetectCLITools() []DetectedCLI {
	var detected []DetectedCLI

	clis := []struct {
		name  string
		vFlag string
	}{
		{"claude", "--version"},
		{"codex", "--version"},
	}

	for _, cli := range clis {
		path, err := exec.LookPath(cli.name)
		if err != nil {
			continue
		}

		version := ""
		out, err := exec.Command(path, cli.vFlag).Output()
		if err == nil {
			version = strings.TrimSpace(string(out))
			if idx := strings.IndexByte(version, '\n'); idx >= 0 {
				version = version[:idx]
			}
		}

		detected = append(detected, DetectedCLI{
			Name:    cli.name,
			Path:    path,
			Version: version,
		})
	}

	return detected
}

// IsFirstRun returns true if no user config file exists.
func IsFirstRun() bool {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return true
	}
	configPath := filepath.Join(userConfigDir, "kov", "config.yaml")
	_, err = os.Stat(configPath)
	return os.IsNotExist(err)
}

// hasProviderNamed checks if a provider name already exists in the list.
func hasProviderNamed(providers []DetectedProvider, name string) bool {
	for _, p := range providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

// isCommandAvailable checks if a CLI tool exists in PATH.
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// detectClaudeCodeCLI checks for Claude Code CLI and reads its API key.
func detectClaudeCodeCLI() string {
	if !isCommandAvailable("claude") {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Check known Claude Code config locations
	configPaths := []string{
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".claude", "config.json"),
		filepath.Join(home, ".config", "claude", "config.json"),
	}

	for _, path := range configPaths {
		if key := readAPIKeyFromJSON(path, "apiKey"); key != "" {
			return key
		}
	}

	// Claude Code may use OAuth (no stored API key). Return empty.
	return ""
}

// detectCodexCLI checks for OpenAI Codex CLI and reads its API key.
func detectCodexCLI() string {
	if !isCommandAvailable("codex") {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configPaths := []string{
		filepath.Join(home, ".codex", "config.json"),
		filepath.Join(home, ".config", "codex", "config.json"),
	}

	for _, path := range configPaths {
		if key := readAPIKeyFromJSON(path, "apiKey"); key != "" {
			return key
		}
		if key := readAPIKeyFromJSON(path, "api_key"); key != "" {
			return key
		}
	}

	return ""
}

// isOllamaRunning pings the Ollama API to check if it's actually running.
func isOllamaRunning() bool {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/api/tags", nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// readAPIKeyFromJSON reads a JSON config file and extracts an API key by field name.
func readAPIKeyFromJSON(path, keyField string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return ""
	}

	if val, ok := config[keyField]; ok {
		if str, ok := val.(string); ok && str != "" {
			return str
		}
	}

	return ""
}

// HasDetectedCLI returns true if Claude Code or Codex CLI is found on the system.
func HasDetectedCLI() (hasClaude, hasCodex bool) {
	hasClaude = isCommandAvailable("claude")
	hasCodex = isCommandAvailable("codex")
	return
}
