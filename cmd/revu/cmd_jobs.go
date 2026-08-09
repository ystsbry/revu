package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/jobs"
)

func newJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Inspect background review jobs (see `revu review --bg`)",
	}
	cmd.AddCommand(newJobsListCmd())
	cmd.AddCommand(newJobsLogCmd())
	return cmd
}

func newJobsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List background review jobs, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := jobs.List()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(all) == 0 {
				fmt.Fprintln(out, "No background jobs. Start one with `revu review <PR> --bg`.")
				return nil
			}
			now := time.Now()
			for _, j := range all {
				state, errText := j.Effective(now)
				engine := j.Engine
				if j.Model != "" {
					engine += "(" + j.Model + ")"
				}
				line := fmt.Sprintf("%-8s %s  %s#%d  %s  %s",
					state, j.ID, j.Slug, j.PR, engine, j.StartedAt.Format("2006-01-02 15:04"))
				if state == jobs.StateFailed && errText != "" {
					line += "  err: " + firstLine(errText)
				}
				fmt.Fprintln(out, line)
			}
			return nil
		},
	}
}

func newJobsLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log <job-id>",
		Short: "Print a background job's log",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := jobs.Load(args[0])
			if err != nil {
				return fmt.Errorf("unknown job %s: %w", args[0], err)
			}
			raw, err := os.ReadFile(j.LogPath)
			if err != nil {
				return fmt.Errorf("read job log: %w", err)
			}
			_, err = cmd.OutOrStdout().Write(raw)
			return err
		},
	}
}

// firstLine truncates s at its first newline so job errors stay one row
// in the list output.
func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i] + " ..."
		}
	}
	return s
}
