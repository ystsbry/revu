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
	var (
		dryRun        bool
		yes           bool
		acceptPending bool
		noApprove     bool
	)
	cmd := &cobra.Command{
		Use:   "submit [dir]",
		Short: "Submit the review to GitHub",
		Long: `Submit the review (summary + accepted inline comments) to GitHub.

Without --dry-run or --yes, you must type 'submit' literally to confirm.
The flow refuses to proceed if:
  - gh is not authenticated
  - the PR's head_sha has moved since the review was generated
  - review.yml already records a successful submission

Use --no-approve from CI: GitHub does not permit the Actions GITHUB_TOKEN to
approve pull requests, so an APPROVE review fails with HTTP 422 and nothing is
posted. With the flag such a review is posted as COMMENT instead.`,
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
				Review:        r,
				Client:        github.New(),
				Saver:         store.SaveStatuses,
				Out:           cmd.OutOrStdout(),
				In:            os.Stdin,
				DryRun:        dryRun,
				Yes:           yes,
				AcceptPending: acceptPending,
				NoApprove:     noApprove,
			}
			return submit.Run(context.Background(), opts)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the payload without contacting GitHub")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the typed-confirmation prompt (for non-interactive use)")
	cmd.Flags().BoolVar(&acceptPending, "accept-pending", false, "promote pending comments to accepted before submitting (for non-interactive use; rejected comments are left untouched)")
	cmd.Flags().BoolVar(&noApprove, "no-approve", false, "post an APPROVE review as COMMENT instead (for CI: the Actions GITHUB_TOKEN is not permitted to approve pull requests; COMMENT and REQUEST_CHANGES are unaffected)")
	return cmd
}
