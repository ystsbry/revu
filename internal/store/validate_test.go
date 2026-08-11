package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/model"
)

// validReview is the smallest review.yml Load accepts, as a template the
// tests below break one field at a time.
const validReview = `schema_version: 1
pr:
  repo: o/r
  number: 1
  head_sha: abc1234
review_event: COMMENT
summary_file: summary.md
comments:
  - id: c1
    status: pending
    severity: major
    category: bug
    path: x.go
    line: 10
    side: RIGHT
    body_file: c1.md
`

// writeReview lays out a review directory whose review.yml is body.
func writeReview(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"review.yml": body,
		"summary.md": "summary\n",
		"c1.md":      "comment body\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadAcceptsAValidReview(t *testing.T) {
	t.Parallel()
	r, err := Load(writeReview(t, validReview))
	if err != nil {
		t.Fatal(err)
	}
	if r.PR.Repo != "o/r" || len(r.Comments) != 1 {
		t.Fatalf("review = %+v, want the fixture loaded", r)
	}
	if r.SummaryBody != "summary\n" || r.Comments[0].Body != "comment body\n" {
		t.Fatal("body files should be read into the review")
	}
}

// Each validation exists to stop a review that would post nonsense to
// GitHub, so each rejection is pinned with the field it names.
func TestLoadRejectsInvalidReviews(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mutate   func(string) string
		wantWord string
	}{
		{
			name:     "no pr.repo",
			mutate:   func(s string) string { return strings.Replace(s, "  repo: o/r\n", "", 1) },
			wantWord: "pr.repo",
		},
		{
			name:     "pr.number is not positive",
			mutate:   func(s string) string { return strings.Replace(s, "number: 1", "number: 0", 1) },
			wantWord: "pr.number",
		},
		{
			name:     "comment has no id",
			mutate:   func(s string) string { return strings.Replace(s, "id: c1", `id: ""`, 1) },
			wantWord: "id is required",
		},
		{
			name:     "unknown status",
			mutate:   func(s string) string { return strings.Replace(s, "status: pending", "status: maybe", 1) },
			wantWord: "status",
		},
		{
			name:     "unknown severity",
			mutate:   func(s string) string { return strings.Replace(s, "severity: major", "severity: huge", 1) },
			wantWord: "severity",
		},
		{
			name:     "unknown category",
			mutate:   func(s string) string { return strings.Replace(s, "category: bug", "category: vibes", 1) },
			wantWord: "category",
		},
		{
			name:     "unknown side",
			mutate:   func(s string) string { return strings.Replace(s, "side: RIGHT", "side: MIDDLE", 1) },
			wantWord: "side",
		},
		{
			name:     "no path",
			mutate:   func(s string) string { return strings.Replace(s, "    path: x.go\n", "", 1) },
			wantWord: "path is required",
		},
		{
			name:     "line is not positive",
			mutate:   func(s string) string { return strings.Replace(s, "line: 10", "line: 0", 1) },
			wantWord: "line must be positive",
		},
		{
			name:     "no body_file",
			mutate:   func(s string) string { return strings.Replace(s, "    body_file: c1.md\n", "", 1) },
			wantWord: "body_file",
		},
		{
			// A range needs its start line; a side alone is meaningless.
			name:     "start_side without start_line",
			mutate:   func(s string) string { return s + "    start_side: LEFT\n" },
			wantWord: "start_side requires start_line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(writeReview(t, tt.mutate(validReview)))
			if err == nil {
				t.Fatalf("%s should be rejected", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantWord) {
				t.Fatalf("error = %q, want it to mention %q", err, tt.wantWord)
			}
		})
	}
}

func TestResolveReviewDir(t *testing.T) {
	t.Parallel()

	t.Run("a directory holding review.yml is used as-is", func(t *testing.T) {
		t.Parallel()
		dir := writeReview(t, validReview)

		got, err := ResolveReviewDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != dir {
			t.Fatalf("ResolveReviewDir = %q, want %q", got, dir)
		}
	})

	// Callers that know only the PR number pass pr-N/ and get the newest
	// SHA subdir back — the non-interactive workflow depends on this.
	t.Run("a pr-N parent resolves to its newest SHA subdir", func(t *testing.T) {
		t.Parallel()
		prDir := t.TempDir()
		shaDir := filepath.Join(prDir, "abc1234")
		if err := os.MkdirAll(shaDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(shaDir, "review.yml"), []byte(validReview), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := ResolveReviewDir(prDir)
		if err != nil {
			t.Fatal(err)
		}
		if got != shaDir {
			t.Fatalf("ResolveReviewDir = %q, want the SHA subdir %q", got, shaDir)
		}
	})

	// With nothing to resolve, the path comes back unchanged so the
	// caller's loader is the one that reports the missing review.yml.
	t.Run("an empty directory is returned unchanged", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		got, err := ResolveReviewDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != dir {
			t.Fatalf("ResolveReviewDir = %q, want %q", got, dir)
		}
	})

	t.Run("a missing path is an error", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveReviewDir(filepath.Join(t.TempDir(), "nope"))
		if err == nil || !strings.Contains(err.Error(), "review dir") {
			t.Fatalf("error = %v, want a missing-dir complaint", err)
		}
	})

	t.Run("a file is not a review directory", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "review.yml")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := ResolveReviewDir(path)
		if err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("error = %v, want a not-a-directory complaint", err)
		}
	})
}

