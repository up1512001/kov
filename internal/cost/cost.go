// Package cost provides cost tracking and formatting utilities for kov sessions.
package cost

import (
	"fmt"
	"strings"
)

// Formatter helps display cost information in human-readable format.
type Formatter struct {
	budget float64 // max budget (0 = unlimited)
	warnAt float64 // warn threshold (e.g., 5.00)
}

// NewFormatter creates a cost formatter with budget limits.
func NewFormatter(budget, warnAt float64) *Formatter {
	return &Formatter{
		budget: budget,
		warnAt: warnAt,
	}
}

// FormatCost returns a human-readable cost string.
func FormatCost(cost float64) string {
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	if cost < 1.0 {
		return fmt.Sprintf("$%.3f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// FormatTokens returns a human-readable token count.
func FormatTokens(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1_000_000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
}

// FormatUsage returns a formatted usage line like "$0.23 (12.4K in / 3.2K out)"
func FormatUsage(cost float64, inputTokens, outputTokens int) string {
	return fmt.Sprintf("%s (%s in / %s out)",
		FormatCost(cost),
		FormatTokens(inputTokens),
		FormatTokens(outputTokens),
	)
}

// BudgetStatus returns the budget status indicator.
func (f *Formatter) BudgetStatus(currentCost float64) string {
	if f.budget <= 0 {
		return FormatCost(currentCost)
	}

	pct := (currentCost / f.budget) * 100
	remaining := f.budget - currentCost
	status := FormatCost(currentCost)

	if currentCost >= f.budget {
		return fmt.Sprintf("%s ⛔ BUDGET EXCEEDED (limit: %s)", status, FormatCost(f.budget))
	}

	if currentCost >= f.warnAt {
		return fmt.Sprintf("%s ⚠️ approaching limit (%s remaining)", status, FormatCost(remaining))
	}

	return fmt.Sprintf("%s (%.0f%% of %s budget)", status, pct, FormatCost(f.budget))
}

// FormatSessionSummary produces a multi-line session cost summary.
func FormatSessionSummary(cost float64, inputTokens, outputTokens int, iterations int, filesEdited []string) string {
	var sb strings.Builder

	sb.WriteString("─── Session Summary ───\n")
	sb.WriteString(fmt.Sprintf("  Cost:       %s\n", FormatUsage(cost, inputTokens, outputTokens)))
	sb.WriteString(fmt.Sprintf("  Iterations: %d\n", iterations))

	if len(filesEdited) > 0 {
		sb.WriteString(fmt.Sprintf("  Files:      %d changed\n", len(filesEdited)))
		for _, f := range filesEdited {
			sb.WriteString(fmt.Sprintf("    • %s\n", f))
		}
	}

	return sb.String()
}
