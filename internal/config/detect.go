package config

import (
	"os/exec"
	"strings"
)

// DetectedCLI represents an AI coding CLI tool found on the system.
type DetectedCLI struct {
	Name    string // "claude", "codex"
	Path    string // absolute path to binary
	Version string // version string from --version
}

// DetectCLITools scans the system for known AI coding CLI tools.
func DetectCLITools() []DetectedCLI {
	var detected []DetectedCLI

	clis := []struct {
		name    string
		vFlag   string
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
			// Take only the first line
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
