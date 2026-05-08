package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/prune"
	"github.com/ystsbry/revu/internal/store"
)

func newPruneCmd() *cobra.Command {
	var (
		repoFlag string
		dryRun   bool
		yes      bool
	)
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete review directories for closed/merged PRs",
		Long: `Sweep ~/.revu/{owner}/{repo}/ and delete pr-N/ directories for PRs whose
GitHub state is CLOSED or MERGED. PRs that are still OPEN are kept. PRs
whose state cannot be determined (deleted on GitHub, network errors, etc.)
are listed in an "errored" section and never deleted.

By default operates on the cwd repository. Pass --repo to target another.
Use --dry-run to preview the plan without touching the filesystem, or --yes
to skip the interactive confirmation prompt.

Reviews that have not been submitted to GitHub (no submitted_at in
review.yml) are still included in the delete plan, but flagged with a
WARNING so you can cancel before losing local-only work.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			slug := repoFlag
			if slug == "" {
				s, err := store.CurrentRepoSlug()
				if err != nil {
					return fmt.Errorf("resolve cwd repo: %w (run inside a git clone or pass --repo)", err)
				}
				slug = s
			}
			repoDir, err := store.RepoDir(slug)
			if err != nil {
				return err
			}

			gh := github.New()
			plan, err := prune.Build(ctx, gh, prune.BuildOptions{Slug: slug, RepoDir: repoDir})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			printPlan(out, plan)

			if dryRun {
				fmt.Fprintln(out, "\n(dry-run: not deleting)")
				return nil
			}
			if len(plan.Delete) == 0 {
				return nil
			}
			if !yes {
				if !confirmYes(cmd.InOrStdin(), out, len(plan.Delete)) {
					fmt.Fprintln(out, "Cancelled.")
					return nil
				}
			}
			if err := prune.Execute(plan); err != nil {
				return err
			}
			fmt.Fprintf(out, "Deleted %d review director%s.\n", len(plan.Delete), pluralYies(len(plan.Delete)))
			return nil
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", `target repo as "owner/repo" (default: cwd's git remote)`)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the plan without deleting")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func printPlan(w io.Writer, plan *prune.Plan) {
	total := len(plan.Delete) + len(plan.Keep) + len(plan.Errored)
	if total == 0 {
		fmt.Fprintf(w, "No reviewed PRs found under %s.\n", plan.RepoDir)
		return
	}
	fmt.Fprintf(w, "Inspected %d PRs under %s (slug=%s)\n\n", total, plan.RepoDir, plan.Slug)

	if len(plan.Delete) > 0 {
		fmt.Fprintf(w, "To delete (%d):\n", len(plan.Delete))
		for _, e := range plan.Delete {
			marker := ""
			if e.HasUnsubmitted {
				marker = "  WARNING: contains unsubmitted reviews"
			}
			fmt.Fprintf(w, "  pr-%-5d %-7s %d SHA dir%s%s\n",
				e.Number, e.State, e.SHADirCount, pluralS(e.SHADirCount), marker)
		}
	}
	if len(plan.Keep) > 0 {
		nums := make([]int, 0, len(plan.Keep))
		for _, e := range plan.Keep {
			nums = append(nums, e.Number)
		}
		sort.Ints(nums)
		fmt.Fprintf(w, "\nSkipped (open, %d): %s\n", len(plan.Keep), formatPRList(nums))
	}
	if len(plan.Errored) > 0 {
		fmt.Fprintf(w, "\nSkipped (state query failed, %d):\n", len(plan.Errored))
		for _, e := range plan.Errored {
			fmt.Fprintf(w, "  pr-%-5d %v\n", e.Number, e.QueryErr)
		}
	}
}

func formatPRList(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprintf("pr-%d", n)
	}
	return strings.Join(parts, ", ")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralYies(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func confirmYes(in io.Reader, out io.Writer, count int) bool {
	fmt.Fprintf(out, "\nDelete %d director%s? [y/N]: ", count, pluralYies(count))
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	resp := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return resp == "y" || resp == "yes"
}

// Compile-time interface satisfaction check: github.GhClient must implement
// prune.StateQuerier so Build can accept it directly.
var _ prune.StateQuerier = (*github.GhClient)(nil)