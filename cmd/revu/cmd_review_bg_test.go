package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ystsbry/revu/internal/claude"
	"github.com/ystsbry/revu/internal/jobs"
)

// bgDeps is stubDeps plus a recording spawner and a temp cwd clone.
func bgDeps(t *testing.T, rec *resumeCall, spawned *jobs.Job) reviewDeps {
	t.Helper()
	deps := stubDeps(t, rec)
	deps.cwdRoot = func() (string, error) { return t.TempDir(), nil }
	deps.spawnWorker = func(bin string, j jobs.Job) (int, error) {
		*spawned = j
		return 4242, nil
	}
	return deps
}

func TestReviewBGStartsJob(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	var rec resumeCall
	var spawned jobs.Job
	deps := bgDeps(t, &rec, &spawned)

	stdout, _, err := runReviewCmd(t, deps, "7", "--bg", "--codex")
	if err != nil {
		t.Fatal(err)
	}

	if spawned.ID == "" {
		t.Fatal("worker was never spawned")
	}
	if spawned.Slug != "owner/repo" || spawned.PR != 7 || spawned.Engine != "codex" || spawned.Mode != "bg" {
		t.Errorf("spawned job = %+v", spawned)
	}
	if spawned.WorkDir == "" {
		t.Error("job must carry the resolved cwd clone")
	}

	// The job book holds the running record the TUI will read.
	j, err := jobs.Load(spawned.ID)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != jobs.StateRunning {
		t.Errorf("job state = %s, want running", j.State)
	}

	for _, want := range []string{spawned.ID, "revu jobs log"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q:\n%s", want, stdout)
		}
	}
	if rec.called {
		t.Error("--bg must not resume into the agent TUI")
	}
}

func TestReviewBGRejectsDuplicateRunning(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	var rec resumeCall
	var spawned jobs.Job
	deps := bgDeps(t, &rec, &spawned)

	// Seed a live running job for the same slug#pr.
	existing, err := jobs.New("owner/repo", 7, "claude", "bg", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	existing.PID = os.Getpid()
	if err := jobs.Save(existing); err != nil {
		t.Fatal(err)
	}

	_, _, err = runReviewCmd(t, deps, "7", "--bg")
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("duplicate start should be rejected, got %v", err)
	}
	if spawned.ID != "" {
		t.Error("no worker may spawn for a duplicate")
	}
}

func TestReviewBGRepoResolvesRegisteredClone(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	clone := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := "[[repo]]\nslug = \"acme/api\"\npath = \"" + clone + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfgPath)

	var rec resumeCall
	var spawned jobs.Job
	deps := bgDeps(t, &rec, &spawned)
	deps.cwdSlug = func() (string, error) { return "", errors.New("not in a repo") }
	deps.cwdRoot = func() (string, error) { return "", errors.New("not in a repo") }

	_, _, err := runReviewCmd(t, deps, "9", "--bg", "--repo", "acme/api")
	if err != nil {
		t.Fatal(err)
	}
	if spawned.Slug != "acme/api" || spawned.WorkDir != clone {
		t.Errorf("job = slug %q workdir %q, want acme/api / %q", spawned.Slug, spawned.WorkDir, clone)
	}

	// Unregistered slug fails fast, before any job is written.
	spawned = jobs.Job{}
	_, _, err = runReviewCmd(t, deps, "9", "--bg", "--repo", "nope/nope")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered --repo should error, got %v", err)
	}
	if spawned.ID != "" {
		t.Error("no worker may spawn for an unregistered repo")
	}
}

func TestReviewRepoRequiresBG(t *testing.T) {
	t.Parallel()
	var rec resumeCall
	deps := stubDeps(t, &rec)
	_, _, err := runReviewCmd(t, deps, "7", "--repo", "acme/api")
	if err == nil || !strings.Contains(err.Error(), "--repo requires --bg") {
		t.Fatalf("want --repo/--bg pairing error, got %v", err)
	}
}

func TestReviewBGSpawnFailureMarksJobFailed(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	var rec resumeCall
	var spawned jobs.Job
	deps := bgDeps(t, &rec, &spawned)
	deps.spawnWorker = func(bin string, j jobs.Job) (int, error) {
		spawned = j
		return 0, errors.New("fork bomb shields up")
	}

	_, _, err := runReviewCmd(t, deps, "7", "--bg")
	if err == nil {
		t.Fatal("spawn failure must surface")
	}
	j, loadErr := jobs.Load(spawned.ID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if j.State != jobs.StateFailed || !strings.Contains(j.Err, "fork bomb") {
		t.Errorf("job after spawn failure = %+v, want failed with cause", j)
	}
}

// --- worker ----------------------------------------------------------------

func runWorker(t *testing.T, deps reviewDeps, jobID string) error {
	t.Helper()
	cmd := newReviewWorkerCmdWith(deps)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--job", jobID})
	return cmd.ExecuteContext(context.Background())
}

func seedJob(t *testing.T, workDir string) jobs.Job {
	t.Helper()
	j, err := jobs.New("owner/repo", 7, "claude", "bg", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	j.WorkDir = workDir
	if err := jobs.Save(j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestWorkerTransitionsToDone(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	outDir := reviewFixture(t)
	var rec resumeCall
	var gotWorkDir string
	deps := stubDeps(t, &rec)
	deps.runClaude = func(_ context.Context, args claude.ReviewArgs) (claude.ReviewResult, error) {
		gotWorkDir = args.WorkDir
		return claude.ReviewResult{OutDir: outDir, SessionID: "sess-1"}, nil
	}

	job := seedJob(t, t.TempDir())
	if err := runWorker(t, deps, job.ID); err != nil {
		t.Fatal(err)
	}

	got, err := jobs.Load(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != jobs.StateDone {
		t.Fatalf("state = %s, want done (err=%q)", got.State, got.Err)
	}
	if got.OutDir != outDir || got.SessionID != "sess-1" {
		t.Errorf("result not recorded: %+v", got)
	}
	if got.PID != os.Getpid() {
		t.Errorf("worker pid = %d, want %d", got.PID, os.Getpid())
	}
	if got.FinishedAt == nil {
		t.Error("finished_at not recorded")
	}
	if gotWorkDir != job.WorkDir {
		t.Errorf("agent ran in %q, want the job's clone %q", gotWorkDir, job.WorkDir)
	}
}

func TestWorkerTransitionsToFailed(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	var rec resumeCall
	deps := stubDeps(t, &rec)
	deps.runClaude = func(context.Context, claude.ReviewArgs) (claude.ReviewResult, error) {
		return claude.ReviewResult{}, errors.New("agent exploded")
	}

	job := seedJob(t, t.TempDir())
	if err := runWorker(t, deps, job.ID); err == nil {
		t.Fatal("worker must propagate the failure")
	}

	got, err := jobs.Load(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != jobs.StateFailed || !strings.Contains(got.Err, "agent exploded") {
		t.Errorf("job = %+v, want failed with cause", got)
	}
	if got.FinishedAt == nil {
		t.Error("finished_at not recorded on failure")
	}
}

func TestWorkerFailsWhenCloneIsGone(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	var rec resumeCall
	deps := stubDeps(t, &rec) // runClaude stub errors the test if called

	job := seedJob(t, filepath.Join(t.TempDir(), "does-not-exist"))
	if err := runWorker(t, deps, job.ID); err == nil {
		t.Fatal("missing clone must fail the job")
	}

	got, err := jobs.Load(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != jobs.StateFailed || !strings.Contains(got.Err, "revu repo add") {
		t.Errorf("job = %+v, want failed with re-register hint", got)
	}
}
