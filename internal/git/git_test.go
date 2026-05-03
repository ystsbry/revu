package git

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a minimal git repo under t.TempDir() with two commits on
// "main" and a feature branch that diverges, so tests can exercise both
// pre-image lookups and merge-base resolution.
func initRepo(t *testing.T) (root, baseSHA, headSHA string) {
	t.Helper()
	root = t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "commit.gpgsign", "false")
	write("foo.go", "package foo\n\nfunc Old() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "initial")

	// Capture the base SHA before branching.
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	baseSHA = strings.TrimSpace(string(out))

	// Diverge on a feature branch with a modified file.
	run("checkout", "-q", "-b", "feature")
	write("foo.go", "package foo\n\nfunc New() {}\n")
	run("commit", "-q", "-am", "rename")
	out, err = exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	headSHA = strings.TrimSpace(string(out))

	return root, baseSHA, headSHA
}

func TestShowReturnsPreImage(t *testing.T) {
	root, baseSHA, _ := initRepo(t)

	got, err := Show(root, baseSHA, "foo.go")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !strings.Contains(string(got), "func Old()") {
		t.Errorf("expected pre-image to contain Old(), got:\n%s", got)
	}
	if strings.Contains(string(got), "func New()") {
		t.Errorf("pre-image should not contain New():\n%s", got)
	}
}

func TestShowMissingPath(t *testing.T) {
	root, baseSHA, _ := initRepo(t)
	if _, err := Show(root, baseSHA, "does-not-exist.go"); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestShowEmptyArgs(t *testing.T) {
	if _, err := Show("", "ref", "p"); err == nil {
		t.Error("empty repoRoot should error")
	}
	if _, err := Show("/tmp", "", "p"); err == nil {
		t.Error("empty ref should error")
	}
	if _, err := Show("/tmp", "ref", ""); err == nil {
		t.Error("empty path should error")
	}
}

func TestMergeBaseResolves(t *testing.T) {
	root, baseSHA, headSHA := initRepo(t)

	got, err := MergeBase(root, "main", headSHA)
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if got != baseSHA {
		t.Errorf("MergeBase = %s, want %s", got, baseSHA)
	}
}

func TestDiffReturnsUnifiedDiff(t *testing.T) {
	root, baseSHA, headSHA := initRepo(t)

	got, err := Diff(root, baseSHA, headSHA, "foo.go", 3)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	out := string(got)
	for _, want := range []string{"@@", "-func Old()", "+func New()"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in diff:\n%s", want, out)
		}
	}
}

func TestDiffEmptyArgs(t *testing.T) {
	if _, err := Diff("", "a", "b", "p", 3); err == nil {
		t.Error("empty repoRoot should error")
	}
	if _, err := Diff("/tmp", "", "b", "p", 3); err == nil {
		t.Error("empty baseRef should error")
	}
	if _, err := Diff("/tmp", "a", "", "p", 3); err == nil {
		t.Error("empty headRef should error")
	}
	if _, err := Diff("/tmp", "a", "b", "", 3); err == nil {
		t.Error("empty path should error")
	}
}

func TestMergeBaseEmptyArgs(t *testing.T) {
	if _, err := MergeBase("", "a", "b"); err == nil {
		t.Error("empty repoRoot should error")
	}
	if _, err := MergeBase("/tmp", "", "b"); err == nil {
		t.Error("empty a should error")
	}
}
