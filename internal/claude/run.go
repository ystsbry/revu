// Package claude wraps the `claude` CLI so revu can invoke the review-pr
// skill from its own commands.
//
// We shell out to `claude` rather than re-implementing the skill in Go: the
// skill already lives in skills/review-pr/SKILL.md and Claude Code is the
// authoritative runtime for it.
package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// ErrCLINotFound is returned when the `claude` executable is not on PATH.
// Callers surface a friendly install hint to the user.
var ErrCLINotFound = errors.New("claude CLI not found on PATH")

// ReviewArgs configures one invocation of the review-pr skill.
type ReviewArgs struct {
	// PRNumber is the GitHub PR number. Required.
	PRNumber int

	// Focus is an optional comma-separated category list passed through
	// to the skill (e.g. "security,perf"). Empty means all categories.
	Focus string

	// OwnerRepo is the GitHub slug ("owner/repo") the skill will write its
	// output under, i.e. ~/.revu/{owner}/{repo}/pr-{N}/. Required so the
	// caller can resolve the output dir without re-running gh.
	OwnerRepo string

	// Bin overrides the resolved claude binary path. Empty falls back to
	// "claude" on PATH.
	Bin string
}

// RunReviewPR invokes `claude --print "/review-pr <PR>"` in the foreground,
// passing stdin/stdout/stderr through so the user sees claude's progress
// live and Ctrl-C is forwarded.
//
// Returns the absolute path to the generated review directory under
// ~/.revu/, which the caller can hand to the existing TUI.
func RunReviewPR(ctx context.Context, args ReviewArgs) (string, error) {
	if args.PRNumber <= 0 {
		return "", fmt.Errorf("PRNumber must be positive, got %d", args.PRNumber)
	}
	if args.OwnerRepo == "" {
		return "", errors.New("OwnerRepo is required")
	}

	bin := args.Bin
	if bin == "" {
		bin = "claude"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return "", ErrCLINotFound
	}

	prompt := "/review-pr " + strconv.Itoa(args.PRNumber)
	if args.Focus != "" {
		prompt += " --focus " + args.Focus
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	// Ensure ~/.revu exists before passing it to `claude --add-dir`: the
	// flag grants sandbox/tool access to that directory, and claude rejects
	// non-existent paths. The skill writes its output under it.
	revuRoot := filepath.Join(home, ".revu")
	if err := os.MkdirAll(revuRoot, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", revuRoot, err)
	}

	// Argument order matters: `--add-dir <directories...>` is variadic and
	// will swallow the prompt if placed right before it. Put it before
	// `--print` so the next `-` flag terminates the variadic capture and
	// the prompt remains a clean trailing positional.
	//
	// `--permission-mode acceptEdits` is required: in non-interactive
	// `--print` mode, write/edit operations cannot prompt the user, so
	// they are blocked by default. acceptEdits auto-approves them so the
	// skill can write its review files to ~/.revu/.
	cmd := exec.CommandContext(ctx, bin,
		"--add-dir", revuRoot,
		"--permission-mode", "acceptEdits",
		"--print", prompt,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude --print %q: %w", prompt, err)
	}

	out := filepath.Join(revuRoot, args.OwnerRepo, "pr-"+strconv.Itoa(args.PRNumber))
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("expected review at %s but it was not created (claude may have failed silently): %w", out, err)
	}
	return out, nil
}

// InstallHint returns the friendly message to show when ErrCLINotFound is hit.
func InstallHint() string {
	return `claude CLI not found on PATH.

Install Claude Code, then ensure ` + "`claude`" + ` is on PATH:

  https://docs.claude.com/en/docs/claude-code

After install, run ` + "`claude --version`" + ` to verify, then retry.`
}
