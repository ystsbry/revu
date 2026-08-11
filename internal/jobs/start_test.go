package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// StartReview already takes Now/Spawn/RevuBin, so the whole start path can
// be exercised without launching a worker.
func startOpts(spawn func(string, Job) (int, error), now time.Time) StartReviewOptions {
	return StartReviewOptions{
		Slug:    "o/r",
		PR:      7,
		Engine:  "claude",
		Focus:   "security",
		Model:   "claude-opus-5",
		PRTitle: "add dashboard",
		WorkDir: "/tmp/clone",
		RevuBin: "/usr/local/bin/revu",
		Now:     func() time.Time { return now },
		Spawn:   spawn,
	}
}

func TestStartReviewRegistersAndSpawns(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	now := time.Now()

	var gotBin string
	var gotJob Job
	job, err := StartReview(startOpts(func(bin string, j Job) (int, error) {
		gotBin, gotJob = bin, j
		return 4321, nil
	}, now))
	if err != nil {
		t.Fatal(err)
	}

	if job.PID != 4321 {
		t.Errorf("PID = %d, want the spawned pid", job.PID)
	}
	if job.State != StateRunning {
		t.Errorf("state = %q, want running", job.State)
	}
	for _, tt := range []struct{ name, got, want string }{
		{"slug", job.Slug, "o/r"},
		{"engine", job.Engine, "claude"},
		{"mode", job.Mode, "bg"},
		{"focus", job.Focus, "security"},
		{"model", job.Model, "claude-opus-5"},
		{"title", job.PRTitle, "add dashboard"},
		{"work dir", job.WorkDir, "/tmp/clone"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}

	if gotBin != "/usr/local/bin/revu" {
		t.Errorf("spawned bin = %q, want the configured revu binary", gotBin)
	}
	if gotJob.ID != job.ID {
		t.Errorf("the worker was handed job %q, want %q", gotJob.ID, job.ID)
	}

	// The record has to be on disk before the worker starts, otherwise the
	// worker cannot find its own job.
	saved, err := Load(job.ID)
	if err != nil {
		t.Fatalf("the job should be in the book: %v", err)
	}
	if saved.Slug != "o/r" || saved.PR != 7 {
		t.Fatalf("saved job = %+v, want o/r#7", saved)
	}
}

// Two concurrent reviews of the same PR would race on the same review
// directory, so the second one is refused.
func TestStartReviewRefusesADuplicateRun(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	now := time.Now()

	first, err := StartReview(startOpts(func(string, Job) (int, error) { return os.Getpid(), nil }, now))
	if err != nil {
		t.Fatal(err)
	}

	_, err = StartReview(startOpts(func(string, Job) (int, error) {
		t.Fatal("the second start must not spawn a worker")
		return 0, nil
	}, now))
	if err == nil {
		t.Fatal("a second review of the same PR should be refused")
	}
	if !strings.Contains(err.Error(), first.ID) {
		t.Errorf("error = %q, want it to name the running job", err)
	}

	// A different PR is unaffected.
	other := startOpts(func(string, Job) (int, error) { return os.Getpid(), nil }, now)
	other.PR = 8
	if _, err := StartReview(other); err != nil {
		t.Fatalf("a different PR should still start: %v", err)
	}
}

// A spawn failure must not leave a record claiming to be running for a
// worker that never existed.
func TestStartReviewRecordsASpawnFailure(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	now := time.Now()
	boom := errors.New("fork/exec: permission denied")

	job, err := StartReview(startOpts(func(string, Job) (int, error) { return 0, boom }, now))
	if err == nil {
		t.Fatal("a spawn failure should be returned")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want it to wrap the spawn error", err)
	}

	saved, loadErr := Load(job.ID)
	if loadErr != nil {
		t.Fatalf("the failed job should still be in the book: %v", loadErr)
	}
	if saved.State != StateFailed {
		t.Errorf("state = %q, want failed", saved.State)
	}
	if saved.FinishedAt == nil {
		t.Error("a failed job should carry a finish time")
	}
	if !strings.Contains(saved.Err, "permission denied") {
		t.Errorf("recorded error = %q, want the spawn cause", saved.Err)
	}

	// The PR is free to be retried immediately.
	if _, ok := RunningFor("o/r", 7, now); ok {
		t.Error("a failed start must not hold the PR as running")
	}
}

// With no RevuBin the current executable is used — under `go test` that is
// the test binary, which is enough to prove the fallback resolves.
func TestStartReviewFallsBackToTheCurrentExecutable(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())

	var gotBin string
	opts := startOpts(func(bin string, _ Job) (int, error) {
		gotBin = bin
		return 1, nil
	}, time.Now())
	opts.RevuBin = ""

	if _, err := StartReview(opts); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Skip("cannot resolve the test executable in this environment")
	}
	if gotBin != self {
		t.Fatalf("spawned bin = %q, want the current executable %q", gotBin, self)
	}
}

// SpawnWorker is the thin wrapper the production path uses; what matters
// is the argv it hands the worker.
func TestSpawnWorkerLaunchesTheHiddenSubcommand(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "job.log")
	job := Job{ID: "job-x", LogPath: logPath}

	// `echo` stands in for revu: it exits immediately and writes its argv
	// to the job log, which is exactly what needs asserting.
	pid, err := SpawnWorker("echo", job)
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 {
		t.Fatalf("pid = %d, want a real pid", pid)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		raw, err := os.ReadFile(logPath)
		if err == nil && len(raw) > 0 {
			if got := strings.TrimSpace(string(raw)); got != "_review-worker --job job-x" {
				t.Fatalf("worker argv = %q, want the hidden subcommand with the job id", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker never wrote to its log")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSpawnWorkerReportsAnUnopenableLog(t *testing.T) {
	t.Parallel()
	job := Job{ID: "job-x", LogPath: filepath.Join(t.TempDir(), "no-such-dir", "job.log")}

	if _, err := SpawnWorker("echo", job); err == nil {
		t.Fatal("an unopenable log should fail before spawning")
	}
}
