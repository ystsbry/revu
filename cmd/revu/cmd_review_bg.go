package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/jobs"
)

// startBackgroundReview registers a job in the job book and spawns the
// detached worker for it, returning as soon as the worker is off the
// ground. The review itself is the worker's problem (cmd_review_worker.go).
func (deps reviewDeps) startBackgroundReview(cmd *cobra.Command, opts reviewOptions) error {
	workDir, err := deps.resolveBGWorkDir(opts.Slug, opts.RepoSlug != "")
	if err != nil {
		return err
	}

	now := deps.now()
	if running, ok := jobs.RunningFor(opts.Slug, opts.PRNumber, now); ok {
		return fmt.Errorf("a review for %s#%d is already running (job %s) — wait for it or check `revu jobs log %s`",
			opts.Slug, opts.PRNumber, running.ID, running.ID)
	}

	job, err := jobs.New(opts.Slug, opts.PRNumber, string(opts.Engine), "bg", opts.Focus, now)
	if err != nil {
		return err
	}
	job.WorkDir = workDir
	if err := jobs.Save(job); err != nil {
		return err
	}

	bin, err := deps.executable()
	if err != nil {
		return fmt.Errorf("locate revu binary for the worker: %w", err)
	}
	pid, err := deps.spawnWorker(bin, job)
	if err != nil {
		// The record must not sit as running forever when the worker
		// never existed; write the failure down where the TUI will see it.
		job.State = jobs.StateFailed
		job.Err = fmt.Sprintf("spawn worker: %v", err)
		finished := deps.now()
		job.FinishedAt = &finished
		_ = jobs.Save(job)
		return fmt.Errorf("spawn worker: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Started background review job %s (pid %d)\n", job.ID, pid)
	fmt.Fprintf(out, "  repo:  %s#%d (%s)\n", opts.Slug, opts.PRNumber, opts.Engine)
	fmt.Fprintf(out, "  log:   revu jobs log %s\n", job.ID)
	fmt.Fprintf(out, "  state: revu jobs list\n")
	return nil
}

// resolveBGWorkDir picks the clone the worker will run in. With --repo the
// registry is authoritative; otherwise the cwd's clone is used (the same
// tree a foreground review would run in). Either way the directory must
// exist now — failing fast beats a job that dies at boot.
func (deps reviewDeps) resolveBGWorkDir(slug string, viaRepoFlag bool) (string, error) {
	if viaRepoFlag {
		cfg, _, err := config.Load()
		if err != nil {
			return "", fmt.Errorf("load config: %w", err)
		}
		def, ok := cfg.FindRepo(slug)
		if !ok {
			return "", fmt.Errorf("%s is not registered — run `revu repo add <path>` (or `revu repo scan`) first", slug)
		}
		st, err := os.Stat(def.Path)
		if err != nil || !st.IsDir() {
			return "", fmt.Errorf("registered clone for %s not found at %s", slug, def.Path)
		}
		return def.Path, nil
	}
	root, err := deps.cwdRoot()
	if err != nil {
		return "", errors.New("resolve cwd clone: run inside a git clone, or pass --repo <owner>/<repo>")
	}
	return root, nil
}
