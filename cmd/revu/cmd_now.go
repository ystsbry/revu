package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// newNowCmd prints the current ISO 8601 timestamp the review-pr skill
// embeds in review.yml's generated_at. Replaces the skill's `date -Iseconds`
// invocation so the allowlist needs only Bash(revu *).
func newNowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "now",
		Short: "Print the current ISO 8601 timestamp",
		Long:  `Wraps "date -Iseconds" so the review-pr skill needs only Bash(revu *).`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), time.Now().Format(time.RFC3339))
			return nil
		},
	}
}
