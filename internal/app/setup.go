package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/up1512001/kov/internal/config"
)

// Setup styles
var (
	setupTitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true)

	setupSuccess = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34D399"))

	setupWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24"))

	setupError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F87171")).
			Bold(true)

	setupMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	setupHighlight = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)
)

// needsSetup checks if KOV needs first-time configuration.
func needsSetup() bool {
	return config.IsFirstRun()
}

// runSetupWizard runs the first-time configuration wizard.
// It detects available providers, shows results, and saves config.
func runSetupWizard() (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)

	// Welcome banner
	fmt.Println()
	fmt.Println(setupTitle.Render("  ⚡ Welcome to Kov — Indestructible AI Coding"))
	fmt.Println()
	fmt.Println(setupMuted.Render("  Let's set up your providers. This only takes a moment."))
	fmt.Println()

	// Detect providers
	fmt.Println(setupMuted.Render("  🔍 Scanning for providers..."))
	fmt.Println()
	detected := config.DetectSystemProviders()
	hasClaude, hasCodex := config.HasDetectedCLI()

	// Show detected CLIs
	if hasClaude || hasCodex {
		fmt.Println(setupMuted.Render("  CLI tools found:"))
		if hasClaude {
			fmt.Println(setupSuccess.Render("    ✓ claude") + setupMuted.Render("  (Claude Code CLI)"))
		}
		if hasCodex {
			fmt.Println(setupSuccess.Render("    ✓ codex") + setupMuted.Render("  (OpenAI Codex CLI)"))
		}
		fmt.Println()
	}

	// Show detected providers
	allProviders := []struct {
		name    string
		display string
	}{
		{"anthropic", "Anthropic (Claude)"},
		{"openai", "OpenAI (GPT/o-series)"},
		{"google", "Google (Gemini)"},
		{"ollama", "Ollama (local)"},
	}

	fmt.Println(setupMuted.Render("  Providers:"))
	for _, p := range allProviders {
		found := findDetected(detected, p.name)
		if found != nil {
			source := setupMuted.Render(fmt.Sprintf("  (via %s)", found.Source))
			fmt.Printf("    %s %s\n", setupSuccess.Render("✓ "+p.display), source)
		} else {
			fmt.Printf("    %s\n", setupMuted.Render("✗ "+p.display))
		}
	}
	fmt.Println()

	// If no providers found, prompt for API key
	if len(detected) == 0 {
		fmt.Println(setupWarning.Render("  ⚠ No providers detected!"))
		fmt.Println(setupMuted.Render("  You need at least one provider to use Kov."))
		fmt.Println()
		fmt.Println(setupMuted.Render("  Quickest setup — paste your API key:"))
		fmt.Println()

		// Try to get an Anthropic key
		fmt.Print(setupHighlight.Render("  ANTHROPIC_API_KEY") + setupMuted.Render(" (or press Enter to skip): "))
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key != "" {
			detected = append(detected, config.DetectedProvider{
				Name: "anthropic", Source: "manual", APIKey: key,
			})
		}

		if len(detected) == 0 {
			// Try OpenAI
			fmt.Print(setupHighlight.Render("  OPENAI_API_KEY") + setupMuted.Render(" (or press Enter to skip): "))
			key, _ = reader.ReadString('\n')
			key = strings.TrimSpace(key)
			if key != "" {
				detected = append(detected, config.DetectedProvider{
					Name: "openai", Source: "manual", APIKey: key,
				})
			}
		}

		if len(detected) == 0 {
			// Try Google
			fmt.Print(setupHighlight.Render("  GEMINI_API_KEY") + setupMuted.Render(" (or press Enter to skip): "))
			key, _ = reader.ReadString('\n')
			key = strings.TrimSpace(key)
			if key != "" {
				detected = append(detected, config.DetectedProvider{
					Name: "google", Source: "manual", APIKey: key,
				})
			}
		}

		if len(detected) == 0 {
			fmt.Println()
			fmt.Println(setupError.Render("  ✗ No providers configured. Set an API key to get started:"))
			fmt.Println(setupMuted.Render("    export ANTHROPIC_API_KEY=sk-ant-..."))
			fmt.Println(setupMuted.Render("    export OPENAI_API_KEY=sk-..."))
			fmt.Println(setupMuted.Render("    export GEMINI_API_KEY=..."))
			fmt.Println()
			return nil, fmt.Errorf("no providers configured")
		}

		fmt.Println()
	}

	// Build config from detected providers
	cfg := config.Defaults()
	for _, d := range detected {
		switch d.Name {
		case "anthropic":
			cfg.Providers.Anthropic = &config.ProviderConfig{APIKey: d.APIKey}
			if cfg.Provider == "" || cfg.Provider == "anthropic" {
				cfg.Provider = "anthropic"
				cfg.Model = "claude-sonnet-4-20250514"
			}
		case "openai":
			cfg.Providers.OpenAI = &config.ProviderConfig{APIKey: d.APIKey}
			if cfg.Provider == "" {
				cfg.Provider = "openai"
				cfg.Model = "gpt-4.1"
			}
		case "google":
			cfg.Providers.Google = &config.ProviderConfig{APIKey: d.APIKey}
			if cfg.Provider == "" {
				cfg.Provider = "google"
				cfg.Model = "gemini-2.5-pro"
			}
		case "ollama":
			url := "http://localhost:11434"
			if d.Source != "" && strings.HasPrefix(d.Source, "local:") {
				url = "http://localhost:" + strings.TrimPrefix(d.Source, "local:")
			}
			cfg.Providers.Ollama = &config.OllamaConfig{
				URL:   url,
				Model: "qwen3:8b",
			}
			if cfg.Provider == "" {
				cfg.Provider = "ollama"
				cfg.Model = "qwen3:8b"
			}
		}
	}

	// Show summary
	fmt.Println(setupMuted.Render("  ─────────────────────────────────────"))
	fmt.Printf("  %s %s\n", setupMuted.Render("Default provider:"), setupHighlight.Render(cfg.Provider))
	fmt.Printf("  %s %s\n", setupMuted.Render("Default model:   "), setupHighlight.Render(cfg.Model))
	fmt.Println()

	// Ask to save
	fmt.Print(setupMuted.Render("  Save configuration? ") + setupHighlight.Render("[Y/n] "))
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "" || answer == "y" || answer == "yes" {
		if err := saveConfig(cfg); err != nil {
			fmt.Printf("  %s\n", setupWarning.Render("⚠ Could not save config: "+err.Error()))
			fmt.Println(setupMuted.Render("  Continuing with detected settings..."))
		} else {
			configDir, _ := os.UserConfigDir()
			configPath := filepath.Join(configDir, "kov", "config.yaml")
			fmt.Printf("  %s %s\n", setupSuccess.Render("✓ Config saved to"), setupMuted.Render(configPath))
		}
	}

	fmt.Println()
	return cfg, nil
}

