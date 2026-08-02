package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ystsbry/revu/internal/claude"
	"github.com/ystsbry/revu/internal/codex"
)

func TestResolveReviewEngine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		useClaude bool
		useCodex  bool
		want      reviewEngine
		wantErr   bool
	}{
		{"neither defaults to claude", false, false, engineClaude, false},
		{"explicit claude", true, false, engineClaude, false},
		{"explicit codex", false, true, engineCodex, false},
		{"both rejected", true, true, "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveReviewEngine(tc.useClaude, tc.useCodex)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("engine = %q, want %q", got, tc.want)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("error should mention mutually exclusive, got %q", err.Error())
			}
		})
	}
}

func TestReviewCmdFlagsRegistered(t *testing.T) {
	t.Parallel()
	cmd := newReviewCmd()
	for _, name := range []string{"claude", "codex", "focus", "no-resume", "json"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("review cmd missing --%s flag", name)
		}
	}
	// Cobra rejects --claude --codex together via MarkFlagsMutuallyExclusive.
	// We verify the annotation is in place by checking the flag set.
	claudeFlag := cmd.Flags().Lookup("claude")
	codexFlag := cmd.Flags().Lookup("codex")
	if claudeFlag.Annotations["cobra_annotation_mutually_exclusive"] == nil {
		t.Errorf("--claude not marked mutually exclusive")
	}
	if codexFlag.Annotations["cobra_annotation_mutually_exclusive"] == nil {
		t.Errorf("--codex not marked mutually exclusive")
	}
	// revu review takes no --repo: the review-pr skill runs in cwd, so a
	// slug flag could only mislead about what gets reviewed.
	if cmd.Flags().Lookup("repo") != nil {
		t.Error("review cmd should not expose --repo while generation is cwd-bound")
	}
}

// --- harness ---------------------------------------------------------------

// reviewFixture creates a review dir holding a minimal review.yml, so the
// generated_by write-back has a real file to patch.
func reviewFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := "pr: 42\ngenerated_by:\n  tool: claude-code\n"
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture review.yml: %v", err)
	}
	return dir
}

type resumeCall struct {
	called    bool
	engine    reviewEngine
	sessionID string
}

// stubDeps builds reviewDeps whose agent calls are fakes. Every field is set
// so a test that reaches an unexpected engine fails loudly instead of
// dereferencing nil.
func stubDeps(t *testing.T, rec *resumeCall) reviewDeps {
	t.Helper()
	fixed := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	return reviewDeps{
		runClaude: func(context.Context, claude.ReviewArgs) (claude.ReviewResult, error) {
			t.Error("claude engine called unexpectedly")
			return claude.ReviewResult{}, nil
		},
		runCodex: func(context.Context, codex.ReviewArgs) (codex.ReviewResult, error) {
			t.Error("codex engine called unexpectedly")
			return codex.ReviewResult{}, nil
		},
		resume: func(_ context.Context, _ io.Writer, engine reviewEngine, sessionID string) error {
			rec.called = true
			rec.engine = engine
			rec.sessionID = sessionID
			return nil
		},
		cwdSlug: func() (string, error) { return "owner/repo", nil },
		now:     func() time.Time { return fixed },
	}
}

// runReviewCmd drives `revu review` through the real root command, so the
// root's SilenceUsage also applies here — that is what keeps cobra's usage
// dump off stdout when a flag combination is rejected, which --json
// consumers depend on.
func runReviewCmd(t *testing.T, deps reviewDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := newRootCmdWith(deps)
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"review"}, args...))
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

// --- non-interactive mode --------------------------------------------------

