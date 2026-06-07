// Package codex wraps the `codex` CLI (OpenAI Codex CLI) so revu can invoke
// the review-pr skill from its own commands, as an alternative to the
// `claude` runtime.
//
// Like internal/claude, we shell out rather than re-implementing the skill:
// skills/review-pr/SKILL.md is the same source of truth, but loaded by
// Codex's skill loader (~/.agents/skills/review-pr — see scripts/install-codex.sh)
// at the user scope.
package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/ystsbry/revu/internal/store"
)

// ErrCLINotFound is returned when the `codex` executable is not on PATH.
var ErrCLINotFound = errors.New("codex CLI not found on PATH")

// ReviewArgs mirrors claude.ReviewArgs so the two engines can be swapped at
// the call site without rewriting the surrounding plumbing.
type ReviewArgs struct {
	PRNumber  int
	Focus     string
	OwnerRepo string

	// Bin overrides the resolved codex binary. Empty falls back to "codex".
	Bin string
}

// ReviewResult mirrors claude.ReviewResult.
type ReviewResult struct {
	OutDir    string
	SessionID string
}

// RunReviewPR invokes `codex exec --json "$review-pr <PR>"` in the
// foreground, relaying progress to stdout and capturing the thread_id so
// the caller can later `codex resume <id>`.
//
// The `$review-pr` prefix is Codex's skill-invocation syntax (see
// scripts/install-codex.sh) — it resolves to skills/review-pr/SKILL.md
// installed under ~/.agents/skills/review-pr.
func RunReviewPR(ctx context.Context, args ReviewArgs) (ReviewResult, error) {
	if args.PRNumber <= 0 {
		return ReviewResult{}, fmt.Errorf("PRNumber must be positive, got %d", args.PRNumber)
	}
	if args.OwnerRepo == "" {
		return ReviewResult{}, errors.New("OwnerRepo is required")
	}

	bin := args.Bin
	if bin == "" {
		bin = "codex"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return ReviewResult{}, ErrCLINotFound
	}

	prompt := "$review-pr " + strconv.Itoa(args.PRNumber)
	if args.Focus != "" {
		prompt += " --focus " + args.Focus
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ReviewResult{}, fmt.Errorf("locate home dir: %w", err)
	}
	revuRoot := filepath.Join(home, ".revu")
	if err := os.MkdirAll(revuRoot, 0o755); err != nil {
		return ReviewResult{}, fmt.Errorf("create %s: %w", revuRoot, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ReviewResult{}, fmt.Errorf("getwd: %w", err)
	}

	// `--sandbox workspace-write` lets the skill write its review files;
	// `--add-dir ~/.revu` extends the writable set to the revu output
	// dir, which lives outside the repo. `codex exec` is already
	// non-interactive, so there's no approval flag to pass.
	//
	// `--json` makes codex emit one event per line on stdout (see
	// stream.go for the event shapes we recognise).
	//
	// All of --cd / --sandbox / --add-dir / --json are options of the
	// `exec` subcommand (not the top-level `codex`), so they go after
	// `exec` in the argv.
	cmd := exec.CommandContext(ctx, bin,
		"exec",
		"--cd", cwd,
		"--sandbox", "workspace-write",
		"--add-dir", revuRoot,
		"--json",
		prompt,
	)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ReviewResult{}, fmt.Errorf("codex exec %q: stdout pipe: %w", prompt, err)
	}
	if err := cmd.Start(); err != nil {
		return ReviewResult{}, fmt.Errorf("codex exec %q: start: %w", prompt, err)
	}
	sessionID, relayErr := relayProgress(stdout, os.Stdout)
	if relayErr != nil {
		fmt.Fprintf(os.Stderr, "codex stream relay: %v\n", relayErr)
	}
	if err := cmd.Wait(); err != nil {
		return ReviewResult{}, fmt.Errorf("codex exec %q: %w", prompt, err)
	}

	out, err := store.LatestReviewDirForPR(args.OwnerRepo, args.PRNumber)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("locate review dir after codex run (codex may have failed silently): %w", err)
	}
	return ReviewResult{OutDir: out, SessionID: sessionID}, nil
}

// ResumeArgs mirrors claude.ResumeArgs.
type ResumeArgs struct {
	SessionID string
	Bin       string
}

// RunResume execs `codex resume <id>` in the foreground with stdio
// passthrough so the user drops into Codex's interactive TUI.
func RunResume(ctx context.Context, args ResumeArgs) error {
	if args.SessionID == "" {
		return errors.New("SessionID is required")
	}
	bin := args.Bin
	if bin == "" {
		bin = "codex"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return ErrCLINotFound
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home dir: %w", err)
	}
	revuRoot := filepath.Join(home, ".revu")
	if err := os.MkdirAll(revuRoot, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", revuRoot, err)
	}

	cmd := exec.CommandContext(ctx, bin,
		"resume",
		"--add-dir", revuRoot,
		args.SessionID,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex resume %s: %w", args.SessionID, err)
	}
	return nil
}

// InstallHint returns the friendly message to surface when ErrCLINotFound
// fires for codex.
func InstallHint() string {
	return `codex CLI not found on PATH.

Install OpenAI Codex CLI, then ensure ` + "`codex`" + ` is on PATH:

  https://developers.openai.com/codex/cli

After install, run ` + "`codex --version`" + ` to verify. To make the
review-pr skill available to codex, run:

  scripts/install-codex.sh

from the revu repo, then restart codex.`
}
