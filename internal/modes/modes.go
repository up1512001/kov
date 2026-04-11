// Package modes resolves per-mode configuration overrides.
// Each mode can have its own model, thinking budget, and tool restrictions.
package modes

import (
	"github.com/up1512001/kov/internal/config"
)

// Resolved holds the resolved configuration for a specific mode.
type Resolved struct {
	Mode        string
	Model       string
	ThinkBudget int
	ReadOnly    bool
	ToolFilter  []string // empty = all tools
}

// Resolve applies mode-specific overrides from the config.
func Resolve(mode, defaultModel string, cfg *config.Config) Resolved {
	r := Resolved{
		Mode:  mode,
		Model: defaultModel,
	}

	switch mode {
	case "plan":
		r.ReadOnly = true
		r.ToolFilter = []string{"file_read", "grep_search", "glob_search", "list_dir"}
		if cfg.Modes.Plan != nil && cfg.Modes.Plan.Model != "" {
			r.Model = cfg.Modes.Plan.Model
		}

	case "think":
		r.ThinkBudget = 20000 // high default for think mode
		if cfg.Modes.Think != nil {
			if cfg.Modes.Think.Model != "" {
				r.Model = cfg.Modes.Think.Model
			}
			r.ThinkBudget = parseBudget(cfg.Modes.Think.ThinkBudget, 20000)
		}

	case "ask":
		r.ReadOnly = true
		r.ToolFilter = []string{"file_read", "grep_search", "glob_search", "list_dir"}
		if cfg.Modes.Ask != nil && cfg.Modes.Ask.Model != "" {
			r.Model = cfg.Modes.Ask.Model
		}

	case "fast":
		r.ThinkBudget = 0 // no thinking
		if cfg.Modes.Fast != nil && cfg.Modes.Fast.Model != "" {
			r.Model = cfg.Modes.Fast.Model
		}

	case "research":
		r.ThinkBudget = 10000
		if cfg.Modes.Research != nil && cfg.Modes.Research.Model != "" {
			r.Model = cfg.Modes.Research.Model
		}

	case "review":
		r.ReadOnly = true
		r.ToolFilter = []string{"file_read", "grep_search", "glob_search", "list_dir", "shell_exec"}
		if cfg.Modes.Review != nil && cfg.Modes.Review.Model != "" {
			r.Model = cfg.Modes.Review.Model
		}

	case "architect":
		// Architect uses two models — handled separately by the ArchitectRunner
		r.ThinkBudget = 30000
		// Default model is used for the planning phase

	case "code":
		if cfg.Modes.Code != nil && cfg.Modes.Code.Model != "" {
			r.Model = cfg.Modes.Code.Model
		}
	}

	return r
}

// ArchitectModels returns the plan and edit models for architect mode.
func ArchitectModels(defaultModel string, cfg *config.Config) (planModel, editModel string) {
	planModel = defaultModel
	editModel = defaultModel

	if cfg.Modes.Architect != nil {
		if cfg.Modes.Architect.PlanModel != "" {
			planModel = cfg.Modes.Architect.PlanModel
		}
		if cfg.Modes.Architect.EditModel != "" {
			editModel = cfg.Modes.Architect.EditModel
		}
	}

	return planModel, editModel
}

// parseBudget maps a budget string to a token count.
func parseBudget(budget string, defaultVal int) int {
	switch budget {
	case "low":
		return 5000
	case "medium":
		return 10000
	case "high":
		return 20000
	case "maximum":
		return 50000
	case "":
		return defaultVal
	default:
		return defaultVal
	}
}
