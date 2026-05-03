package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/model"
)

func TestLoadFixture(t *testing.T) {
	t.Parallel()
	r, err := Load("testdata/pr-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if r.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", r.SchemaVersion)
	}
	if r.PR.Repo != "ystsbry/revu" || r.PR.Number != 1 {
		t.Errorf("PR meta: %#v", r.PR)
	}
	if r.ReviewEvent != model.EventRequestChanges {
		t.Errorf("ReviewEvent = %q", r.ReviewEvent)
	}
	if !strings.Contains(r.SummaryBody, "全体所感") {
		t.Errorf("SummaryBody not loaded; first 80 chars: %q", trunc(r.SummaryBody, 80))
	}
	if len(r.Comments) != 6 {
		t.Fatalf("len(Comments) = %d, want 6", len(r.Comments))
	}
	for i, c := range r.Comments {
		if c.Body == "" {
			t.Errorf("comment[%d] (%s) body not loaded", i, c.ID)
		}
	}
	if r.BaseDir == "" || !filepath.IsAbs(r.BaseDir) {
		t.Errorf("BaseDir = %q, want abs path", r.BaseDir)
	}

	if got := r.Counts(); got[model.StatusPending] != 4 || got[model.StatusAccepted] != 1 || got[model.StatusRejected] != 1 {
		t.Errorf("Counts mismatch: %#v", got)
	}

	// c5 is a same-side RIGHT range comment.
	c5 := r.FindComment("c5")
	if c5 == nil {
		t.Fatalf("c5 not found")
	}
	if c5.StartLine == nil || *c5.StartLine != 197 || c5.Line != 215 {
		t.Errorf("c5 range = (start=%v, line=%d), want (197, 215)", c5.StartLine, c5.Line)
	}
	if c5.Side != model.SideRight || c5.StartSide != nil {
		t.Errorf("c5 should be same-side RIGHT (no explicit start_side); got side=%q start_side=%v", c5.Side, c5.StartSide)
	}

	// c6 is a cross-side range: LEFT 130 -> RIGHT 142.
	c6 := r.FindComment("c6")
	if c6 == nil {
		t.Fatalf("c6 not found")
	}
	if c6.StartLine == nil || *c6.StartLine != 130 || c6.Line != 142 {
		t.Errorf("c6 range = (start=%v, line=%d), want (130, 142)", c6.StartLine, c6.Line)
	}
	if c6.StartSide == nil || *c6.StartSide != model.SideLeft || c6.Side != model.SideRight {
		t.Errorf("c6 cross-side = (start_side=%v, side=%q), want (LEFT, RIGHT)", c6.StartSide, c6.Side)
	}
}

func TestLoadMissingDir(t *testing.T) {
	t.Parallel()
	_, err := Load("testdata/does-not-exist")
	if err == nil {
		t.Fatalf("expected error")
	}
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LoadError, got %T: %v", err, err)
	}
}

func TestLoadSchemaVersionMismatch(t *testing.T) {
	t.Parallel()
	dir := writeFixture(t, map[string]string{
		"review.yml": `schema_version: 99
pr:
  repo: o/r
  number: 1
  head_sha: abc
  base_branch: main
generated_at: 2026-05-03T14:30:00+09:00
generated_by: {tool: claude-code, skill: review-pr, model: x}
review_event: COMMENT
summary_file: summary.md
comments: []
`,
		"summary.md": "ok",
	})
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("expected schema_version error, got %v", err)
	}
}

func TestLoadMissingSummaryFile(t *testing.T) {
	t.Parallel()
	dir := writeFixture(t, map[string]string{
		"review.yml": `schema_version: 1
pr: {repo: o/r, number: 1, head_sha: a, base_branch: main}
generated_at: 2026-05-03T14:30:00+09:00
generated_by: {tool: x, skill: y, model: z}
review_event: COMMENT
summary_file: summary.md
comments: []
`,
	})
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "summary.md") {
		t.Fatalf("expected summary file error, got %v", err)
	}
}

