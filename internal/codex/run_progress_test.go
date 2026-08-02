package codex

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

// fakeCodex stands in for the codex CLI: it drains stdin into $STDIN_LOG,
// writes a review.yml into $OUT_DIR, and emits two --json events.
func fakeCodex(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI is a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "fake-codex")
	script := `#!/bin/sh
cat > "$STDIN_LOG"
mkdir -p "$OUT_DIR"
printf 'pr: 7\ngenerated_by:\n  tool: claude-code\n' > "$OUT_DIR/review.yml"
printf '%s\n' '{"type":"thread.started","thread_id":"thr-fake"}'
printf '%s\n' '{"type":"turn.completed"}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	return path
}

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
	bin := fakeCodex(t)
	outDir, stdinLog := setupFakeRun(t)

	realStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

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
	if res.SessionID != "thr-fake" {
		t.Errorf("SessionID = %q, want thr-fake", res.SessionID)
	}
	if !strings.Contains(progress.String(), "codex session ready") {
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
	bin := fakeCodex(t)
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

	bin := filepath.Join(t.TempDir(), "fake-codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
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
