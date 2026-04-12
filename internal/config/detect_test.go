package config

import (
	"testing"
)

func TestDetectCLITools(t *testing.T) {
	// This test runs against the real system — it's an integration test.
	// It should not fail if CLIs are not installed; it just returns fewer results.
	detected := DetectCLITools()

	for _, cli := range detected {
		if cli.Name == "" {
			t.Error("detected CLI with empty name")
		}
		if cli.Path == "" {
			t.Errorf("detected CLI %s with empty path", cli.Name)
		}
	}
}

func TestDetectedCLI_Fields(t *testing.T) {
	d := DetectedCLI{
		Name:    "claude",
		Path:    "/usr/local/bin/claude",
		Version: "Claude Code v2.1.91",
	}

	if d.Name != "claude" {
		t.Errorf("expected name 'claude', got %s", d.Name)
	}
	if d.Path != "/usr/local/bin/claude" {
		t.Errorf("expected path '/usr/local/bin/claude', got %s", d.Path)
	}
}
