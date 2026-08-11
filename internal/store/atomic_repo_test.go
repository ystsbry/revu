package store

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// atomicWrite is the only thing standing between a crash and a truncated
// review.yml, so its failure paths matter more than most.
func TestAtomicWriteReplacesTheFileInPlace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yml")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := atomicWrite(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("content = %q, want the new bytes", got)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644 (the requested perm, not the old file's)", st.Mode().Perm())
	}
}

func TestAtomicWriteCreatesAMissingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "review.yml")

	if err := atomicWrite(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file should exist: %v", err)
	}
}

// A temp file left behind would eventually show up as a stray .tmp in the
// user's review directory.
func TestAtomicWriteLeavesNoTempFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := atomicWrite(filepath.Join(dir, "review.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

// The temp file is created next to the target, so a missing directory
// fails before anything is written rather than half-way through.
func TestAtomicWriteFailsWhenTheDirectoryIsMissing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "no-such-dir", "review.yml")

	err := atomicWrite(path, []byte("x"), 0o644)
	if err == nil {
		t.Fatal("writing into a missing directory should fail")
	}
	if !strings.Contains(err.Error(), "create temp") {
		t.Fatalf("error = %q, want it to name the failing step", err)
	}
}

// The original file must survive a failed write — that is the whole point
// of writing to a temp file first.
func TestAtomicWriteKeepsTheOldFileWhenTheDirectoryIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yml")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := atomicWrite(path, []byte("new\n"), 0o644); err == nil {
		t.Fatal("a read-only directory should fail the write")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Fatalf("content = %q, want the original preserved", got)
	}
}

func TestHomeFollowsRevuHome(t *testing.T) {
	t.Setenv("REVU_HOME", "/custom/revu")

	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/custom/revu" {
		t.Fatalf("Home() = %q, want the override", got)
	}
}

func TestHomeDefaultsToDotRevuUnderHome(t *testing.T) {
	t.Setenv("REVU_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".revu"); got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
}

func TestLoadErrorUnwrapsItsCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("root cause")
	err := &LoadError{Path: "review.yml", Field: "comments[0].line", Cause: cause}

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should see through LoadError")
	}
	if !strings.Contains(err.Error(), "comments[0].line") {
		t.Fatalf("Error() = %q, want the field named", err)
	}
}

// initRepo builds a repo with the given origin URL and returns its root.
func initRepo(t *testing.T, origin string) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
		}
	}
	run("init", "-q", "-b", "main")
	if origin != "" {
		run("remote", "add", "origin", origin)
	}
	return root
}

// chdir moves into dir for the duration of the test. These tests cannot be
// parallel: cwd is process-wide, and CwdRepoRoot reads it.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestCwdRepoRoot(t *testing.T) {
	root := initRepo(t, "")
	chdir(t, root)

	got, err := CwdRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	// macOS resolves /var through a symlink, so compare the real paths.
	wantReal, _ := filepath.EvalSymlinks(root)
	gotReal, _ := filepath.EvalSymlinks(got)
	if gotReal != wantReal {
		t.Fatalf("CwdRepoRoot = %q, want %q", gotReal, wantReal)
	}
}

func TestCwdRepoRootOutsideARepository(t *testing.T) {
	dir := t.TempDir() // not a git repo
	chdir(t, dir)

	if _, err := CwdRepoRoot(); err == nil {
		t.Fatal("outside a repository this should fail")
	}
}

func TestCurrentRepoSlug(t *testing.T) {
	chdir(t, initRepo(t, "git@github.com:owner/repo.git"))

	got, err := CurrentRepoSlug()
	if err != nil {
		t.Fatal(err)
	}
	if got != "owner/repo" {
		t.Fatalf("CurrentRepoSlug = %q, want owner/repo", got)
	}
}

func TestCurrentRepoSlugWithoutAnOrigin(t *testing.T) {
	chdir(t, initRepo(t, ""))

	if _, err := CurrentRepoSlug(); err == nil {
		t.Fatal("a repo with no origin should fail")
	}
}

// VerifyRepoMatches guards `revu open` against being run in the wrong
// tree — opening a review against a mismatched checkout would render
// comments at meaningless line numbers.
func TestVerifyRepoMatches(t *testing.T) {
	root := initRepo(t, "https://github.com/owner/repo.git")
	chdir(t, root)

	got, err := VerifyRepoMatches("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	gotReal, _ := filepath.EvalSymlinks(got)
	wantReal, _ := filepath.EvalSymlinks(root)
	if gotReal != wantReal {
		t.Fatalf("VerifyRepoMatches = %q, want the repo root %q", gotReal, wantReal)
	}
}

func TestVerifyRepoMatchesRejectsAMismatch(t *testing.T) {
	chdir(t, initRepo(t, "https://github.com/owner/repo.git"))

	_, err := VerifyRepoMatches("other/project")
	if err == nil {
		t.Fatal("a different repo should be rejected")
	}
	// Both sides belong in the message: the user needs to know which tree
	// they are in and which one the review wants.
	if !strings.Contains(err.Error(), "owner/repo") || !strings.Contains(err.Error(), "other/project") {
		t.Fatalf("error = %q, want both slugs named", err)
	}
}

func TestVerifyRepoMatchesOutsideARepository(t *testing.T) {
	chdir(t, t.TempDir())

	if _, err := VerifyRepoMatches("owner/repo"); err == nil {
		t.Fatal("outside a repository this should fail")
	}
}

func TestVerifyRepoMatchesWithoutAnOrigin(t *testing.T) {
	chdir(t, initRepo(t, ""))

	_, err := VerifyRepoMatches("owner/repo")
	if err == nil {
		t.Fatal("a repo with no origin should fail")
	}
	if !strings.Contains(err.Error(), "git remote") {
		t.Fatalf("error = %q, want it to blame the remote lookup", err)
	}
}

// With no argument the review dir is resolved from cwd's repo: this is the
// path `revu open` takes when the user just runs it inside a clone.
func TestResolveReviewDirFromTheCurrentRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)
	chdir(t, initRepo(t, "https://github.com/owner/repo.git"))

	want := filepath.Join(home, "owner", "repo", "pr-4", "abc1234")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(want, "review.yml"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveReviewDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ResolveReviewDir(\"\") = %q, want %q", got, want)
	}
}

func TestResolveReviewDirOutsideARepository(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	chdir(t, t.TempDir())

	_, err := ResolveReviewDir("")
	if err == nil || !strings.Contains(err.Error(), "auto-resolve") {
		t.Fatalf("error = %v, want an auto-resolve complaint", err)
	}
}

func TestRepoDirRejectsAMalformedSlug(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())

	for _, slug := range []string{"", "no-slash", "a/b/c"} {
		if _, err := RepoDir(slug); err == nil {
			t.Errorf("RepoDir(%q) should fail", slug)
		}
	}
}

func TestPRDirRejectsANonPositiveNumber(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())

	for _, n := range []int{0, -1} {
		if _, err := PRDir("o/r", n); err == nil {
			t.Errorf("PRDir(o/r, %d) should fail", n)
		}
	}
}
