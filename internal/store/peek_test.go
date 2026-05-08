package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPeekSubmittedAt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// No review.yml at all → false, no error.
	got, err := PeekSubmittedAt(dir)
	if err != nil {
		t.Fatalf("missing review.yml: unexpected err %v", err)
	}
	if got {
		t.Errorf("missing review.yml: got true, want false")
	}

	// review.yml without submitted_at.
	must := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("schema_version: 1\n")
	got, err = PeekSubmittedAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Errorf("no submitted_at: got true, want false")
	}

	// review.yml with submitted_at populated.
	must("schema_version: 1\nsubmitted_at: 2026-05-08T10:00:00+09:00\n")
	got, err = PeekSubmittedAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Errorf("submitted_at present: got false, want true")
	}

	// Empty reviewDir is rejected.
	if _, err := PeekSubmittedAt(""); err == nil {
		t.Fatalf("empty reviewDir should error")
	}

	// Malformed YAML surfaces an error.
	must("not: valid: yaml: ::\n")
	if _, err := PeekSubmittedAt(dir); err == nil {
		t.Fatalf("malformed yaml should error")
	}
}

func TestListPRNumbers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, name := range []string{"pr-3", "pr-12", "pr-7", "pr-not-a-number", "notes", "pr-0"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListPRNumbers(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{3, 7, 12}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, n := range want {
		if got[i] != n {
			t.Fatalf("got[%d]=%d want %d (full: %v)", i, got[i], n, got)
		}
	}
}
