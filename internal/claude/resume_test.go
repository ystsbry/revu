package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunResumeRequiresASessionID(t *testing.T) {
	t.Parallel()
	err := RunResume(context.Background(), ResumeArgs{})
	if err == nil || !strings.Contains(err.Error(), "SessionID") {
		t.Fatalf("error = %v, want a missing-SessionID complaint", err)
	}
}

// A missing CLI must surface as ErrCLINotFound so callers can show
// InstallHint instead of a raw exec error.
func TestRunResumeReportsAMissingCLI(t *testing.T) {
	t.Parallel()
	err := RunResume(context.Background(), ResumeArgs{
		SessionID: "abc",
		Bin:       "revu-no-such-claude",
	})
	if !errors.Is(err, ErrCLINotFound) {
		t.Fatalf("error = %v, want ErrCLINotFound", err)
	}
}

// The resumed session has to be able to reach ~/.revu, so the directory is
// created before claude starts rather than left to fail mid-session.
func TestRunResumeCreatesTheRevuRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// `true` accepts and ignores the argv, so this exercises the whole
	// function without needing claude installed.
	if err := RunResume(context.Background(), ResumeArgs{SessionID: "abc", Bin: "true"}); err != nil {
		t.Fatalf("RunResume = %v, want nil", err)
	}

	st, err := os.Stat(filepath.Join(home, ".revu"))
	if err != nil {
		t.Fatalf("~/.revu should exist: %v", err)
	}
	if !st.IsDir() {
		t.Fatal("~/.revu should be a directory")
	}
}

// A non-zero exit from claude is reported with the session it was for.
func TestRunResumeWrapsAFailingExit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := RunResume(context.Background(), ResumeArgs{SessionID: "abc", Bin: "false"})
	if err == nil {
		t.Fatal("a non-zero exit should be an error")
	}
	if !strings.Contains(err.Error(), "abc") {
		t.Fatalf("error = %q, want the session id in it", err)
	}
}

func TestInstallHint(t *testing.T) {
	t.Parallel()
	hint := InstallHint()

	for _, want := range []string{"claude", "PATH", "docs.claude.com"} {
		if !strings.Contains(hint, want) {
			t.Errorf("InstallHint() is missing %q:\n%s", want, hint)
		}
	}
}
