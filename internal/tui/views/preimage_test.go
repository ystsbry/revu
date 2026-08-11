package views

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initPreImageRepo builds a throwaway repo with one commit on main and one
// on a feature branch, mirroring what a review is generated against:
// baseRef is "main" and headSHA is the feature commit.
func initPreImageRepo(t *testing.T) (root, headSHA string) {
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

	run("checkout", "-q", "-b", "feature")
	write("foo.go", "package foo\n\nfunc New() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "rename Old to New")

	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	// The working tree is left on the feature branch, then dirtied, so the
	// tests prove the source reads commits rather than the working copy.
	write("foo.go", "package foo\n\nfunc Uncommitted() {}\n")
	return root, strings.TrimSpace(string(out))
}

func TestGitPreImageContentReadsTheBaseCommit(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	raw, err := src.Content("foo.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "func Old()") {
		t.Fatalf("pre-image = %q, want the base-commit version", raw)
	}
	if strings.Contains(string(raw), "Uncommitted") {
		t.Fatal("pre-image must not read the working tree")
	}
}

// PostImage reads the head commit, so RIGHT-side line numbers stay aligned
// even when the working tree has drifted.
func TestGitPreImagePostImageReadsTheHeadCommit(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	raw, err := src.PostImage("foo.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "func New()") {
		t.Fatalf("post-image = %q, want the head-commit version", raw)
	}
	if strings.Contains(string(raw), "Uncommitted") {
		t.Fatal("post-image must not read the working tree")
	}
}

func TestGitPreImageDiffSpansBaseToHead(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	raw, err := src.Diff("foo.go")
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, "-func Old() {}") || !strings.Contains(out, "+func New() {}") {
		t.Fatalf("diff = %q, want both sides of the change", out)
	}
}

// Results are cached per path: the detail view queries the same file
// repeatedly as the user moves between comments.
func TestGitPreImageCachesPerPath(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	first, err := src.Content("foo.go")
	if err != nil {
		t.Fatal(err)
	}
	// Deleting the repo makes any further git call fail; a cache hit is
	// the only way the second read can succeed.
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	second, err := src.Content("foo.go")
	if err != nil {
		t.Fatalf("second read should come from the cache: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("cached content differs from the first read")
	}
}

func TestGitPreImagePostImageCachesPerPath(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	if _, err := src.PostImage("foo.go"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := src.PostImage("foo.go"); err != nil {
		t.Fatalf("second read should come from the cache: %v", err)
	}
}

func TestGitPreImageDiffCachesPerPath(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	if _, err := src.Diff("foo.go"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Diff("foo.go"); err != nil {
		t.Fatalf("second read should come from the cache: %v", err)
	}
}

// Missing configuration has to name what is missing: these errors surface
// in the detail view's code pane, where the user can act on them.
func TestGitPreImageReportsMissingConfiguration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		root     string
		head     string
		baseRef  string
		wantWord string
	}{
		{name: "no repo root", root: "", head: "abc", baseRef: "main", wantWord: "repo root"},
		{name: "no head SHA", root: "/tmp", head: "", baseRef: "main", wantWord: "head SHA"},
		{name: "no base branch", root: "/tmp", head: "abc", baseRef: "", wantWord: "base branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := NewGitPreImage(tt.root, tt.head, tt.baseRef)

			_, err := src.Content("foo.go")
			if err == nil {
				t.Fatal("Content should fail without the base commit")
			}
			if !strings.Contains(err.Error(), tt.wantWord) {
				t.Fatalf("error = %q, want it to mention %q", err, tt.wantWord)
			}
		})
	}
}

// PostImage does not need the base, so it has its own guards.
func TestGitPreImagePostImageGuards(t *testing.T) {
	t.Parallel()

	if _, err := NewGitPreImage("", "abc", "main").PostImage("foo.go"); err == nil ||
		!strings.Contains(err.Error(), "repo root") {
		t.Fatalf("error = %v, want a repo-root complaint", err)
	}
	if _, err := NewGitPreImage("/tmp", "", "main").PostImage("foo.go"); err == nil ||
		!strings.Contains(err.Error(), "head SHA") {
		t.Fatalf("error = %v, want a head-SHA complaint", err)
	}
}

// The base is resolved once; a failure is remembered rather than
// re-spawning git for every comment in the review.
func TestGitPreImageResolvesTheBaseOnlyOnce(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "does-not-exist")

	first := errString(src.Content("foo.go"))
	if first == "" {
		t.Fatal("an unknown base ref should fail")
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if second := errString(src.Content("other.go")); second != first {
		t.Fatalf("second error = %q, want the remembered %q", second, first)
	}
}

func errString(_ []byte, err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// A path that does not exist in the base commit is an error the code pane
// renders, not a panic.
func TestGitPreImageUnknownPath(t *testing.T) {
	t.Parallel()
	root, head := initPreImageRepo(t)
	src := NewGitPreImage(root, head, "main")

	if _, err := src.Content("nope.go"); err == nil {
		t.Fatal("an unknown path should fail")
	}
}
