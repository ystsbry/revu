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
	if r.PR.Repo != "ystsbry/some-app" || r.PR.Number != 1 {
		t.Errorf("PR meta: %#v", r.PR)
	}
	if r.ReviewEvent != model.EventRequestChanges {
		t.Errorf("ReviewEvent = %q", r.ReviewEvent)
	}
	if !strings.Contains(r.SummaryBody, "全体所感") {
		t.Errorf("SummaryBody not loaded; first 80 chars: %q", trunc(r.SummaryBody, 80))
	}
	if len(r.Comments) != 4 {
		t.Fatalf("len(Comments) = %d, want 4", len(r.Comments))
	}
	for i, c := range r.Comments {
		if c.Body == "" {
			t.Errorf("comment[%d] (%s) body not loaded", i, c.ID)
		}
	}
	if r.BaseDir == "" || !filepath.IsAbs(r.BaseDir) {
		t.Errorf("BaseDir = %q, want abs path", r.BaseDir)
	}

	if got := r.Counts(); got[model.StatusPending] != 2 || got[model.StatusAccepted] != 1 || got[model.StatusRejected] != 1 {
		t.Errorf("Counts mismatch: %#v", got)
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
