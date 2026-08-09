package claude

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The zero value of ReviewArgs has to keep behaving like the original
// hard-coded os.Stdout / os.Stdin, or every interactive caller silently
// loses its progress output.
func TestResolveProgressAndStdinDefaults(t *testing.T) {
	t.Parallel()
	if got := resolveProgress(nil); got != os.Stdout {
		t.Errorf("resolveProgress(nil) = %v, want os.Stdout", got)
	}
	var buf bytes.Buffer
	if got := resolveProgress(&buf); got != &buf {
		t.Error("resolveProgress(w) should return w unchanged")
	}
	if got := resolveStdin(nil); got != os.Stdin {
		t.Errorf("resolveStdin(nil) = %v, want os.Stdin", got)
	}
	r := strings.NewReader("")
	if got := resolveStdin(r); got != r {
		t.Error("resolveStdin(r) should return r unchanged")
	}
}

// fakeClaude writes a shell script that stands in for the claude CLI: it
// drains stdin into $STDIN_LOG, writes a review.yml into $OUT_DIR, and
// emits two stream-json events. Returns its path.
func fakeClaude(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI is a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "fake-claude")
	script := `#!/bin/sh
cat > "$STDIN_LOG"
mkdir -p "$OUT_DIR"
printf 'pr: 7\ngenerated_by:\n  tool: claude-code\n' > "$OUT_DIR/review.yml"
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-fake"}'
printf '%s\n' '{"type":"result","duration_ms":1000,"num_turns":1}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}

// setupFakeRun points REVU_HOME at a temp tree and tells the fake CLI where
// to write. Returns the review dir the fake will create and the stdin log.
func setupFakeRun(t *testing.T) (outDir, stdinLog string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("REVU_HOME", root)
	outDir = filepath.Join(root, "owner", "repo", "pr-7", "abc1234")
	stdinLog = filepath.Join(root, "stdin.log")
	t.Setenv("OUT_DIR", outDir)
	t.Setenv("STDIN_LOG", stdinLog)
	return outDir, stdinLog
}

// TestRunReviewPRRoutesProgressAndStdin exercises the real subprocess path:
// resolver unit tests alone cannot catch RunReviewPR forgetting to use them.
func TestRunReviewPRRoutesProgressAndStdin(t *testing.T) {
	bin := fakeClaude(t)
	outDir, stdinLog := setupFakeRun(t)

	// Capture the process-wide stdout so we can prove the relay did not
	// leak onto it when Progress is set.
	realStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = realStdout }()

	var progress bytes.Buffer
	res, err := RunReviewPR(context.Background(), ReviewArgs{
		PRNumber:  7,
		OwnerRepo: "owner/repo",
		Bin:       bin,
		Progress:  &progress,
		Stdin:     strings.NewReader("hello from the caller\n"),
	})
	_ = w.Close()
	os.Stdout = realStdout

	var leaked bytes.Buffer
	if _, cerr := leaked.ReadFrom(r); cerr != nil {
		t.Fatalf("read captured stdout: %v", cerr)
	}
	if err != nil {
		t.Fatalf("RunReviewPR: %v", err)
	}

	if res.OutDir != outDir {
		t.Errorf("OutDir = %q, want %q", res.OutDir, outDir)
	}
	if res.SessionID != "sess-fake" {
		t.Errorf("SessionID = %q, want sess-fake", res.SessionID)
	}
	if !strings.Contains(progress.String(), "claude session ready") {
		t.Errorf("progress writer did not receive the relay, got %q", progress.String())
	}
	if leaked.Len() != 0 {
		t.Errorf("relay leaked onto os.Stdout: %q", leaked.String())
	}

	got, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatalf("read stdin log: %v", err)
	}
	if strings.TrimSpace(string(got)) != "hello from the caller" {
		t.Errorf("child stdin = %q, want the reader we passed", string(got))
	}
}

// An empty reader must reach the child as an immediate EOF: a
// non-interactive run may have no terminal to block on.
func TestRunReviewPREmptyStdinIsImmediateEOF(t *testing.T) {
	bin := fakeClaude(t)
	_, stdinLog := setupFakeRun(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var progress bytes.Buffer
	if _, err := RunReviewPR(ctx, ReviewArgs{
		PRNumber:  7,
		OwnerRepo: "owner/repo",
		Bin:       bin,
		Progress:  &progress,
		Stdin:     strings.NewReader(""),
	}); err != nil {
		t.Fatalf("RunReviewPR: %v", err)
	}

	got, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatalf("read stdin log: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("child stdin = %q, want empty", string(got))
	}
}

// A run that writes nothing must fail rather than hand back the previous
// run's review.
func TestRunReviewPRRejectsStaleReviewDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI is a POSIX shell script")
	}
	root := t.TempDir()
	t.Setenv("REVU_HOME", root)
	stale := filepath.Join(root, "owner", "repo", "pr-7", "old1234")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stale, "review.yml"), []byte("pr: 7\n"), 0o644); err != nil {
		t.Fatalf("write stale review.yml: %v", err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(filepath.Join(stale, "review.yml"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// A CLI that produces no review at all.
	bin := filepath.Join(t.TempDir(), "fake-claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	var progress bytes.Buffer
	_, err := RunReviewPR(context.Background(), ReviewArgs{
		PRNumber:  7,
		OwnerRepo: "owner/repo",
		Bin:       bin,
		Progress:  &progress,
		Stdin:     strings.NewReader(""),
		NotBefore: time.Now(),
	})
	if err == nil {
		t.Fatal("expected a stale review dir to be rejected")
	}
	if !strings.Contains(err.Error(), "no review was generated for this run") {
		t.Errorf("error should explain the review is stale, got %q", err.Error())
	}
}

// Without NotBefore the original behaviour is preserved: the newest review
// dir is returned regardless of its age.
func TestRunReviewPRWithoutNotBeforeAcceptsExistingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI is a POSIX shell script")
	}
	root := t.TempDir()
	t.Setenv("REVU_HOME", root)
	dir := filepath.Join(root, "owner", "repo", "pr-7", "old1234")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte("pr: 7\n"), 0o644); err != nil {
		t.Fatalf("write review.yml: %v", err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "review.yml"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	bin := filepath.Join(t.TempDir(), "fake-claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	var progress bytes.Buffer
	res, err := RunReviewPR(context.Background(), ReviewArgs{
		PRNumber:  7,
		OwnerRepo: "owner/repo",
		Bin:       bin,
		Progress:  &progress,
		Stdin:     strings.NewReader(""),
	})
	if err != nil {
		t.Fatalf("RunReviewPR: %v", err)
	}
	if res.OutDir != dir {
		t.Errorf("OutDir = %q, want %q", res.OutDir, dir)
	}
}

// The --model flag appears only when a model was picked, right before the
// flags whose order the printing pipeline depends on.
func TestBuildPrintArgsModelOverride(t *testing.T) {
	args := buildPrintArgs("/revu:pr 42", "/home/u/.revu", "claude-sonnet-5")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--model claude-sonnet-5") {
		t.Fatalf("model flag missing: %v", args)
	}
	if args[len(args)-1] != "/revu:pr 42" {
		t.Fatalf("prompt must stay the trailing positional: %v", args)
	}

	joined = strings.Join(buildPrintArgs("/revu:pr 42", "/home/u/.revu", ""), " ")
	if strings.Contains(joined, "--model") {
		t.Fatalf("no model picked: --model must be absent: %v", joined)
	}
}
