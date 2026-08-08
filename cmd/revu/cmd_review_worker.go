package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/jobs"
)

// newReviewWorkerCmd is the hidden entry point `revu review --bg` spawns
// (detached, stdout/stderr already redirected to the job log). It runs the
// same generation pipeline as a non-interactive `revu review` and writes
// the outcome — running→done/failed, session id, output dir — into the
// job book, which is the only channel the parent and the TUI observe.
func newReviewWorkerCmd() *cobra.Command { return newReviewWorkerCmdWith(defaultReviewDeps()) }

func newReviewWorkerCmdWith(deps reviewDeps) *cobra.Command {
	var jobID string
	cmd := &cobra.Command{
		Use:    "_review-worker",
		Short:  "internal: run one background review job",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jobID == "" {
				return errors.New("--job is required")
			}
			return deps.runReviewWorker(cmd, jobID)
		},
	}
	cmd.Flags().StringVar(&jobID, "job", "", "job id to execute")
	return cmd
}

func (deps reviewDeps) runReviewWorker(cmd *cobra.Command, jobID string) error {
	job, err := jobs.Load(jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}

	// Record our PID first thing: from here on the read side can tell a
	// live worker from a dead one. The worker is the file's only writer
	// now, so this cannot race the parent.
	job.PID = os.Getpid()
	job.State = jobs.StateRunning
	if err := jobs.Save(job); err != nil {
		return fmt.Errorf("record worker pid: %w", err)
	}

	fail := func(cause error) error {
		job.State = jobs.StateFailed
		job.Err = cause.Error()
		finished := deps.now()
		job.FinishedAt = &finished
		if saveErr := jobs.Save(job); saveErr != nil {
			return fmt.Errorf("%w (and saving the failure failed too: %v)", cause, saveErr)
		}
		return cause
	}

	if st, statErr := os.Stat(job.WorkDir); statErr != nil || !st.IsDir() {
		return fail(fmt.Errorf("clone %s is gone — re-register it with `revu repo add`", job.WorkDir))
	}

	res, err := deps.generateReview(cmd.Context(), reviewGenOptions{
		Engine:   reviewEngine(job.Engine),
		Slug:     job.Slug,
		PRNumber: job.PR,
		Focus:    job.Focus,
		WorkDir:  job.WorkDir,
		// stdout is the job log; nothing may block on input in a
		// detached process; and a run that wrote nothing must fail
		// rather than adopt an older review dir.
		Progress:  cmd.OutOrStdout(),
		Warn:      cmd.ErrOrStderr(),
		Stdin:     strings.NewReader(""),
		NotBefore: job.StartedAt,
	})
	if err != nil {
		return fail(err)
	}

	job.State = jobs.StateDone
	job.OutDir = res.OutDir
	job.SessionID = res.SessionID
	finished := deps.now()
	job.FinishedAt = &finished
	if err := jobs.Save(job); err != nil {
		return fmt.Errorf("record job result: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\njob %s done: %s\n", job.ID, res.OutDir)
	return nil
}
