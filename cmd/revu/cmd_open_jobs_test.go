package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ystsbry/revu/internal/jobs"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/store"
)

// resolveRepoRoot decides which checkout the TUI renders code from. Every
// branch here is a way the user can end up pointed at the wrong tree, so
// each one is pinned. (Launching the TUI itself stays out of scope.)
func TestResolveRepoRootWithAnExplicitOverride(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()

	got, err := resolveRepoRoot(dir, "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("resolveRepoRoot = %q, want %q", got, dir)
	}
}

// An override that does not exist has to fail loudly: silently falling
// back would render the review against some unrelated tree.
func TestResolveRepoRootRejectsABadOverride(t *testing.T) {
	isolateEnv(t)

	if _, err := resolveRepoRoot(filepath.Join(t.TempDir(), "nope"), "o/r"); err == nil {
		t.Fatal("a missing --repo-root should fail")
	}

	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := resolveRepoRoot(file, "o/r")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %v, want a not-a-directory complaint", err)
	}
}

// Without an override, cwd is checked against the review's repo.
func TestResolveRepoRootVerifiesTheCurrentClone(t *testing.T) {
	isolateEnv(t)
	clone := filepath.Join(t.TempDir(), "clone")
	gitClone(t, clone, "https://github.com/o/r.git")
	chdirTo(t, clone)

	got, err := resolveRepoRoot("", "o/r")
	if err != nil {
		t.Fatal(err)
	}
	gotReal, _ := filepath.EvalSymlinks(got)
	wantReal, _ := filepath.EvalSymlinks(clone)
	if gotReal != wantReal {
		t.Fatalf("resolveRepoRoot = %q, want the cwd clone %q", gotReal, wantReal)
	}
}

// Outside the matching repo, a registered clone is used — this is what
// makes `revu open` work from anywhere once the repo is registered.
func TestResolveRepoRootFallsBackToTheRegisteredClone(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	clone := t.TempDir()
	writeConfigFile(t, "[[repo]]\nslug = \"o/r\"\npath = \""+clone+"\"\n")

	got, err := resolveRepoRoot("", "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if got != clone {
		t.Fatalf("resolveRepoRoot = %q, want the registered clone %q", got, clone)
	}
}

// A registered path that has since been deleted must say so rather than
// falling through to the generic "run inside the repo" message.
func TestResolveRepoRootReportsAMissingRegisteredClone(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	gone := filepath.Join(t.TempDir(), "removed")
	writeConfigFile(t, "[[repo]]\nslug = \"o/r\"\npath = \""+gone+"\"\n")

	_, err := resolveRepoRoot("", "o/r")
	if err == nil {
		t.Fatal("a missing registered clone should fail")
	}
	if !strings.Contains(err.Error(), gone) {
		t.Fatalf("error = %q, want the stale path named", err)
	}
}