// SaveSessionID records which agent session produced a review so it can be
// resumed later.
func TestSaveSessionID(t *testing.T) {
	t.Parallel()
	dir := writeReview(t, validReview)

	if err := SaveSessionID(dir, "sess-123"); err != nil {
		t.Fatal(err)
	}
	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.GeneratedBy.SessionID != "sess-123" {
		t.Fatalf("generated_by = %+v, want the session id recorded", r.GeneratedBy)
	}
}

func TestSaveSessionIDOnAMissingReview(t *testing.T) {
	t.Parallel()
	if err := SaveSessionID(t.TempDir(), "sess-123"); err == nil {
		t.Fatal("saving into a directory with no review.yml should fail")
	}
}

func TestSaveGeneratedBy(t *testing.T) {
	t.Parallel()
	dir := writeReview(t, validReview)

	err := SaveGeneratedBy(dir, GeneratedByPatch{
		Tool:  "claude-code",
		Model: "claude-opus-5",
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.GeneratedBy.Tool != "claude-code" || r.GeneratedBy.Model != "claude-opus-5" {
		t.Fatalf("generated_by = %+v, want the patch applied", r.GeneratedBy)
	}
}

// Saving statuses must not disturb anything else in the file — the YAML is
// hand-editable and users have comments in it.
func TestSaveStatusesKeepsTheRestOfTheFile(t *testing.T) {
	t.Parallel()
	dir := writeReview(t, validReview)

	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r.Comments[0].Status = model.StatusAccepted
	if err := SaveStatuses(r); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: accepted") {
		t.Fatalf("review.yml should carry the new status:\n%s", raw)
	}
	for _, want := range []string{"summary_file: summary.md", "body_file: c1.md", "head_sha: abc1234"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("review.yml lost %q:\n%s", want, raw)
		}
	}
}

// A patch with nothing in it must not rewrite the file at all — callers
// pass a partially-filled patch on every run.
func TestSaveGeneratedByEmptyPatchIsANoOp(t *testing.T) {
	t.Parallel()
	dir := writeReview(t, validReview)
	path := filepath.Join(dir, "review.yml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := SaveGeneratedBy(dir, GeneratedByPatch{}); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("review.yml was rewritten:\n%s", after)
	}
}

func TestSaveGeneratedByRequiresAReviewDir(t *testing.T) {
	t.Parallel()
	if err := SaveGeneratedBy("", GeneratedByPatch{Tool: "claude-code"}); err == nil {
		t.Fatal("an empty reviewDir should fail")
	}
}

// Patching an existing generated_by must overwrite the named fields and
// leave the others alone.
func TestSaveGeneratedByPatchesOnlyTheGivenFields(t *testing.T) {
	t.Parallel()
	dir := writeReview(t, validReview+`generated_by:
  tool: old-tool
  skill: revu:pr
`)

	if err := SaveGeneratedBy(dir, GeneratedByPatch{Tool: "new-tool", Model: "m"}); err != nil {
		t.Fatal(err)
	}

	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.GeneratedBy.Tool != "new-tool" {
		t.Errorf("tool = %q, want it overwritten", r.GeneratedBy.Tool)
	}
	if r.GeneratedBy.Model != "m" {
		t.Errorf("model = %q, want it added", r.GeneratedBy.Model)
	}
	if r.GeneratedBy.Skill != "revu:pr" {
		t.Errorf("skill = %q, want the untouched field kept", r.GeneratedBy.Skill)
	}
}

// Malformed YAML has to be reported rather than silently replaced.
func TestSaveGeneratedByOnBrokenYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte("\tnot: yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SaveGeneratedBy(dir, GeneratedByPatch{Tool: "x"}); err == nil {
		t.Fatal("broken YAML should fail")
	}
}

// A review.yml whose root is a list (not a mapping) is not something the
// patcher can edit.
func TestSaveGeneratedByOnANonMappingRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte("- a\n- b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := SaveGeneratedBy(dir, GeneratedByPatch{Tool: "x"})
	if err == nil || !strings.Contains(err.Error(), "not a mapping") {
		t.Fatalf("error = %v, want a not-a-mapping complaint", err)
	}
}

func TestLoadErrorMessageShapes(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")

	tests := []struct {
		name string
		err  *LoadError
		want string
	}{
		{name: "path and field", err: &LoadError{Path: "review.yml", Field: "line", Cause: cause}, want: "review.yml: line: boom"},
		{name: "path only", err: &LoadError{Path: "review.yml", Cause: cause}, want: "review.yml: boom"},
		{name: "neither", err: &LoadError{Cause: cause}, want: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
