package main

import (
	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/tui/dashboard"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the multi-repository review dashboard",
		Long: `Open the review dashboard.

Unlike the other commands, "revu tui" does not need to run inside a git
clone: it works across every registered repository, letting you browse
PRs, generate reviews, and open the results from one place.

The dashboard navigates the reviews stored under ~/.revu: repositories
(L0) → reviewed PRs (L1) → PR actions (L2) → the review TUI itself (L3),
with Enter descending and Esc/q going back up. The config-registered
repository list and review-generation actions land in later changes. The
existing commands ("revu open", "revu review", "revu resume", ...) are
unaffected and keep working exactly as before.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dashboard.Run()
		},
	}
}
