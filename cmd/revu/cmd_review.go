package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

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

// reviewDeps are the outward-facing calls `revu review` makes. They are
// injected rather than referenced directly so tests can drive the command
// without launching an agent CLI — and without mutating package state,
// which would make the tests unsafe to run in parallel.
type reviewDeps struct {
	runClaude func(context.Context, claude.ReviewArgs) (claude.ReviewResult, error)
	runCodex  func(context.Context, codex.ReviewArgs) (codex.ReviewResult, error)
	resume    func(ctx context.Context, out io.Writer, engine reviewEngine, sessionID string) error
	cwdSlug   func() (string, error)
	now       func() time.Time
}

func defaultReviewDeps() reviewDeps {
	return reviewDeps{
		runClaude: claude.RunReviewPR,
		runCodex:  codex.RunReviewPR,
		resume:    resumeReviewSession,
		cwdSlug:   store.CurrentRepoSlug,
		now:       time.Now,
	}
}

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

func newReviewCmd() *cobra.Command { return newReviewCmdWith(defaultReviewDeps()) }

func newReviewCmdWith(deps reviewDeps) *cobra.Command {
	var (
		focus     string
		useClaude bool
		useCodex  bool
		noResume  bool
		asJSON    bool
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
the review later, run "revu open" (revu TUI) or "revu resume" (agent TUI).

Non-interactive mode (CI, background workers):

  --no-resume exits once the review is generated instead of dropping into
  the agent's TUI, and prints where the review landed. PR_NUMBER is
  required in this mode — the interactive picker is never started, since
  nothing could answer it.

  --json (which requires --no-resume) prints the result as a JSON object
  on stdout: {engine, repo, pr, out_dir, session_id}. The agent's progress
  relay and revu's own status lines move to stderr so stdout stays
  parseable. Note that session_id holds codex's thread_id under --codex,
  matching how review.yml records it.

  Either way revu exits non-zero if the run produced no review, and the
  agent is given an empty stdin so it cannot block on a terminal that a
  CI runner does not have.

The repository is always resolved from the cwd git remote: the review-pr
skill runs in cwd, so CI must invoke revu from inside the checkout.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			engine, err := resolveReviewEngine(useClaude, useCodex)
			if err != nil {
				return err
			}
			// Emitting a result object and then handing the terminal
			// to the agent's TUI would leave the caller parsing a
			// stdout it no longer owns, so --json is confined to the
			// non-interactive path rather than implying it.
			if asJSON && !noResume {
				return errors.New("--json requires --no-resume")
			}

			slug, err := deps.cwdSlug()
			if err != nil {
				return fmt.Errorf("resolve cwd repo: %w (run revu review inside a git clone)", err)
			}

			prNumber, err := resolvePRNumber(ctx, cmd, args, slug, noResume)
			if err != nil || prNumber == 0 {
				return err
			}

			return deps.runReview(ctx, cmd, reviewOptions{
				Engine:   engine,
				Slug:     slug,
				PRNumber: prNumber,
				Focus:    focus,
				NoResume: noResume,
				AsJSON:   asJSON,
			})
		},
	}
	cmd.Flags().StringVar(&focus, "focus", "", "categories to focus on, passed through to /review-pr (e.g. \"security,perf\")")
	cmd.Flags().BoolVar(&useClaude, "claude", false, "drive the review-pr skill via the claude CLI (default if neither flag is set)")
	cmd.Flags().BoolVar(&useCodex, "codex", false, "drive the review-pr skill via the codex CLI instead of claude")
	cmd.Flags().BoolVar(&noResume, "no-resume", false, "exit after generating the review instead of entering the agent's TUI (requires PR_NUMBER)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON on stdout, with progress on stderr (requires --no-resume)")
	cmd.MarkFlagsMutuallyExclusive("claude", "codex")
	return cmd
}

// resolvePRNumber returns the PR number either from args[0] or from the
// interactive picker. Returns (0, nil) when the user cancels the picker
// or no PRs are awaiting review — caller should treat that as a clean exit.
//
// noPicker disables the picker: in non-interactive mode a missing PR number
// has to be an error, because falling through to a TUI would hang a CI job
// or a detached worker on a prompt nobody can answer.
func resolvePRNumber(ctx context.Context, cmd *cobra.Command, args []string, slug string, noPicker bool) (int, error) {
	if len(args) == 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("PR_NUMBER must be a positive integer, got %q", args[0])
		}
		return n, nil
	}
	if noPicker {
		return 0, errors.New("PR_NUMBER is required with --no-resume (the interactive picker is disabled in non-interactive mode)")
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

// reviewOptions is one resolved `revu review` invocation.
type reviewOptions struct {
	Engine   reviewEngine
	Slug     string
	PRNumber int
	Focus    string
	NoResume bool
	AsJSON   bool
}

func (deps reviewDeps) runReview(ctx context.Context, cmd *cobra.Command, opts reviewOptions) error {
	// With --json, stdout belongs to the result object alone: the agent's
	// progress relay and revu's own status lines go to stderr so the
	// caller can parse stdout without filtering it first.
	progress := cmd.OutOrStdout()
	if opts.AsJSON {
		progress = cmd.ErrOrStderr()
	}

	gen := reviewGenOptions{
		Engine:   opts.Engine,
		Slug:     opts.Slug,
		PRNumber: opts.PRNumber,
		Focus:    opts.Focus,
		Progress: progress,
		Warn:     cmd.ErrOrStderr(),
	}
	if opts.NoResume {
		// Once we've promised to exit after generation, nothing may
		// block on input: hand the agent an empty stdin rather than
		// the terminal's, which may not even exist.
		gen.Stdin = strings.NewReader("")
		// And a run that wrote nothing must fail rather than hand back
		// whatever the previous run left behind — nobody is watching.
		gen.NotBefore = deps.now()
	}

	res, err := deps.generateReview(ctx, gen)
	if err != nil {
		return err
	}

	if opts.AsJSON {
		return writeReviewResultJSON(cmd.OutOrStdout(), res)
	}

	fmt.Fprintf(progress, "\nReview generated at %s\n", res.OutDir)
	if opts.NoResume {
		return nil
	}
	if res.SessionID == "" {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: %s did not surface a %s; cannot drop into the interactive TUI. Run `revu open` to inspect the review instead.\n",
			opts.Engine, sessionIDLabel(opts.Engine))
		return nil
	}
	return deps.resume(ctx, cmd.OutOrStdout(), opts.Engine, res.SessionID)
}

