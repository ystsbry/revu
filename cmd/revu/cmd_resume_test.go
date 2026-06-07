package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/claude"
	"github.com/ystsbry/revu/internal/codex"
)

// withEmptyPATH points $PATH at an empty directory so exec.LookPath fails
// for both claude and codex. That forces resumeAgent down the
// ErrCLINotFound branch — which is the only branch we can exercise in a
// unit test without actually launching an interactive TUI.
func withEmptyPATH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	// Best-effort guard against an absolute claude/codex left around from
	// a previous test cycle.
	_ = os.Unsetenv("CLAUDE_BIN")
	_ = os.Unsetenv("CODEX_BIN")
}

func TestResumeAgentDispatchesByTool(t *testing.T) {
	withEmptyPATH(t)

	cases := []struct {
		name     string
		tool     string
		wantErr  error
		wantHint string
	}{
		{"codex tool dispatches to codex", "codex", codex.ErrCLINotFound, "codex CLI not found"},
		{"empty tool falls back to claude", "", claude.ErrCLINotFound, "claude CLI not found"},
		{"claude-code falls through to claude", "claude-code", claude.ErrCLINotFound, "claude CLI not found"},
		{"unknown tool falls through to claude", "future-runtime", claude.ErrCLINotFound, "claude CLI not found"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := resumeAgent(context.Background(), &stdout, &stderr, tc.tool, "deadbeef", "owner/repo", 42)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.wantErr)
			}
			if !strings.Contains(stderr.String(), tc.wantHint) {
				t.Errorf("stderr should mention %q, got %q", tc.wantHint, stderr.String())
			}
			// stdout always announces the resume target so the user
			// sees which engine was chosen.
			if !strings.Contains(stdout.String(), "owner/repo#42") {
				t.Errorf("stdout should mention pr ref, got %q", stdout.String())
			}
		})
	}
}
