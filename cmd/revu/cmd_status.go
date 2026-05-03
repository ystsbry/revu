package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/store"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [dir]",
		Short: "Print accept/reject counts for a review directory",
		Args:  cobra.MaximumNArgs(1),
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
			c := r.Counts()
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Repo:       %s\n", r.PR.Repo)
			fmt.Fprintf(out, "PR:         #%d\n", r.PR.Number)
			fmt.Fprintf(out, "Head SHA:   %s\n", r.PR.HeadSHA)
			fmt.Fprintf(out, "Event:      %s\n", r.ReviewEvent)
			fmt.Fprintf(out, "Comments:   %d total\n", len(r.Comments))
			fmt.Fprintf(out, "  pending:  %d\n", c[model.StatusPending])
			fmt.Fprintf(out, "  accepted: %d\n", c[model.StatusAccepted])
			fmt.Fprintf(out, "  rejected: %d\n", c[model.StatusRejected])
			fmt.Fprintf(out, "  edited:   %d\n", c[model.StatusEdited])
			if r.SubmittedAt != nil {
				fmt.Fprintf(out, "Submitted:  %s (review_id=%d)\n", r.SubmittedAt.Format("2006-01-02 15:04:05 -0700"), derefInt64(r.ReviewID))
			} else {
				fmt.Fprintln(out, "Submitted:  not yet")
			}
			return nil
		},
	}
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