// saveConfig writes a minimal config file to ~/.config/kov/config.yaml.
func saveConfig(cfg *config.Config) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	kovDir := filepath.Join(configDir, "kov")
	if err := os.MkdirAll(kovDir, 0750); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# Kov configuration — generated by setup wizard\n")
	sb.WriteString("# Edit this file or set environment variables to configure providers.\n\n")
	sb.WriteString(fmt.Sprintf("provider: %s\n", cfg.Provider))
	sb.WriteString(fmt.Sprintf("model: %s\n", cfg.Model))
	sb.WriteString("permissions: confirm\n\n")

	sb.WriteString("providers:\n")
	if cfg.Providers.Anthropic != nil && cfg.Providers.Anthropic.APIKey != "" {
		sb.WriteString("  anthropic:\n")
		sb.WriteString(fmt.Sprintf("    apiKey: %s\n", cfg.Providers.Anthropic.APIKey))
	} else {
		sb.WriteString("  anthropic: {}  # Set ANTHROPIC_API_KEY env var\n")
	}
	if cfg.Providers.OpenAI != nil && cfg.Providers.OpenAI.APIKey != "" {
		sb.WriteString("  openai:\n")
		sb.WriteString(fmt.Sprintf("    apiKey: %s\n", cfg.Providers.OpenAI.APIKey))
	}
	if cfg.Providers.Google != nil && cfg.Providers.Google.APIKey != "" {
		sb.WriteString("  google:\n")
		sb.WriteString(fmt.Sprintf("    apiKey: %s\n", cfg.Providers.Google.APIKey))
	}
	if cfg.Providers.Ollama != nil {
		sb.WriteString("  ollama:\n")
		sb.WriteString(fmt.Sprintf("    url: %s\n", cfg.Providers.Ollama.URL))
		sb.WriteString(fmt.Sprintf("    model: %s\n", cfg.Providers.Ollama.Model))
	}

	return os.WriteFile(filepath.Join(kovDir, "config.yaml"), []byte(sb.String()), 0640)
}

// findDetected returns the first DetectedProvider with the given name.
func findDetected(providers []config.DetectedProvider, name string) *config.DetectedProvider {
	for _, p := range providers {
		if p.Name == name {
			return &p
		}
	}
	return nil
}
