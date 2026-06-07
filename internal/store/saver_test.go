package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ystsbry/revu/internal/model"
)

// copyDir recursively duplicates src into dst. It is intentionally minimal —
// only used to clone testdata into a temp dir so saver tests can mutate.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyDir(t, s, d)
			continue
		}
		b, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSaveStatusesRoundTrip(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "pr-1")
	copyDir(t, "testdata/pr-1", tmp)

	r, err := Load(tmp)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}

	// Flip statuses: c1 pending -> accepted, c4 rejected -> accepted.
	c1 := r.FindComment("c1")
	c4 := r.FindComment("c4")
	c1.Status = model.StatusAccepted
	c4.Status = model.StatusAccepted

	if err := SaveStatuses(r); err != nil {
		t.Fatalf("SaveStatuses: %v", err)
	}

	r2, err := Load(tmp)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := r2.FindComment("c1").Status; got != model.StatusAccepted {
		t.Errorf("c1 status after reload: %s", got)
	}
	if got := r2.FindComment("c4").Status; got != model.StatusAccepted {
		t.Errorf("c4 status after reload: %s", got)
	}
	// c2 and c3 should be untouched.
	if got := r2.FindComment("c2").Status; got != model.StatusPending {
		t.Errorf("c2 status drifted: %s", got)
	}
	if got := r2.FindComment("c3").Status; got != model.StatusAccepted {
		t.Errorf("c3 status drifted: %s", got)
	}
	// Body files must remain intact and re-loadable.
	for _, c := range r2.Comments {
		if c.Body == "" {
			t.Errorf("comment %s body lost after save", c.ID)
		}
	}
}

func TestSaveStatusesRequiresBaseDir(t *testing.T) {
	t.Parallel()
	r := &model.Review{SchemaVersion: 1}
	if err := SaveStatuses(r); err == nil {
		t.Fatalf("expected error without BaseDir")
	}
}

func TestSaveStatusesNilReview(t *testing.T) {
	t.Parallel()
	if err := SaveStatuses(nil); err == nil {
		t.Fatalf("expected error on nil")
	}
}

func TestSaveSessionIDAddsField(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "pr-1")
	copyDir(t, "testdata/pr-1", tmp)

	if err := SaveSessionID(tmp, "sess-abc-123"); err != nil {
		t.Fatalf("SaveSessionID: %v", err)
	}
	r, err := Load(tmp)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := r.GeneratedBy.SessionID; got != "sess-abc-123" {
		t.Fatalf("session_id = %q want %q", got, "sess-abc-123")
	}
	// Other generated_by fields must survive the patch.
	if r.GeneratedBy.Tool != "claude-code" || r.GeneratedBy.Skill != "review-pr" {
		t.Fatalf("generated_by clobbered: %+v", r.GeneratedBy)
	}
	// Comments must remain loadable and unchanged.
	if len(r.Comments) != 6 {
		t.Fatalf("comments count = %d want 6", len(r.Comments))
	}
}

func TestSaveSessionIDOverwrites(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "pr-1")
	copyDir(t, "testdata/pr-1", tmp)

	if err := SaveSessionID(tmp, "first"); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionID(tmp, "second"); err != nil {
		t.Fatal(err)
	}
	r, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.GeneratedBy.SessionID; got != "second" {
		t.Fatalf("session_id = %q want %q", got, "second")
	}
}

func TestSaveGeneratedByOverwritesTool(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "pr-1")
	copyDir(t, "testdata/pr-1", tmp)

	patch := GeneratedByPatch{
		Tool:      "codex",
		SessionID: "thread-019e9ff6",
	}
	if err := SaveGeneratedBy(tmp, patch); err != nil {
		t.Fatalf("SaveGeneratedBy: %v", err)
	}

	r, err := Load(tmp)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if r.GeneratedBy.Tool != "codex" {
		t.Errorf("tool = %q, want %q", r.GeneratedBy.Tool, "codex")
	}
	if r.GeneratedBy.SessionID != "thread-019e9ff6" {
		t.Errorf("session_id = %q, want %q", r.GeneratedBy.SessionID, "thread-019e9ff6")
	}
	// Fields we did not patch must survive.
	if r.GeneratedBy.Skill != "review-pr" || r.GeneratedBy.Model != "claude-opus-4-7" {
		t.Errorf("untouched fields clobbered: %+v", r.GeneratedBy)
	}
	// Comments must remain loadable and unchanged.
	if len(r.Comments) != 6 {
		t.Errorf("comments count = %d, want 6", len(r.Comments))
	}
}

func TestSaveGeneratedByEmptyPatchIsNoOp(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "pr-1")
	copyDir(t, "testdata/pr-1", tmp)
	before, err := os.ReadFile(filepath.Join(tmp, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveGeneratedBy(tmp, GeneratedByPatch{}); err != nil {
		t.Fatalf("SaveGeneratedBy empty: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(tmp, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("review.yml mutated by empty patch")
	}
}

func TestSaveSessionIDEmptyIsNoOp(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "pr-1")
	copyDir(t, "testdata/pr-1", tmp)
	before, err := os.ReadFile(filepath.Join(tmp, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionID(tmp, ""); err != nil {
		t.Fatalf("SaveSessionID empty: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(tmp, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("review.yml mutated by empty session id")
	}
}
