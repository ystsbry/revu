package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/model"
)

func TestEditorCmdHonorsEDITOR(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")
	c := editorCmd("/tmp/x.md", "")
	if !strings.HasSuffix(c.Path, "code") && c.Args[0] != "code" {
		t.Errorf("editorCmd path = %q args = %v", c.Path, c.Args)
	}
	if c.Args[len(c.Args)-1] != "/tmp/x.md" {
		t.Errorf("path arg missing: %v", c.Args)
	}
}

func TestEditorCmdFallsBackToVi(t *testing.T) {
	t.Setenv("EDITOR", "")
	c := editorCmd("/tmp/x.md", "")
	if !strings.HasSuffix(c.Args[0], "vi") {
		t.Errorf("expected vi fallback, got %q", c.Args[0])
	}
}

func TestReloadBodyForSummary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(summary, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &model.Review{
		BaseDir:     dir,
		SummaryFile: "summary.md",
		SummaryBody: "initial",
	}
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	// Simulate external edit.
	if err := os.WriteFile(summary, []byte("updated body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.reloadBody(summary); err != nil {
		t.Fatalf("reloadBody: %v", err)
	}
	if r.SummaryBody != "updated body" {
		t.Errorf("SummaryBody = %q", r.SummaryBody)
	}
}

func TestReloadBodyForComment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cPath := filepath.Join(dir, "comments", "c1.md")
	if err := os.MkdirAll(filepath.Dir(cPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cPath, []byte("body v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &model.Review{
		BaseDir: dir,
		Comments: []model.Comment{
			{ID: "c1", BodyFile: "comments/c1.md", Body: "body v1"},
		},
	}
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	if err := os.WriteFile(cPath, []byte("body v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.reloadBody(cPath); err != nil {
		t.Fatalf("reloadBody: %v", err)
	}
	if r.Comments[0].Body != "body v2" {
		t.Errorf("c1 body = %q", r.Comments[0].Body)
	}
}

func TestReloadBodyUnknownPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := &model.Review{BaseDir: dir, SummaryFile: "summary.md"}
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	other := filepath.Join(dir, "other.md")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.reloadBody(other); err == nil {
		t.Errorf("expected error for unknown path")
	}
}