func TestReviewJSONRequiresNoResume(t *testing.T) {
	t.Parallel()
	rec := &resumeCall{}
	deps := stubDeps(t, rec)

	stdout, _, err := runReviewCmd(t, deps, "--json", "42")
	if err == nil {
		t.Fatal("expected --json without --no-resume to be rejected")
	}
	if !strings.Contains(err.Error(), "--no-resume") {
		t.Fatalf("error should name --no-resume, got %q", err.Error())
	}
	if stdout != "" {
		t.Fatalf("stdout should stay empty on error (no usage dump), got %q", stdout)
	}
	if rec.called {
		t.Error("resume must not run when the flags were rejected")
	}
}

func TestReviewNoResumeRequiresPRNumber(t *testing.T) {
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	// No gh binary reachable: the error must come from the missing
	// argument, not from a failed PR lookup.
	t.Setenv("PATH", t.TempDir())

	_, _, err := runReviewCmd(t, deps, "--no-resume")
	if err == nil {
		t.Fatal("expected an error when PR_NUMBER is omitted in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "PR_NUMBER is required") {
		t.Fatalf("error should say PR_NUMBER is required, got %q", err.Error())
	}
}

func TestReviewNoResumeExitsWithHumanOutput(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	var gotArgs claude.ReviewArgs
	deps.runClaude = func(_ context.Context, args claude.ReviewArgs) (claude.ReviewResult, error) {
		gotArgs = args
		return claude.ReviewResult{OutDir: dir, SessionID: "sess-1"}, nil
	}

	stdout, _, err := runReviewCmd(t, deps, "--no-resume", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.called {
		t.Error("--no-resume must not enter the agent's TUI")
	}
	if !strings.Contains(stdout, "Review generated at "+dir) {
		t.Errorf("stdout should tell the user where the review landed, got %q", stdout)
	}
	if !strings.Contains(stdout, "Generating review for owner/repo#42") {
		t.Errorf("status line should stay on stdout without --json, got %q", stdout)
	}
	if json.Valid([]byte(stdout)) {
		t.Errorf("--no-resume alone should print human-readable text, not JSON: %q", stdout)
	}
	// Nothing may block on the terminal once we've promised to exit, and a
	// run that produced nothing must not resolve to an older review.
	if gotArgs.Stdin == nil {
		t.Error("non-interactive generation should hand the agent an explicit stdin, not os.Stdin")
	}
	if gotArgs.NotBefore.IsZero() {
		t.Error("non-interactive generation should require a freshly written review dir")
	}
	if gotArgs.OwnerRepo != "owner/repo" || gotArgs.PRNumber != 42 {
		t.Errorf("engine args = %+v, want owner/repo#42", gotArgs)
	}
}

func TestReviewJSONPutsOnlyJSONOnStdout(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	deps.runClaude = func(_ context.Context, args claude.ReviewArgs) (claude.ReviewResult, error) {
		// Mimic the real relay writing progress as the agent works.
		_, _ = args.Progress.Write([]byte("  ▸ claude session ready\n"))
		return claude.ReviewResult{OutDir: dir, SessionID: "sess-1"}, nil
	}

	stdout, stderr, err := runReviewCmd(t, deps, "--no-resume", "--json", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.called {
		t.Error("--json must not enter the agent's TUI")
	}

	var got reviewGenResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not a single JSON object (%v): %q", err, stdout)
	}
	want := reviewGenResult{Engine: "claude", Repo: "owner/repo", PR: 42, OutDir: dir, SessionID: "sess-1"}
	if got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}

	// The progress relay and revu's own status lines belong on stderr.
	if !strings.Contains(stderr, "claude session ready") {
		t.Errorf("agent progress should go to stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "Generating review for owner/repo#42") {
		t.Errorf("status line should go to stderr under --json, got %q", stderr)
	}
}

// The JSON field names are what CI steps and background workers read, so
// pin them rather than letting a struct rename break consumers silently.
func TestReviewJSONFieldNames(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := writeReviewResultJSON(&buf, reviewGenResult{
		Engine: "codex", Repo: "owner/repo", PR: 42,
		OutDir: "/home/u/.revu/owner/repo/pr-42/a1b2c3d", SessionID: "thr-9",
	})
	if err != nil {
		t.Fatalf("writeReviewResultJSON: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]any{
		"engine":     "codex",
		"repo":       "owner/repo",
		"pr":         float64(42),
		"out_dir":    "/home/u/.revu/owner/repo/pr-42/a1b2c3d",
		"session_id": "thr-9",
	}
	if len(raw) != len(want) {
		t.Fatalf("field set = %v, want exactly %v", raw, want)
	}
	for k, v := range want {
		if raw[k] != v {
			t.Errorf("%s = %v, want %v", k, raw[k], v)
		}
	}
}

func TestReviewJSONSurvivesWithoutTTY(t *testing.T) {
	dir := reviewFixture(t)
	// Approximate a CI runner: no TERM to render against.
	t.Setenv("TERM", "")

	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	var gotArgs claude.ReviewArgs
	deps.runClaude = func(_ context.Context, args claude.ReviewArgs) (claude.ReviewResult, error) {
		gotArgs = args
		return claude.ReviewResult{OutDir: dir, SessionID: "sess-1"}, nil
	}

	stdout, _, err := runReviewCmd(t, deps, "--no-resume", "--json", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("stdout should still be valid JSON without a TTY, got %q", stdout)
	}
	// The agent must not be handed a terminal that isn't there.
	if gotArgs.Stdin == nil || gotArgs.Stdin == os.Stdin {
		t.Errorf("agent stdin = %v, want an explicit non-terminal reader", gotArgs.Stdin)
	}
}

func TestReviewCodexRecordsGeneratedByWithoutResume(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	deps.runCodex = func(context.Context, codex.ReviewArgs) (codex.ReviewResult, error) {
		return codex.ReviewResult{OutDir: dir, SessionID: "thread-9"}, nil
	}

	stdout, _, err := runReviewCmd(t, deps, "--codex", "--no-resume", "--json", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.called {
		t.Error("--no-resume must not enter the agent's TUI")
	}
	if !strings.Contains(stdout, `"engine": "codex"`) {
		t.Errorf("JSON should report the codex engine, got %q", stdout)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "review.yml"))
	if err != nil {
		t.Fatalf("read review.yml: %v", err)
	}
	// The write-back must happen whether or not we resume: revu resume
	// and the TUI both key off generated_by.
	if !strings.Contains(string(raw), "tool: codex") {
		t.Errorf("generated_by.tool should be rewritten to codex, got:\n%s", raw)
	}
	if !strings.Contains(string(raw), "session_id: thread-9") {
		t.Errorf("generated_by.session_id should be recorded, got:\n%s", raw)
	}
}

func TestReviewClaudeRecordsSessionIDWithoutResume(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	deps.runClaude = func(context.Context, claude.ReviewArgs) (claude.ReviewResult, error) {
		return claude.ReviewResult{OutDir: dir, SessionID: "sess-7"}, nil
	}

	if _, _, err := runReviewCmd(t, deps, "--no-resume", "42"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "review.yml"))
	if err != nil {
		t.Fatalf("read review.yml: %v", err)
	}
	if !strings.Contains(string(raw), "session_id: sess-7") {
		t.Errorf("generated_by.session_id should be recorded, got:\n%s", raw)
	}
	// claude runs leave the skill's own tool value alone.
	if !strings.Contains(string(raw), "tool: claude-code") {
		t.Errorf("generated_by.tool should be untouched for claude, got:\n%s", raw)
	}
}

func TestReviewNonInteractiveFailsWhenGenerationFails(t *testing.T) {
	t.Parallel()
	genErr := errors.New("locate review dir after claude run (claude may have failed silently)")
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	deps.runClaude = func(context.Context, claude.ReviewArgs) (claude.ReviewResult, error) {
		return claude.ReviewResult{}, genErr
	}

	stdout, _, err := runReviewCmd(t, deps, "--no-resume", "--json", "42")
	if !errors.Is(err, genErr) {
		t.Fatalf("err = %v, want the generation failure to propagate (non-zero exit)", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("no result JSON should be printed when generation failed, got %q", stdout)
	}
}

// --- default behaviour ------------------------------------------------------

func TestReviewDefaultStillResumes(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	var gotArgs claude.ReviewArgs
	deps.runClaude = func(_ context.Context, args claude.ReviewArgs) (claude.ReviewResult, error) {
		gotArgs = args
		return claude.ReviewResult{OutDir: dir, SessionID: "sess-1"}, nil
	}

	stdout, _, err := runReviewCmd(t, deps, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.called {
		t.Fatal("the default invocation must still drop into the agent's TUI")
	}
	if rec.engine != engineClaude || rec.sessionID != "sess-1" {
		t.Errorf("resume called with (%q, %q), want (claude, sess-1)", rec.engine, rec.sessionID)
	}
	// Interactive runs keep the agent on the terminal's stdin, and keep
	// accepting a review dir regardless of age, exactly as before.
	if gotArgs.Stdin != nil {
		t.Error("interactive generation should leave Stdin unset so os.Stdin passes through")
	}
	if !gotArgs.NotBefore.IsZero() {
		t.Error("interactive generation should not impose a freshness constraint")
	}
	if !strings.Contains(stdout, "Review generated at "+dir) {
		t.Errorf("stdout should keep the existing status lines, got %q", stdout)
	}
}

// Under --json an absent session id disappears from the object entirely
// (omitempty), so the only place a consumer can learn about it is stderr.
func TestReviewNonInteractiveWarnsWhenNoSessionID(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"json", []string{"--no-resume", "--json", "42"}},
		{"human", []string{"--no-resume", "42"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &resumeCall{}
			deps := stubDeps(t, rec)
			deps.runClaude = func(context.Context, claude.ReviewArgs) (claude.ReviewResult, error) {
				return claude.ReviewResult{OutDir: dir}, nil
			}

			stdout, stderr, err := runReviewCmd(t, deps, tc.args...)
			if err != nil {
				t.Fatalf("a missing session id is not fatal: %v", err)
			}
			if !strings.Contains(stderr, "did not surface a session_id") {
				t.Errorf("stderr should warn that resume is unavailable, got %q", stderr)
			}
			if tc.name != "json" {
				return
			}
			// The key is dropped rather than emitted empty, which is
			// exactly why the warning has to exist.
			var raw map[string]any
			if uerr := json.Unmarshal([]byte(stdout), &raw); uerr != nil {
				t.Fatalf("stdout is not JSON (%v): %q", uerr, stdout)
			}
			if _, ok := raw["session_id"]; ok {
				t.Errorf("session_id should be omitted when empty, got %v", raw)
			}
		})
	}
}

func TestReviewWarnsWhenNoSessionID(t *testing.T) {
	t.Parallel()
	dir := reviewFixture(t)
	rec := &resumeCall{}
	deps := stubDeps(t, rec)
	deps.runCodex = func(context.Context, codex.ReviewArgs) (codex.ReviewResult, error) {
		return codex.ReviewResult{OutDir: dir}, nil
	}

	_, stderr, err := runReviewCmd(t, deps, "--codex", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.called {
		t.Error("resume must not run without a session id")
	}
	// codex calls it a thread_id; the warning should say so.
	if !strings.Contains(stderr, "codex did not surface a thread_id") {
		t.Errorf("warning should name the engine's own identifier, got %q", stderr)
	}
	// The interactive warning is the only one here: the non-interactive
	// variant must not double up on it.
	if n := strings.Count(stderr, "did not surface a thread_id"); n != 1 {
		t.Errorf("warning emitted %d times, want exactly 1: %q", n, stderr)
	}
	if strings.Contains(stderr, "revu resume` is unavailable") {
		t.Errorf("the non-interactive warning should not fire in interactive mode, got %q", stderr)
	}
}