func TestResolveRepoRootWithNothingToGoOn(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	writeConfigFile(t, "")

	_, err := resolveRepoRoot("", "o/r")
	if err == nil {
		t.Fatal("with no cwd match, no registration and no flag this should fail")
	}
	// The message has to list the ways out.
	for _, want := range []string{"repo add", "--repo-root"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

// After `:submit` runs in a subprocess the TUI re-reads the file to pick
// up the submission record.
func TestReloadSubmissionMeta(t *testing.T) {
	isolateEnv(t)
	dir := fullReviewFixture(t)

	r, err := store.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.SubmittedAt != nil {
		t.Fatal("the fixture should start unsubmitted")
	}

	// Simulate the subprocess recording a submission.
	raw, err := os.ReadFile(filepath.Join(dir, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	updated := string(raw) + "submitted_at: 2026-08-11T00:00:00Z\nreview_id: 4242\n"
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := reloadSubmissionMeta(r); err != nil {
		t.Fatal(err)
	}
	if r.SubmittedAt == nil {
		t.Fatal("SubmittedAt should have been picked up")
	}
	if r.ReviewID == nil || *r.ReviewID != 4242 {
		t.Fatalf("ReviewID = %v, want 4242", r.ReviewID)
	}
}

func TestReloadSubmissionMetaOnAnUnreadableReview(t *testing.T) {
	isolateEnv(t)
	r := &model.Review{BaseDir: filepath.Join(t.TempDir(), "gone")}

	if err := reloadSubmissionMeta(r); err == nil {
		t.Fatal("a missing review.yml should surface as an error")
	}
}

// pickReviewedPRDir opens an interactive picker, so only the resolution
// that happens before it is in scope here.
func TestPickReviewedPRDirWithAnEmptyRepoDir(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	repoDir, err := store.RepoDir("o/r")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = pickReviewedPRDir(newOpenCmd(), "o/r")
	if err == nil {
		t.Fatal("with no reviews on disk this should fail")
	}
	if !strings.Contains(err.Error(), "revu review") {
		t.Fatalf("error = %q, want it to point at how to create one", err)
	}
}

// A repo that has never been reviewed has no directory at all. Today that
// surfaces as the raw stat error rather than the friendly message above —
// pinned so a future fix is a deliberate change, not a surprise.
func TestPickReviewedPRDirForANeverReviewedRepo(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)

	_, err := pickReviewedPRDir(newOpenCmd(), "o/r")
	if err == nil {
		t.Fatal("a never-reviewed repo should fail")
	}
	if !strings.Contains(err.Error(), "read repo dir") {
		t.Fatalf("error = %q, want the directory read error", err)
	}
}

func TestPickReviewedPRDirOutsideAClone(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)

	_, err := pickReviewedPRDir(newOpenCmd(), "")
	if err == nil {
		t.Fatal("without a slug or a clone this should fail")
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Fatalf("error = %q, want it to suggest --repo", err)
	}
}

func TestJobsListWhenEmpty(t *testing.T) {
	isolateEnv(t)

	out := runCmd(t, newJobsListCmd())
	if !strings.Contains(out, "No background jobs") {
		t.Fatalf("an empty book should say so:\n%s", out)
	}
	if !strings.Contains(out, "--bg") {
		t.Fatalf("it should point at how to start one:\n%s", out)
	}
}

func TestJobsListShowsStateAndEngine(t *testing.T) {
	isolateEnv(t)
	seedJobRecord(t, "o/r", 7, jobs.StateDone, func(j *jobs.Job) { j.Model = "claude-opus-5" })
	seedJobRecord(t, "o/r", 8, jobs.StateFailed, func(j *jobs.Job) {
		j.Err = "spawn worker: boom\nstack trace here"
	})

	out := runCmd(t, newJobsListCmd())
	for _, want := range []string{"o/r#7", "o/r#8", "done", "failed", "claude(claude-opus-5)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("jobs list is missing %q:\n%s", want, out)
		}
	}
	// A multi-line failure is collapsed so the list stays one row per job.
	if !strings.Contains(out, "err: spawn worker: boom ...") {
		t.Fatalf("the failure should be shown on one line:\n%s", out)
	}
	if strings.Contains(out, "stack trace here") {
		t.Fatalf("the rest of the error should not spill into the list:\n%s", out)
	}
}

func TestJobsLog(t *testing.T) {
	isolateEnv(t)
	j := seedJobRecord(t, "o/r", 7, jobs.StateDone, nil)
	if err := os.WriteFile(j.LogPath, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, newJobsLogCmd(), j.ID)
	if out != "line one\nline two\n" {
		t.Fatalf("log = %q, want the file contents verbatim", out)
	}
}

func TestJobsLogUnknownJob(t *testing.T) {
	isolateEnv(t)

	_, err := runCmdErr(t, newJobsLogCmd(), "no-such-job")
	if err == nil {
		t.Fatal("an unknown job id should fail")
	}
	if !strings.Contains(err.Error(), "no-such-job") {
		t.Fatalf("error = %q, want the id echoed", err)
	}
}

// A job whose worker never wrote a log is a different failure from an
// unknown job, and the message has to distinguish them.
func TestJobsLogWithoutALogFile(t *testing.T) {
	isolateEnv(t)
	j := seedJobRecord(t, "o/r", 7, jobs.StateRunning, nil)

	_, err := runCmdErr(t, newJobsLogCmd(), j.ID)
	if err == nil {
		t.Fatal("a missing log file should fail")
	}
	if !strings.Contains(err.Error(), "read job log") {
		t.Fatalf("error = %q, want it to blame the log read", err)
	}
}

// seedJobRecord writes a job to the book under the isolated REVU_HOME.
func seedJobRecord(t *testing.T, slug string, pr int, state jobs.State, mutate func(*jobs.Job)) jobs.Job {
	t.Helper()
	j, err := jobs.New(slug, pr, "claude", "bg", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	j.State = state
	if mutate != nil {
		mutate(&j)
	}
	if err := jobs.Save(j); err != nil {
		t.Fatal(err)
	}
	return j
}

// writeConfigFile points $REVU_CONFIG at a throwaway config.
func writeConfigFile(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)
}

// chdirTo moves into dir for the duration of the test.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}
