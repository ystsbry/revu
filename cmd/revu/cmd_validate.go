package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/store"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [dir]",
		Short: "Validate a review directory's YAML/Markdown integrity",
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
			counts := r.Counts()
			fmt.Fprintf(cmd.OutOrStdout(),
				"OK %s (PR #%d, %d comments: pending=%d accepted=%d rejected=%d edited=%d)\n",
				dir, r.PR.Number, len(r.Comments),
				counts["pending"], counts["accepted"], counts["rejected"], counts["edited"],
			)
			return nil
		},
	}
}
