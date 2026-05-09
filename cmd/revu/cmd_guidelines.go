package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/guideline"
)

func newGuidelinesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guidelines",
		Short: "Inspect or emit the review-guideline files configured under [review] guidelines",
		Long: `Each config layer (~/.config/revu/, <repo>/.revu/, <repo>/.revu-local/)
may set a list of guideline file paths under [review] guidelines = [...].
Paths are resolved relative to the layer's config.toml and concatenated
in layer order (user → .revu → .revu-local).

Use 'revu guidelines paths' from the review-pr skill (or any script) to
get one absolute path per line, filtered to files that currently exist.
Use 'revu guidelines list' for a human-readable status table that
includes missing entries.`,
	}
	cmd.AddCommand(newGuidelinesPathsCmd())
	cmd.AddCommand(newGuidelinesListCmd())
	return cmd
}

func newGuidelinesPathsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Print existing guideline files, one absolute path per line",
		Long: `Emits one absolute path per line for every configured guideline whose
file currently exists. Missing entries are silently skipped, so callers
can wrap the output in a Read loop without extra checks:

  mapfile -t GUIDELINES < <(revu guidelines paths 2>/dev/null)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := guideline.Paths()
			if err != nil {
				return err
			}
			for _, p := range paths {
				fmt.Fprintln(cmd.OutOrStdout(), p)
			}
			return nil
		},
	}
}

func newGuidelinesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show every configured guideline with its existence status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			items, err := guideline.List()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(out, "No guidelines configured. Set [review] guidelines = [...] in any config layer.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  #\tSTATUS\tPATH")
			for i, r := range items {
				status := "MISSING"
				if r.Exists {
					status = "OK"
				}
				fmt.Fprintf(tw, "  %d\t%s\t%s\n", i+1, status, r.Path)
			}
			return tw.Flush()
		},
	}
}
