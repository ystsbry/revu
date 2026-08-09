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

	bin, err := deps.executable()
	if err != nil {
		return fmt.Errorf("locate revu binary for the worker: %w", err)
	}
	job, err := jobs.StartReview(jobs.StartReviewOptions{
		Slug:    opts.Slug,
		PR:      opts.PRNumber,
		Engine:  string(opts.Engine),
		Focus:   opts.Focus,
		Model:   opts.Model,
		WorkDir: workDir,
		RevuBin: bin,
		Now:     deps.now,
		Spawn:   deps.spawnWorker,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Started background review job %s (pid %d)\n", job.ID, job.PID)
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