// sessionIDLabel names the engine's session identifier the way that engine
// does, so warnings match what the user would look for in review.yml.
func sessionIDLabel(engine reviewEngine) string {
	if engine == engineCodex {
		return "thread_id"
	}
	return "session_id"
}

// reviewGenOptions is the input to one generation run. It carries its own
// writers rather than a *cobra.Command so callers can redirect progress
// without touching the command's stdout.
type reviewGenOptions struct {
	Engine   reviewEngine
	Slug     string
	PRNumber int
	Focus    string

	// Progress receives status lines and the agent's progress relay.
	Progress io.Writer
	// Warn receives non-fatal warnings and install hints.
	Warn io.Writer
	// Stdin is handed to the agent process. nil means os.Stdin.
	Stdin io.Reader
	// NotBefore rejects a review dir that predates this run. Zero accepts
	// the newest review dir regardless of age.
	NotBefore time.Time
}

// reviewGenResult describes one generated review. It doubles as the --json
// payload, so its field names are a compatibility surface: consumers such
// as CI steps and background workers read out_dir and session_id from it.
type reviewGenResult struct {
	Engine    string `json:"engine"`
	Repo      string `json:"repo"`
	PR        int    `json:"pr"`
	OutDir    string `json:"out_dir"`
	SessionID string `json:"session_id,omitempty"`
}

// generateReview runs the review-pr skill and writes the agent's identity
// back into review.yml's generated_by. It never resumes: whether to drop
// into the agent's TUI is the caller's decision.
func (deps reviewDeps) generateReview(ctx context.Context, opts reviewGenOptions) (reviewGenResult, error) {
	switch opts.Engine {
	case engineCodex:
		return deps.generateReviewCodex(ctx, opts)
	default:
		return deps.generateReviewClaude(ctx, opts)
	}
}

func (deps reviewDeps) generateReviewClaude(ctx context.Context, opts reviewGenOptions) (reviewGenResult, error) {
	fmt.Fprintf(opts.Progress, "Generating review for %s#%d via claude --print /review-pr ...\n\n", opts.Slug, opts.PRNumber)

	result, err := deps.runClaude(ctx, claude.ReviewArgs{
		PRNumber:  opts.PRNumber,
		Focus:     opts.Focus,
		OwnerRepo: opts.Slug,
		Progress:  opts.Progress,
		Stdin:     opts.Stdin,
		NotBefore: opts.NotBefore,
	})
	if err != nil {
		if errors.Is(err, claude.ErrCLINotFound) {
			fmt.Fprintln(opts.Warn, claude.InstallHint())
		}
		return reviewGenResult{}, err
	}

	if err := store.SaveSessionID(result.OutDir, result.SessionID); err != nil {
		// Non-fatal: review files are already written; resume just
		// won't be available for this PR until you re-review.
		fmt.Fprintf(opts.Warn, "warning: could not record session_id in review.yml: %v\n", err)
	}

	return reviewGenResult{
		Engine:    string(engineClaude),
		Repo:      opts.Slug,
		PR:        opts.PRNumber,
		OutDir:    result.OutDir,
		SessionID: result.SessionID,
	}, nil
}

func (deps reviewDeps) generateReviewCodex(ctx context.Context, opts reviewGenOptions) (reviewGenResult, error) {
	fmt.Fprintf(opts.Progress, "Generating review for %s#%d via codex exec $review-pr ...\n\n", opts.Slug, opts.PRNumber)

	result, err := deps.runCodex(ctx, codex.ReviewArgs{
		PRNumber:  opts.PRNumber,
		Focus:     opts.Focus,
		OwnerRepo: opts.Slug,
		Progress:  opts.Progress,
		Stdin:     opts.Stdin,
		NotBefore: opts.NotBefore,
	})
	if err != nil {
		if errors.Is(err, codex.ErrCLINotFound) {
			fmt.Fprintln(opts.Warn, codex.InstallHint())
		}
		return reviewGenResult{}, err
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
		fmt.Fprintf(opts.Warn, "warning: could not record codex generated_by in review.yml: %v\n", err)
	}

	return reviewGenResult{
		Engine:    string(engineCodex),
		Repo:      opts.Slug,
		PR:        opts.PRNumber,
		OutDir:    result.OutDir,
		SessionID: result.SessionID,
	}, nil
}

func writeReviewResultJSON(w io.Writer, res reviewGenResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// resumeReviewSession execs the engine's resume command, handing the
// terminal to the agent's interactive TUI.
func resumeReviewSession(ctx context.Context, out io.Writer, engine reviewEngine, sessionID string) error {
	fmt.Fprintf(out, "Resuming %s session %s ...\n\n", engine, sessionID)
	if engine == engineCodex {
		return codex.RunResume(ctx, codex.ResumeArgs{SessionID: sessionID})
	}
	return claude.RunResume(ctx, claude.ResumeArgs{SessionID: sessionID})
}
