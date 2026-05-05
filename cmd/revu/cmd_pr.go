package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/github"
)

// newPRCmd groups PR-related helpers used by the review-pr skill so the
// permission allowlist needs only `Bash(revu *)` instead of multiple
// `Bash(gh *)` and `Bash(mkdir *)` entries.
func newPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "PR helpers used by the review-pr skill",
	}
	cmd.AddCommand(newPRPrepareCmd())
	cmd.AddCommand(newPRDiffCmd())
	cmd.AddCommand(newPRListMineCmd())
	return cmd
}

// prPrepareOutput is the JSON the skill consumes after `revu pr prepare`.
// Field names match review.yml so the skill can copy values 1:1.
type prPrepareOutput struct {
	Repo       string `json:"repo"`
	Number     int    `json:"number"`
	HeadSha    string `json:"head_sha"`
	BaseBranch string `json:"base_branch"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	ReviewDir  string `json:"review_dir"`
}

func newPRPrepareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prepare <PR_NUMBER>",
		Short: "Fetch PR metadata, create the review output dir, and print everything as JSON",
		Long: `Fetch PR metadata via gh, create ~/.revu/{owner}/{repo}/pr-{N}/comments/,
and print a JSON object the review-pr skill can consume directly.

Replaces the skill's previous "gh pr view" + "mkdir -p" steps with one
revu call so the permission allowlist needs only Bash(revu *).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return fmt.Errorf("PR_NUMBER must be a positive integer, got %q", args[0])
			}
			gh := github.New()
			meta, err := gh.PRMeta(cmd.Context(), n)
			if err != nil {
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("locate home dir: %w", err)
			}
			reviewDir := filepath.Join(home, ".revu", meta.BaseRepo, fmt.Sprintf("pr-%d", n))
			if err := os.MkdirAll(filepath.Join(reviewDir, "comments"), 0o755); err != nil {
				return fmt.Errorf("create review dir: %w", err)
			}
			out := prPrepareOutput{
				Repo:       meta.BaseRepo,
				Number:     n,
				HeadSha:    meta.HeadSha,
				BaseBranch: meta.BaseBranch,
				Title:      meta.Title,
				Body:       meta.Body,
				ReviewDir:  reviewDir,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}

func newPRDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <PR_NUMBER>",
		Short: "Print the PR's unified diff",
		Long:  "Wraps `gh pr diff <PR_NUMBER>` so the skill needs only Bash(revu *).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return fmt.Errorf("PR_NUMBER must be a positive integer, got %q", args[0])
			}
			gh := github.New()
			diff, err := gh.PRDiff(cmd.Context(), n)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), diff)
			return err
		},
	}
}

func newPRListMineCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list-mine",
		Short: "List open PRs in the cwd repo awaiting your review",
		Long: `Wraps "gh pr list --search review-requested:@me" with the JSON fields
revu uses internally. Use --json for machine output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			gh := github.New()
			items, err := gh.ListReviewRequestedPRs(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(items)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No open PRs are awaiting your review.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NUMBER\tAUTHOR\tBRANCH\tTITLE")
			for _, it := range items {
				fmt.Fprintf(w, "#%d\t@%s\t%s → %s\t%s\n",
					it.Number, it.Author.Login, it.HeadRefName, it.BaseRefName, it.Title)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON for machine consumption")
	return cmd
}
