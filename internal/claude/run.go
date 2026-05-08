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

	"github.com/ystsbry/revu/internal/store"
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
	// output under, i.e. ~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/. Required
	// so the caller can resolve the output dir without re-running gh.
	OwnerRepo string

	// Bin overrides the resolved claude binary path. Empty falls back to
	// "claude" on PATH.
	Bin string
}

// ReviewResult is what RunReviewPR returns on success.
type ReviewResult struct {
	// OutDir is the absolute path to the generated review directory under
	// ~/.revu/, which the caller can hand to the TUI.
	OutDir string
	// SessionID is the claude session ID observed on the system/init
	// event. Empty if the stream did not surface one. Useful for
	// `claude --resume <id>` later.
	SessionID string
}

// RunReviewPR invokes `claude --print "/review-pr <PR>"` in the foreground,
// passing stdin/stdout/stderr through so the user sees claude's progress
// live and Ctrl-C is forwarded.
func RunReviewPR(ctx context.Context, args ReviewArgs) (ReviewResult, error) {
	if args.PRNumber <= 0 {
		return ReviewResult{}, fmt.Errorf("PRNumber must be positive, got %d", args.PRNumber)
	}
	if args.OwnerRepo == "" {
		return ReviewResult{}, errors.New("OwnerRepo is required")
	}

	bin := args.Bin
	if bin == "" {
		bin = "claude"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return ReviewResult{}, ErrCLINotFound
	}

	prompt := "/review-pr " + strconv.Itoa(args.PRNumber)
	if args.Focus != "" {
		prompt += " --focus " + args.Focus
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ReviewResult{}, fmt.Errorf("locate home dir: %w", err)
	}
	// Ensure ~/.revu exists before passing it to `claude --add-dir`: the
	// flag grants sandbox/tool access to that directory, and claude rejects
	// non-existent paths. The skill writes its output under it.
	revuRoot := filepath.Join(home, ".revu")
	if err := os.MkdirAll(revuRoot, 0o755); err != nil {
		return ReviewResult{}, fmt.Errorf("create %s: %w", revuRoot, err)
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
	//
	// `--output-format stream-json --verbose` makes claude emit one JSON
	// event per line as it runs (tool calls, assistant messages, the
	// final result). We pipe that through relayProgress so the user sees
	// what the skill is doing in real time instead of staring at a blank
	// terminal until the review is fully written.
	cmd := exec.CommandContext(ctx, bin,
		"--add-dir", revuRoot,
		"--permission-mode", "acceptEdits",
		"--output-format", "stream-json",
		"--verbose",
		"--print", prompt,
	)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ReviewResult{}, fmt.Errorf("claude --print %q: stdout pipe: %w", prompt, err)
	}
	if err := cmd.Start(); err != nil {
		return ReviewResult{}, fmt.Errorf("claude --print %q: start: %w", prompt, err)
	}
	sessionID, relayErr := relayProgress(stdout, os.Stdout)
	if relayErr != nil {
		// Don't fail the whole run on a broken stream — wait for the
		// process to exit and surface its real status.
		fmt.Fprintf(os.Stderr, "claude stream relay: %v\n", relayErr)
	}
	if err := cmd.Wait(); err != nil {
		return ReviewResult{}, fmt.Errorf("claude --print %q: %w", prompt, err)
	}

	// The skill writes to ~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/. We don't
	// know head_sha here without re-running gh, so discover the SHA dir by
	// picking the most recently written review under pr-{N}/.
	out, err := store.LatestReviewDirForPR(args.OwnerRepo, args.PRNumber)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("locate review dir after claude run (claude may have failed silently): %w", err)
	}
	return ReviewResult{OutDir: out, SessionID: sessionID}, nil
}

// ResumeArgs configures a `claude --resume` invocation.
type ResumeArgs struct {
	SessionID string
	// Bin overrides the resolved claude binary path. Empty falls back to
	// "claude" on PATH.
	Bin string
}

// RunResume execs `claude --resume <id>` in the foreground with stdio
// passthrough so the user drops directly into claude's interactive TUI.
// `~/.revu` is added via --add-dir so the resumed session can still read
// and edit the review files generated by /review-pr.
func RunResume(ctx context.Context, args ResumeArgs) error {
	if args.SessionID == "" {
		return errors.New("SessionID is required")
	}
	bin := args.Bin
	if bin == "" {
		bin = "claude"
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
		"--add-dir", revuRoot,
		"--resume", args.SessionID,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude --resume %s: %w", args.SessionID, err)
	}
	return nil
}

// InstallHint returns the friendly message to show when ErrCLINotFound is hit.
func InstallHint() string {
	return `claude CLI not found on PATH.

Install Claude Code, then ensure ` + "`claude`" + ` is on PATH:

  https://docs.claude.com/en/docs/claude-code

After install, run ` + "`claude --version`" + ` to verify, then retry.`
}
