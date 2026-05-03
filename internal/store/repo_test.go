package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileInRepo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "src", "a.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FileInRepo(root, "src/a.go")
	if err != nil {
		t.Fatalf("FileInRepo: %v", err)
	}
	if got != target {
		t.Errorf("got %q want %q", got, target)
	}

	if _, err := FileInRepo(root, "src/missing.go"); err == nil {
		t.Errorf("missing file should error")
	}
	if _, err := FileInRepo("", "anything"); err == nil {
		t.Errorf("empty root should error")
	}
}
