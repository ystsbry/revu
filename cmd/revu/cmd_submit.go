package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/store"
	"github.com/ystsbry/revu/internal/submit"
)

func newSubmitCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "submit [dir]",
		Short: "Submit the review to GitHub",
		Long: `Submit the review (summary + accepted inline comments) to GitHub.

Without --dry-run, you must type 'submit' literally to confirm.
The flow refuses to proceed if:
  - gh is not authenticated
  - the PR's head_sha has moved since the review was generated
  - review.yml already records a successful submission`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			dir, err := store.ResolveReviewDir(arg)
			if err != nil {
				return err
			}
			r, err := store.Load(dir)
			if err != nil {
				return err
			}

			opts := submit.Options{
				Review: r,
				Client: github.New(),
				Saver:  store.SaveStatuses,
				Out:    cmd.OutOrStdout(),
				In:     os.Stdin,
				DryRun: dryRun,
			}
			return submit.Run(context.Background(), opts)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the payload without contacting GitHub")
	return cmd
}
