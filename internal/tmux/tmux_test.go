package tmux

import (
	"os"
	"testing"
)

func TestIsAvailable(t *testing.T) {
	// This is a system-level check; just ensure it doesn't panic.
	_ = IsAvailable()
}

func TestIsInsideSession(t *testing.T) {
	// When TMUX is not set, should return false.
	original := os.Getenv("TMUX")
	os.Unsetenv("TMUX")
	defer func() {
		if original != "" {
			os.Setenv("TMUX", original)
		}
	}()

	if IsInsideSession() {
		t.Error("expected false when TMUX env is not set")
	}
}

func TestIsInsideSession_Set(t *testing.T) {
	original := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	defer func() {
		if original != "" {
			os.Setenv("TMUX", original)
		} else {
			os.Unsetenv("TMUX")
		}
	}()

	if !IsInsideSession() {
		t.Error("expected true when TMUX env is set")
	}
}
