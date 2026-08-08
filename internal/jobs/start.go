package jobs

import (
	"fmt"
	"os"
	"time"
)

// StartReviewOptions configures one background review start.
type StartReviewOptions struct {
	Slug   string // "owner/repo"
	PR     int
	Engine string // "claude" | "codex"
	Focus  string
	// WorkDir is the clone the worker runs in; the caller resolves it
	// (registry or cwd) so a bad path fails here, not at worker boot.
	WorkDir string

	// RevuBin is the binary to spawn the worker from. Empty resolves the
	// current executable.
	RevuBin string
	// Now defaults to time.Now; tests pin it.
	Now func() time.Time
	// Spawn defaults to SpawnWorker; tests capture it.
	Spawn func(revuBin string, j Job) (int, error)
}

// StartReview registers a job in the book and spawns its detached worker,
// returning the running job as soon as the worker is off the ground. This
// is the single start path shared by `revu review --bg` and the dashboard.
//
// A job for a slug#pr that is already effectively running is refused. A
// spawn failure is written to the book as failed before it is returned,
// so the record never sits as running for a worker that never existed.
func StartReview(opts StartReviewOptions) (Job, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	spawn := SpawnWorker
	if opts.Spawn != nil {
		spawn = opts.Spawn
	}

	if running, ok := RunningFor(opts.Slug, opts.PR, now()); ok {
		return Job{}, fmt.Errorf("a review for %s#%d is already running (job %s) — wait for it or check `revu jobs log %s`",
			opts.Slug, opts.PR, running.ID, running.ID)
	}

	job, err := New(opts.Slug, opts.PR, opts.Engine, "bg", opts.Focus, now())
	if err != nil {
		return Job{}, err
	}
	job.WorkDir = opts.WorkDir
	if err := Save(job); err != nil {
		return Job{}, err
	}

	bin := opts.RevuBin
	if bin == "" {
		bin, err = os.Executable()
		if err != nil {
			return Job{}, fmt.Errorf("locate revu binary for the worker: %w", err)
		}
	}
	pid, err := spawn(bin, job)
	if err != nil {
		job.State = StateFailed
		job.Err = fmt.Sprintf("spawn worker: %v", err)
		finished := now()
		job.FinishedAt = &finished
		_ = Save(job)
		return job, fmt.Errorf("spawn worker: %w", err)
	}
	job.PID = pid
	return job, nil
}
