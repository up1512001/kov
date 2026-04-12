package cost

import (
	"strings"
	"testing"
)

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost float64
		want string
	}{
		{0.001, "$0.0010"},
		{0.005, "$0.0050"},
		{0.123, "$0.123"},
		{0.999, "$0.999"},
		{1.50, "$1.50"},
		{10.99, "$10.99"},
	}
	for _, tc := range tests {
		got := FormatCost(tc.cost)
		if got != tc.want {
			t.Errorf("FormatCost(%f) = %q, want %q", tc.cost, got, tc.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		tokens int
		want   string
	}{
		{100, "100"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{12400, "12.4K"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	}
	for _, tc := range tests {
		got := FormatTokens(tc.tokens)
		if got != tc.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tc.tokens, got, tc.want)
		}
	}
}

func TestFormatUsage(t *testing.T) {
	got := FormatUsage(0.23, 12400, 3200)
	if !strings.Contains(got, "$0.230") {
		t.Errorf("expected cost in usage: %s", got)
	}
	if !strings.Contains(got, "12.4K in") {
		t.Errorf("expected input tokens: %s", got)
	}
	if !strings.Contains(got, "3.2K out") {
		t.Errorf("expected output tokens: %s", got)
	}
}

func TestBudgetStatus_Unlimited(t *testing.T) {
	f := NewFormatter(0, 5.0)
	got := f.BudgetStatus(0.50)
	if got != "$0.500" {
		t.Errorf("expected just cost for unlimited, got %q", got)
	}
}

func TestBudgetStatus_UnderBudget(t *testing.T) {
	f := NewFormatter(10.0, 8.0)
	got := f.BudgetStatus(2.0)
	if !strings.Contains(got, "$2.00") {
		t.Errorf("expected cost: %s", got)
	}
	if !strings.Contains(got, "budget") {
		t.Errorf("expected budget info: %s", got)
	}
}

func TestBudgetStatus_Warning(t *testing.T) {
	f := NewFormatter(10.0, 8.0)
	got := f.BudgetStatus(9.0)
	if !strings.Contains(got, "⚠️") {
		t.Errorf("expected warning emoji: %s", got)
	}
	if !strings.Contains(got, "remaining") {
		t.Errorf("expected remaining info: %s", got)
	}
}

func TestBudgetStatus_Exceeded(t *testing.T) {
	f := NewFormatter(10.0, 8.0)
	got := f.BudgetStatus(11.0)
	if !strings.Contains(got, "⛔") {
		t.Errorf("expected exceeded emoji: %s", got)
	}
	if !strings.Contains(got, "EXCEEDED") {
		t.Errorf("expected EXCEEDED: %s", got)
	}
}

func TestFormatSessionSummary(t *testing.T) {
	got := FormatSessionSummary(0.45, 15000, 5000, 3, []string{"main.go", "util.go"})

	if !strings.Contains(got, "Session Summary") {
		t.Error("expected header")
	}
	if !strings.Contains(got, "$0.450") {
		t.Error("expected cost")
	}
	if !strings.Contains(got, "3") {
		t.Error("expected iterations")
	}
	if !strings.Contains(got, "2 changed") {
		t.Error("expected files count")
	}
	if !strings.Contains(got, "main.go") {
		t.Error("expected file name")
	}
}

func TestFormatSessionSummary_NoFiles(t *testing.T) {
	got := FormatSessionSummary(0.10, 5000, 2000, 1, nil)
	if strings.Contains(got, "changed") {
		t.Error("should not show files section when empty")
	}
}
