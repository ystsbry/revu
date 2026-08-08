package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// isolateEnv points the user config and revu home at temp dirs so repo
// commands never touch the real machine state.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("REVU_HOME", t.TempDir())
}

func gitClone(t *testing.T, dir, origin string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", origin}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func runCmd(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, buf.String())
	}
	return buf.String()
}

func TestRepoScanAddListRemoveRoundTrip(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	gitClone(t, filepath.Join(root, "github.com", "alice", "app"), "https://github.com/alice/app.git")
	gitClone(t, filepath.Join(root, "github.com", "bob", "lib"), "git@github.com:bob/lib.git")
	extra := filepath.Join(t.TempDir(), "extra")
	gitClone(t, extra, "https://github.com/carol/extra.git")

	// scan registers both clones under root.
	out := runCmd(t, newRepoCmd(), "scan", root)
	if !strings.Contains(out, "alice/app") || !strings.Contains(out, "bob/lib") {
		t.Fatalf("scan output missing repos:\n%s", out)
	}
	if !strings.Contains(out, "Registered 2 repo(s)") {
		t.Fatalf("scan should register 2 repos:\n%s", out)
	}

	// A second scan is a no-op: everything already registered.
	out = runCmd(t, newRepoCmd(), "scan", root)
	if !strings.Contains(out, "Nothing new to register.") {
		t.Fatalf("re-scan should skip registered repos:\n%s", out)
	}

	// add registers a single clone by path.
	out = runCmd(t, newRepoCmd(), "add", extra)
	if !strings.Contains(out, "Registered carol/extra") {
		t.Fatalf("add output:\n%s", out)
	}

	// list shows all three, in slug order from the merged config.
	out = runCmd(t, newRepoCmd(), "list")
	for _, want := range []string{"alice/app", "bob/lib", "carol/extra"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %s:\n%s", want, out)
		}
	}

	// remove drops one and errors on unknown slugs.
	runCmd(t, newRepoCmd(), "remove", "bob/lib")
	out = runCmd(t, newRepoCmd(), "list")
	if strings.Contains(out, "bob/lib") {
		t.Errorf("removed repo still listed:\n%s", out)
	}

	rm := newRepoCmd()
	rm.SetOut(&bytes.Buffer{})
	rm.SetErr(&bytes.Buffer{})
	rm.SetArgs([]string{"remove", "bob/lib"})
	if err := rm.Execute(); err == nil {
		t.Error("removing an unknown slug should error")
	}
}

func TestRepoScanDryRunWritesNothing(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	gitClone(t, filepath.Join(root, "github.com", "alice", "app"), "https://github.com/alice/app.git")

	out := runCmd(t, newRepoCmd(), "scan", root, "--dry-run")
	if !strings.Contains(out, "Dry run") {
		t.Fatalf("dry-run output:\n%s", out)
	}
	out = runCmd(t, newRepoCmd(), "list")
	if !strings.Contains(out, "No repositories registered") {
		t.Errorf("dry-run must not register anything:\n%s", out)
	}
}

func TestResolveReviewDirArgWithRepoSlug(t *testing.T) {
	isolateEnv(t)
	home := os.Getenv("REVU_HOME")

	// One reviewed PR for o/r under the fake revu home.
	dir := filepath.Join(home, "o", "r", "pr-7", "abc1234")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveReviewDirArg("", "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("resolved %q, want %q", got, dir)
	}

	if _, err := resolveReviewDirArg("/some/dir", "o/r"); err == nil {
		t.Error("[dir] together with --repo should error")
	}
}

func TestStatusWithRepoFlag(t *testing.T) {
	isolateEnv(t)
	home := os.Getenv("REVU_HOME")

	dir := filepath.Join(home, "o", "r", "pr-7", "abc1234")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewYML := `schema_version: 1
pr:
  repo: o/r
  number: 7
  head_sha: abc1234def
  base_branch: main
generated_at: 2026-01-01T00:00:00Z
generated_by:
  tool: claude-code
  skill: revu:pr
  model: x
review_event: COMMENT
summary_file: summary.md
comments: []
`
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte(reviewYML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run from a cwd that is not a git repo at all: --repo must be enough.
	t.Chdir(t.TempDir())

	out := runCmd(t, newStatusCmd(), "--repo", "o/r")
	for _, want := range []string{"Repo:       o/r", "PR:         #7"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}