func TestLoadMissingCommentBody(t *testing.T) {
	t.Parallel()
	dir := writeFixture(t, map[string]string{
		"review.yml": `schema_version: 1
pr: {repo: o/r, number: 1, head_sha: a, base_branch: main}
generated_at: 2026-05-03T14:30:00+09:00
generated_by: {tool: x, skill: y, model: z}
review_event: COMMENT
summary_file: summary.md
comments:
  - id: c1
    status: pending
    severity: minor
    category: bug
    path: a.go
    line: 10
    side: RIGHT
    body_file: comments/c1.md
`,
		"summary.md": "ok",
	})
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "c1.md") {
		t.Fatalf("expected comment body error, got %v", err)
	}
}

func TestLoadDuplicateIDs(t *testing.T) {
	t.Parallel()
	dir := writeFixture(t, map[string]string{
		"review.yml": `schema_version: 1
pr: {repo: o/r, number: 1, head_sha: a, base_branch: main}
generated_at: 2026-05-03T14:30:00+09:00
generated_by: {tool: x, skill: y, model: z}
review_event: COMMENT
summary_file: summary.md
comments:
  - {id: c1, status: pending, severity: minor, category: bug, path: a.go, line: 1, side: RIGHT, body_file: a.md}
  - {id: c1, status: pending, severity: minor, category: bug, path: b.go, line: 1, side: RIGHT, body_file: b.md}
`,
		"summary.md": "ok",
		"a.md":       "a",
		"b.md":       "b",
	})
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestLoadInvalidEnum(t *testing.T) {
	t.Parallel()
	dir := writeFixture(t, map[string]string{
		"review.yml": `schema_version: 1
pr: {repo: o/r, number: 1, head_sha: a, base_branch: main}
generated_at: 2026-05-03T14:30:00+09:00
generated_by: {tool: x, skill: y, model: z}
review_event: COMMENT
summary_file: summary.md
comments:
  - {id: c1, status: zzz, severity: minor, category: bug, path: a.go, line: 1, side: RIGHT, body_file: a.md}
`,
		"summary.md": "ok",
		"a.md":       "a",
	})
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "invalid status") {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func TestValidateCommentRange(t *testing.T) {
	t.Parallel()
	base := func() model.Comment {
		return model.Comment{
			ID: "c1", Status: model.StatusPending,
			Severity: model.SeverityMinor, Category: model.CategoryDesign,
			Path: "a.go", Line: 10, Side: model.SideRight, BodyFile: "b.md",
		}
	}

	t.Run("same-side range valid", func(t *testing.T) {
		c := base()
		s := 5
		c.StartLine = &s
		if err := validateComment(&c); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("same-side range with start>line rejected", func(t *testing.T) {
		c := base()
		s := 11
		c.StartLine = &s
		if err := validateComment(&c); err == nil {
			t.Error("want error for start_line > line")
		}
	})
	t.Run("cross-side range allows start_line>line", func(t *testing.T) {
		c := base()
		s := 99
		left := model.SideLeft
		c.StartLine = &s
		c.StartSide = &left
		if err := validateComment(&c); err != nil {
			t.Errorf("cross-side should allow any line ordering: %v", err)
		}
	})
	t.Run("start_side without start_line rejected", func(t *testing.T) {
		c := base()
		left := model.SideLeft
		c.StartSide = &left
		if err := validateComment(&c); err == nil {
			t.Error("want error for start_side without start_line")
		}
	})
	t.Run("invalid start_side rejected", func(t *testing.T) {
		c := base()
		s := 5
		bad := model.Side("UP")
		c.StartLine = &s
		c.StartSide = &bad
		if err := validateComment(&c); err == nil {
			t.Error("want error for invalid start_side")
		}
	})
	t.Run("non-positive start_line rejected", func(t *testing.T) {
		c := base()
		s := 0
		c.StartLine = &s
		if err := validateComment(&c); err == nil {
			t.Error("want error for start_line <= 0")
		}
	})
}
