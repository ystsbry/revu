package prune

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ystsbry/revu/internal/github"
)

// fakeQuerier returns canned PRState results keyed by PR number.
type fakeQuerier struct {
	states map[int]string
	errs   map[int]error
	calls  int
}

func (f *fakeQuerier) PRState(_ context.Context, _ string, n int) (string, error) {
	f.calls++
	if err, ok := f.errs[n]; ok {
		return "", err
	}
	return f.states[n], nil
}

// writeReview lays out repoDir/pr-N/sha/review.yml. submittedAt true means
// the file embeds a non-empty submitted_at scalar.
func writeReview(t *testing.T, repoDir string, pr int, sha string, submittedAt bool) {
	t.Helper()
	dir := filepath.Join(repoDir, fmt.Sprintf("pr-%d", pr), sha)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "schema_version: 1\n"
	if submittedAt {
		body += "submitted_at: 2026-05-08T10:00:00+09:00\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildClassifiesByState(t *testing.T) {
	repoDir := t.TempDir()
	writeReview(t, repoDir, 3, "aaaaaaa", true)  // MERGED → delete
	writeReview(t, repoDir, 7, "bbbbbbb", true)  // CLOSED → delete
	writeReview(t, repoDir, 12, "ccccccc", true) // OPEN   → keep

	q := &fakeQuerier{
		states: map[int]string{
			3:  github.PRStateMerged,
			7:  github.PRStateClosed,
			12: github.PRStateOpen,
		},
	}

	plan, err := Build(context.Background(), q, BuildOptions{Slug: "o/r", RepoDir: repoDir})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Delete); got != 2 {
		t.Fatalf("len(Delete)=%d want 2 (got %v)", got, plan.Delete)
	}
	if got := len(plan.Keep); got != 1 {
		t.Fatalf("len(Keep)=%d want 1 (got %v)", got, plan.Keep)
	}
	if len(plan.Errored) != 0 {
		t.Fatalf("expected no errors, got %v", plan.Errored)
	}
	// Delete order must match ascending PR number from ListPRNumbers.
	if plan.Delete[0].Number != 3 || plan.Delete[1].Number != 7 {
		t.Fatalf("Delete order = %v, want [3, 7]", []int{plan.Delete[0].Number, plan.Delete[1].Number})
	}
	if q.calls != 3 {
		t.Fatalf("PRState calls=%d want 3", q.calls)
	}
}

func TestBuildSurfacesUnsubmittedWarning(t *testing.T) {
	repoDir := t.TempDir()
	// pr-9: one SHA dir, no submitted_at.
	writeReview(t, repoDir, 9, "ddddddd", false)
	// pr-10: two SHA dirs, only the older one was submitted.
	writeReview(t, repoDir, 10, "eeeeeee", true)
	writeReview(t, repoDir, 10, "fffffff", false)

	q := &fakeQuerier{states: map[int]string{
		9:  github.PRStateClosed,
		10: github.PRStateMerged,
	}}
	plan, err := Build(context.Background(), q, BuildOptions{Slug: "o/r", RepoDir: repoDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Delete) != 2 {
		t.Fatalf("Delete len=%d want 2", len(plan.Delete))
	}
	for _, e := range plan.Delete {
		if !e.HasUnsubmitted {
			t.Errorf("pr-%d: HasUnsubmitted=false, want true", e.Number)
		}
	}
	// pr-10 must report 2 SHA dirs.
	for _, e := range plan.Delete {
		if e.Number == 10 && e.SHADirCount != 2 {
			t.Errorf("pr-10 SHADirCount=%d want 2", e.SHADirCount)
		}
	}
}

func TestBuildErroredQueriesNotDeleted(t *testing.T) {
	repoDir := t.TempDir()
	writeReview(t, repoDir, 5, "ggggggg", true)

	wantErr := errors.New("gh pr view: not found")
	q := &fakeQuerier{errs: map[int]error{5: wantErr}}

	plan, err := Build(context.Background(), q, BuildOptions{Slug: "o/r", RepoDir: repoDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Delete) != 0 {
		t.Fatalf("Delete should be empty when the only PR errored, got %v", plan.Delete)
	}
	if len(plan.Errored) != 1 {
		t.Fatalf("Errored len=%d want 1", len(plan.Errored))
	}
	if !errors.Is(plan.Errored[0].QueryErr, wantErr) {
		t.Errorf("QueryErr=%v, want wraps %v", plan.Errored[0].QueryErr, wantErr)
	}
}

func TestBuildOnMissingRepoDir(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "does-not-exist")
	plan, err := Build(context.Background(), &fakeQuerier{}, BuildOptions{Slug: "o/r", RepoDir: repoDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Delete)+len(plan.Keep)+len(plan.Errored) != 0 {
		t.Fatalf("missing repoDir should yield empty plan, got %+v", plan)
	}
}

func TestBuildIgnoresUnreviewedPRDir(t *testing.T) {
	// pr-N exists but has no SHA subdir / review.yml. PRState is still
	// queried (since the dir might be a half-finished generation), and
	// CLOSED/MERGED still leads to deletion (clearing the stale shell).
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "pr-1"), 0o755); err != nil {
		t.Fatal(err)
	}

	q := &fakeQuerier{states: map[int]string{1: github.PRStateMerged}}
	plan, err := Build(context.Background(), q, BuildOptions{Slug: "o/r", RepoDir: repoDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Delete) != 1 {
		t.Fatalf("len(Delete)=%d want 1", len(plan.Delete))
	}
	if plan.Delete[0].SHADirCount != 0 {
		t.Errorf("SHADirCount=%d want 0", plan.Delete[0].SHADirCount)
	}
	if plan.Delete[0].HasUnsubmitted {
		t.Errorf("HasUnsubmitted=true on a no-review dir; want false")
	}
}

func TestExecuteRemovesOnlyDeleteEntries(t *testing.T) {
	repoDir := t.TempDir()
	writeReview(t, repoDir, 3, "aaaaaaa", true) // delete
	writeReview(t, repoDir, 7, "bbbbbbb", true) // delete
	writeReview(t, repoDir, 12, "ccccccc", true) // keep

	plan := &Plan{
		RepoDir: repoDir,
		Slug:    "o/r",
		Delete: []Entry{
			{Number: 3, Path: filepath.Join(repoDir, "pr-3")},
			{Number: 7, Path: filepath.Join(repoDir, "pr-7")},
		},
		Keep: []Entry{{Number: 12, Path: filepath.Join(repoDir, "pr-12")}},
	}
	if err := Execute(plan); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{3, 7} {
		if _, err := os.Stat(filepath.Join(repoDir, fmt.Sprintf("pr-%d", n))); !os.IsNotExist(err) {
			t.Errorf("pr-%d should be gone, stat err=%v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repoDir, "pr-12")); err != nil {
		t.Errorf("pr-12 should remain, stat err=%v", err)
	}
}

func TestExecuteNilPlanIsNoOp(t *testing.T) {
	if err := Execute(nil); err != nil {
		t.Fatalf("nil plan: %v", err)
	}
	if err := Execute(&Plan{}); err != nil {
		t.Fatalf("empty plan: %v", err)
	}
}
