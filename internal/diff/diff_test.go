package diff

import (
	"strings"
	"testing"
)

func TestTracker_Empty(t *testing.T) {
	tr := NewTracker()
	if got := tr.Summary(); got != "No files changed." {
		t.Errorf("expected empty summary, got %q", got)
	}
	if len(tr.Changes()) != 0 {
		t.Errorf("expected 0 changes, got %d", len(tr.Changes()))
	}
	if len(tr.FilesChanged()) != 0 {
		t.Errorf("expected 0 files, got %d", len(tr.FilesChanged()))
	}
}

func TestTracker_RecordCreate(t *testing.T) {
	tr := NewTracker()
	tr.RecordCreate("main.go", "package main\n\nfunc main() {}\n")

	changes := tr.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Operation != OpCreate {
		t.Errorf("expected OpCreate, got %v", changes[0].Operation)
	}
	if changes[0].Path != "main.go" {
		t.Errorf("expected main.go, got %s", changes[0].Path)
	}
}

func TestTracker_RecordModify(t *testing.T) {
	tr := NewTracker()
	tr.Snapshot("main.go", "package main\n")
	tr.RecordModify("main.go", "package main\n\nimport \"fmt\"\n")

	changes := tr.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Operation != OpModify {
		t.Errorf("expected OpModify, got %v", changes[0].Operation)
	}
	if changes[0].Before != "package main\n" {
		t.Error("before content mismatch")
	}
}

func TestTracker_RecordDelete(t *testing.T) {
	tr := NewTracker()
	tr.Snapshot("old.go", "old content")
	tr.RecordDelete("old.go")

	changes := tr.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Operation != OpDelete {
		t.Errorf("expected OpDelete, got %v", changes[0].Operation)
	}
}

func TestTracker_Summary(t *testing.T) {
	tr := NewTracker()
	tr.RecordCreate("new.go", "package new\n")
	tr.Snapshot("edit.go", "old\n")
	tr.RecordModify("edit.go", "new\n")
	tr.Snapshot("del.go", "deleted\n")
	tr.RecordDelete("del.go")

	s := tr.Summary()
	if !strings.Contains(s, "3 file(s) changed") {
		t.Errorf("expected 3 files changed, got %q", s)
	}
	if !strings.Contains(s, "1 created") {
		t.Error("expected '1 created' in summary")
	}
	if !strings.Contains(s, "1 modified") {
		t.Error("expected '1 modified' in summary")
	}
	if !strings.Contains(s, "1 deleted") {
		t.Error("expected '1 deleted' in summary")
	}
}

func TestTracker_FilesChanged(t *testing.T) {
	tr := NewTracker()
	tr.RecordCreate("a.go", "")
	tr.RecordCreate("b.go", "")

	files := tr.FilesChanged()
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0] != "a.go" || files[1] != "b.go" {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestTracker_SnapshotIdempotent(t *testing.T) {
	tr := NewTracker()
	tr.Snapshot("x.go", "first")
	tr.Snapshot("x.go", "second") // should not overwrite

	tr.RecordModify("x.go", "third")
	if tr.Changes()[0].Before != "first" {
		t.Error("expected first snapshot to be preserved")
	}
}

func TestUnifiedDiff_Create(t *testing.T) {
	d := UnifiedDiff(Change{
		Path:      "new.go",
		Operation: OpCreate,
		After:     "package main\n",
	})
	if !strings.Contains(d, "+++ b/new.go") {
		t.Error("expected +++ b/new.go in diff")
	}
	if !strings.Contains(d, "+package main") {
		t.Error("expected +package main in diff")
	}
}

func TestUnifiedDiff_Delete(t *testing.T) {
	d := UnifiedDiff(Change{
		Path:      "old.go",
		Operation: OpDelete,
		Before:    "package old\n",
	})
	if !strings.Contains(d, "--- a/old.go") {
		t.Error("expected --- a/old.go in diff")
	}
	if !strings.Contains(d, "-package old") {
		t.Error("expected -package old in diff")
	}
}

func TestUnifiedDiff_Modify(t *testing.T) {
	d := UnifiedDiff(Change{
		Path:      "main.go",
		Operation: OpModify,
		Before:    "line1\nold\nline3\n",
		After:     "line1\nnew\nline3\n",
	})
	if !strings.Contains(d, "-old") {
		t.Error("expected -old in diff")
	}
	if !strings.Contains(d, "+new") {
		t.Error("expected +new in diff")
	}
	if !strings.Contains(d, " line1") {
		t.Error("expected unchanged line1")
	}
}

func TestOp_String(t *testing.T) {
	tests := []struct {
		op   Op
		want string
	}{
		{OpCreate, "created"},
		{OpModify, "modified"},
		{OpDelete, "deleted"},
		{Op(99), "unknown"},
	}
	for _, tc := range tests {
		if got := tc.op.String(); got != tc.want {
			t.Errorf("Op(%d).String() = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestCountDiffLines(t *testing.T) {
	adds, dels := countDiffLines("a\nb\nc\n", "a\nb\nd\ne\n")
	if adds < 1 {
		t.Errorf("expected at least 1 addition, got %d", adds)
	}
	if dels < 1 {
		t.Errorf("expected at least 1 deletion, got %d", dels)
	}
}
