package main

import (
	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/store"
	"github.com/ystsbry/revu/internal/tui"
)

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [dir]",
		Short: "Open a review directory in the TUI",
		Long: `Open a review directory in the TUI.

If [dir] is omitted, the latest pr-N directory under ~/.revu/{owner}/{repo}/
is resolved from the current git remote.`,
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
			return tui.Run(r, store.SaveStatuses)
		},
	}
}
