package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/claude"
	"github.com/ystsbry/revu/internal/codex"
	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/store"
	"github.com/ystsbry/revu/internal/tui/picker"
)

// reviewEngine names the runtime that drives the /review-pr skill.
type reviewEngine string

const (
	engineClaude reviewEngine = "claude"
	engineCodex  reviewEngine = "codex"
)

// resolveReviewEngine picks an engine from the mutually exclusive
// --claude / --codex flags. Default is claude for backward compatibility.
func resolveReviewEngine(useClaude, useCodex bool) (reviewEngine, error) {
	if useClaude && useCodex {
		return "", errors.New("--claude and --codex are mutually exclusive")
	}
	if useCodex {
		return engineCodex, nil
	}
	return engineClaude, nil
}

func newReviewCmd() *cobra.Command {
	var (
		focus     string
		useClaude bool
		useCodex  bool
	)
	cmd := &cobra.Command{
		Use:   "review [PR_NUMBER]",
		Short: "Generate a review for a PR and drop into the agent's interactive TUI",
		Long: `Generate a review via the review-pr skill, then resume the same agent
session interactively so you can iterate on the review.

The skill is driven by either the local "claude" CLI (default) or the
local "codex" CLI when --codex is given. In both cases the skill itself
(skills/review-pr/SKILL.md) is the single source of truth — only the
runtime that loads it differs.

Without an argument, fetches PRs awaiting your review (gh's
"review-requested:@me" search) in the cwd's repository and shows a picker.

With an argument, treats it as the PR number and skips the picker.

When generation finishes, revu records the agent's session id in
review.yml's generated_by section and execs ` + "`claude --resume <id>`" + ` (or
` + "`codex resume <id>`" + `) so you continue in the agent's TUI. To revisit
the review later, run "revu open" (revu TUI) or "revu resume" (agent TUI).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			engine, err := resolveReviewEngine(useClaude, useCodex)
			if err != nil {
				return err
			}

			slug, err := store.CurrentRepoSlug()
			if err != nil {
				return fmt.Errorf("resolve cwd repo: %w (run revu review inside a git clone)", err)
			}

			prNumber, err := resolvePRNumber(ctx, cmd, args, slug)
			if err != nil || prNumber == 0 {
				return err
			}

			return runReview(ctx, cmd, engine, slug, prNumber, focus)
		},
	}
	cmd.Flags().StringVar(&focus, "focus", "", "categories to focus on, passed through to /review-pr (e.g. \"security,perf\")")
	cmd.Flags().BoolVar(&useClaude, "claude", false, "drive the review-pr skill via the claude CLI (default if neither flag is set)")
	cmd.Flags().BoolVar(&useCodex, "codex", false, "drive the review-pr skill via the codex CLI instead of claude")
	cmd.MarkFlagsMutuallyExclusive("claude", "codex")
	return cmd
}

// resolvePRNumber returns the PR number either from args[0] or from the
// interactive picker. Returns (0, nil) when the user cancels the picker
// or no PRs are awaiting review — caller should treat that as a clean exit.
func resolvePRNumber(ctx context.Context, cmd *cobra.Command, args []string, slug string) (int, error) {
	if len(args) == 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("PR_NUMBER must be a positive integer, got %q", args[0])
		}
		return n, nil
	}
	gh := github.New()
	items, err := gh.ListReviewRequestedPRs(ctx)
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No open PRs are awaiting your review in this repository.")
		return 0, nil
	}
	picked, err := picker.Pick(items)
	if err != nil {
		return 0, err
	}
	if picked == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
		return 0, nil
	}
	return picked.Number, nil
}

func runReview(ctx context.Context, cmd *cobra.Command, engine reviewEngine, slug string, prNumber int, focus string) error {
	switch engine {
	case engineCodex:
		return runReviewCodex(ctx, cmd, slug, prNumber, focus)
	default:
		return runReviewClaude(ctx, cmd, slug, prNumber, focus)
	}
}

func runReviewClaude(ctx context.Context, cmd *cobra.Command, slug string, prNumber int, focus string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Generating review for %s#%d via claude --print /review-pr ...\n\n", slug, prNumber)

	result, err := claude.RunReviewPR(ctx, claude.ReviewArgs{
		PRNumber:  prNumber,
		Focus:     focus,
		OwnerRepo: slug,
	})
	if err != nil {
		if errors.Is(err, claude.ErrCLINotFound) {
			fmt.Fprintln(cmd.ErrOrStderr(), claude.InstallHint())
		}
		return err
	}

	if err := store.SaveSessionID(result.OutDir, result.SessionID); err != nil {
		// Non-fatal: review files are already written; resume just
		// won't be available for this PR until you re-review.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record session_id in review.yml: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nReview generated at %s\n", result.OutDir)
	if result.SessionID == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: claude did not surface a session_id; cannot drop into the interactive TUI. Run `revu open` to inspect the review instead.")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Resuming claude session %s ...\n\n", result.SessionID)
	return claude.RunResume(ctx, claude.ResumeArgs{SessionID: result.SessionID})
}

func runReviewCodex(ctx context.Context, cmd *cobra.Command, slug string, prNumber int, focus string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Generating review for %s#%d via codex exec $review-pr ...\n\n", slug, prNumber)

	result, err := codex.RunReviewPR(ctx, codex.ReviewArgs{
		PRNumber:  prNumber,
		Focus:     focus,
		OwnerRepo: slug,
	})
	if err != nil {
		if errors.Is(err, codex.ErrCLINotFound) {
			fmt.Fprintln(cmd.ErrOrStderr(), codex.InstallHint())
		}
		return err
	}

	// The skill always writes `tool: claude-code` (it was originally a
	// Claude-only skill). Rewrite generated_by so revu resume and other
	// downstream consumers can tell the run was actually driven by codex
	// and pick the right resume command. session_id moves from the
	// Claude session shape to the codex thread_id captured by the
	// stream parser.
	patch := store.GeneratedByPatch{
		Tool:      "codex",
		SessionID: result.SessionID,
	}
	if err := store.SaveGeneratedBy(result.OutDir, patch); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record codex generated_by in review.yml: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nReview generated at %s\n", result.OutDir)
	if result.SessionID == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: codex did not surface a thread_id; cannot drop into the interactive TUI. Run `revu open` to inspect the review instead.")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Resuming codex session %s ...\n\n", result.SessionID)
	return codex.RunResume(ctx, codex.ResumeArgs{SessionID: result.SessionID})
}
