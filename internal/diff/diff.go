// Package diff provides unified diff generation and display for file changes.
// It tracks files modified during agent execution and generates human-readable
// diffs for the TUI output.
package diff

import (
	"fmt"
	"strings"
)

// Change represents a single file change.
type Change struct {
	Path      string
	Operation Op
	Before    string // content before edit (empty for new files)
	After     string // content after edit (empty for deletions)
}

// Op is the type of file operation.
type Op int

const (
	OpCreate Op = iota
	OpModify
	OpDelete
)

func (o Op) String() string {
	switch o {
	case OpCreate:
		return "created"
	case OpModify:
		return "modified"
	case OpDelete:
		return "deleted"
	default:
		return "unknown"
	}
}

// Tracker records all file changes during an agent session.
type Tracker struct {
	changes  []Change
	snapshots map[string]string // path → content before edit
}

// NewTracker creates a new change tracker.
func NewTracker() *Tracker {
	return &Tracker{
		snapshots: make(map[string]string),
	}
}

// Snapshot records the current content of a file before editing.
func (t *Tracker) Snapshot(path, content string) {
	if _, exists := t.snapshots[path]; !exists {
		t.snapshots[path] = content
	}
}

// RecordCreate records a new file creation.
func (t *Tracker) RecordCreate(path, content string) {
	t.changes = append(t.changes, Change{
		Path:      path,
		Operation: OpCreate,
		After:     content,
	})
}

// RecordModify records a file modification.
func (t *Tracker) RecordModify(path, after string) {
	before := t.snapshots[path]
	t.changes = append(t.changes, Change{
		Path:      path,
		Operation: OpModify,
		Before:    before,
		After:     after,
	})
}

// RecordDelete records a file deletion.
func (t *Tracker) RecordDelete(path string) {
	before := t.snapshots[path]
	t.changes = append(t.changes, Change{
		Path:      path,
		Operation: OpDelete,
		Before:    before,
	})
}

// Changes returns all recorded changes.
func (t *Tracker) Changes() []Change {
	return t.changes
}

// FilesChanged returns the list of changed file paths.
func (t *Tracker) FilesChanged() []string {
	paths := make([]string, len(t.changes))
	for i, c := range t.changes {
		paths[i] = c.Path
	}
	return paths
}

// Summary returns a human-readable summary of all changes.
func (t *Tracker) Summary() string {
	if len(t.changes) == 0 {
		return "No files changed."
	}

	var sb strings.Builder
	created, modified, deleted := 0, 0, 0

	for _, c := range t.changes {
		switch c.Operation {
		case OpCreate:
			created++
			sb.WriteString(fmt.Sprintf("  + %s (new)\n", c.Path))
		case OpModify:
			modified++
			adds, dels := countDiffLines(c.Before, c.After)
			sb.WriteString(fmt.Sprintf("  ~ %s (+%d -%d)\n", c.Path, adds, dels))
		case OpDelete:
			deleted++
			sb.WriteString(fmt.Sprintf("  - %s (deleted)\n", c.Path))
		}
	}

	header := fmt.Sprintf("%d file(s) changed", len(t.changes))
	parts := []string{}
	if created > 0 {
		parts = append(parts, fmt.Sprintf("%d created", created))
	}
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", modified))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", deleted))
	}

	return header + " (" + strings.Join(parts, ", ") + ")\n" + sb.String()
}

// UnifiedDiff generates a unified diff for a single change.
func UnifiedDiff(change Change) string {
	switch change.Operation {
	case OpCreate:
		return formatNewFile(change.Path, change.After)
	case OpDelete:
		return formatDeletedFile(change.Path, change.Before)
	case OpModify:
		return formatModifiedFile(change.Path, change.Before, change.After)
	default:
		return ""
	}
}

func formatNewFile(path, content string) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- /dev/null\n+++ b/%s\n", path))
	sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
	for _, line := range lines {
		sb.WriteString("+" + line + "\n")
	}
	return sb.String()
}

func formatDeletedFile(path, content string) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ /dev/null\n", path))
	sb.WriteString(fmt.Sprintf("@@ -1,%d +0,0 @@\n", len(lines)))
	for _, line := range lines {
		sb.WriteString("-" + line + "\n")
	}
	return sb.String()
}

func formatModifiedFile(path, before, after string) string {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", path, path))

	// Simple line-by-line diff (shows context around changes)
	i, j := 0, 0
	for i < len(beforeLines) || j < len(afterLines) {
		if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
			sb.WriteString(" " + beforeLines[i] + "\n")
			i++
			j++
		} else if i < len(beforeLines) {
			sb.WriteString("-" + beforeLines[i] + "\n")
			i++
		} else if j < len(afterLines) {
			sb.WriteString("+" + afterLines[j] + "\n")
			j++
		}
	}

	return sb.String()
}

// countDiffLines counts additions and deletions between two strings.
func countDiffLines(before, after string) (adds, dels int) {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	beforeSet := make(map[string]int)
	for _, line := range beforeLines {
		beforeSet[line]++
	}

	afterSet := make(map[string]int)
	for _, line := range afterLines {
		afterSet[line]++
	}

	for line, count := range afterSet {
		if bc, ok := beforeSet[line]; ok {
			if count > bc {
				adds += count - bc
			}
		} else {
			adds += count
		}
	}

	for line, count := range beforeSet {
		if ac, ok := afterSet[line]; ok {
			if count > ac {
				dels += count - ac
			}
		} else {
			dels += count
		}
	}

	return adds, dels
}
