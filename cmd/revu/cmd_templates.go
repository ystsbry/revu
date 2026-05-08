package main

import (
	"errors"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/template"
)

func newTemplatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Resolve and inspect user-managed review templates",
		Long: `revu templates exposes the file-resolution layer the review-pr skill uses
when generating reviews. Templates are looked up across config layers,
highest priority first:

  1. <repo-root>/.revu-local/templates/<NAME>     (per-clone, gitignored)
  2. <repo-root>/.revu/templates/<NAME>           (project-shared, committed)
  3. $REVU_TEMPLATES/<NAME>                       (when env set)
  4. ~/.config/revu/templates/<NAME>              (global)

Skill-bundled templates are NOT considered here; the skill itself owns
its install path and falls back to its bundled copy when 'revu templates
path' reports the file is not found.`,
	}
	cmd.AddCommand(newTemplatesPathCmd())
	cmd.AddCommand(newTemplatesListCmd())
	return cmd
}

func newTemplatesPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <NAME>",
		Short: "Print the absolute path of the highest-priority override for NAME",
		Long: `Prints one line: the absolute path of the highest-priority template
named NAME (e.g. "summary.md.tmpl"). Exits non-zero with no output on
stdout when no override exists, so callers can fall back via:

  P=$(revu templates path summary.md.tmpl 2>/dev/null) || P=$DEFAULT`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := template.Resolve(args[0])
			if err != nil {
				if errors.Is(err, template.ErrNotFound) {
					return err
				}
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
		SilenceErrors: true,
	}
}

func newTemplatesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show every discovered template and which layer it came from",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			dirs, err := template.SearchDirs()
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "Search dirs (highest → lowest priority):")
			for _, d := range dirs {
				status := "not present"
				if d.Loaded {
					status = "exists"
				}
				fmt.Fprintf(out, "  [%-11s] %-12s %s\n", d.Layer, status, d.Path)
			}

			items, err := template.List()
			if err != nil {
				return err
			}
			fmt.Fprintln(out)
			if len(items) == 0 {
				fmt.Fprintln(out, "No template overrides discovered.")
				return nil
			}
			names := make([]string, 0, len(items))
			for n := range items {
				names = append(names, n)
			}
			sort.Strings(names)

			fmt.Fprintln(out, "Resolved templates:")
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "  NAME\tLAYER\tPATH")
			for _, n := range names {
				s := items[n]
				fmt.Fprintf(tw, "  %s\t%s\t%s\n", n, s.Layer, s.Path)
			}
			return tw.Flush()
		},
	}
}
