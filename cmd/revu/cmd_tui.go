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

The home screen shows the registered repositories in a sidebar (narrowed
by the active profile; the shown repo carries a thick bar) and the
selected repository's open PRs as cards on the right, filtered by the
per-repo search condition. Click or Enter on a card opens the PR's
action panel, and the review TUI below it. Cards carry
[reviewed]/[submitted] badges, and background jobs show as
[running]/[failed]. The job tab lists every background job (newest
first) with its repository, PR, outcome, and job id. The action panel can generate reviews in the
background (a detached job) or in the foreground, and resume the agent
session — foreground and resume hand the terminal to the agent until it
exits, then return here. The existing commands ("revu open",
"revu review", "revu resume", ...) are unaffected and keep working
exactly as before.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dashboard.Run()
		},
	}
}
