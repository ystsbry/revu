package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// mkClone creates a minimal git repo at dir with the given origin URL
// (empty = no origin remote).
func mkClone(t *testing.T, dir, origin string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if origin != "" {
		run("remote", "add", "origin", origin)
	}
}

func TestDetectSlug(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clone")
	mkClone(t, dir, "git@github.com:ystsbry/revu.git")

	slug, err := DetectSlug(dir)
	if err != nil {
		t.Fatal(err)
	}
	if slug != "ystsbry/revu" {
		t.Errorf("slug = %q, want ystsbry/revu", slug)
	}

	if _, err := DetectSlug(t.TempDir()); err == nil {
		t.Error("non-repo dir should error")
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()

	// ghq-style layout: root/host/owner/repo.
	mkClone(t, filepath.Join(root, "github.com", "alice", "app"), "https://github.com/alice/app.git")
	mkClone(t, filepath.Join(root, "github.com", "bob", "lib"), "ssh://git@github.com/bob/lib.git")
	// A repo without an origin is reported as skipped, not fatal.
	mkClone(t, filepath.Join(root, "github.com", "carol", "no-origin"), "")
	// Plain directories are traversed silently.
	if err := os.MkdirAll(filepath.Join(root, "not-a-repo", "stuff"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A nested clone below a found clone must not be reported.
	mkClone(t, filepath.Join(root, "github.com", "alice", "app", "vendor", "dep"), "https://github.com/x/dep.git")

	res, err := Scan(root, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Found) != 2 {
		t.Fatalf("found = %+v, want alice/app + bob/lib", res.Found)
	}
	if res.Found[0].Slug != "alice/app" || res.Found[1].Slug != "bob/lib" {
		t.Errorf("slugs = %q, %q", res.Found[0].Slug, res.Found[1].Slug)
	}
	if want := filepath.Join(root, "github.com", "alice", "app"); res.Found[0].Path != want {
		t.Errorf("path = %q, want %q", res.Found[0].Path, want)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Path != filepath.Join(root, "github.com", "carol", "no-origin") {
		t.Errorf("skipped = %+v, want the origin-less repo", res.Skipped)
	}
}

func TestScanRespectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	mkClone(t, filepath.Join(root, "a", "b", "c", "d", "deep"), "https://github.com/deep/deep.git")

	res, err := Scan(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 0 {
		t.Errorf("depth-2 scan should not reach a depth-5 clone, found %+v", res.Found)
	}

	res, err = Scan(root, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 {
		t.Errorf("depth-5 scan should find the clone, found %+v", res.Found)
	}
}

func TestScanDedupesSameSlug(t *testing.T) {
	root := t.TempDir()
	mkClone(t, filepath.Join(root, "a-first"), "https://github.com/o/r.git")
	mkClone(t, filepath.Join(root, "b-second"), "https://github.com/o/r.git")

	res, err := Scan(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 {
		t.Fatalf("found = %+v, want 1 after dedupe", res.Found)
	}
	if want := filepath.Join(root, "a-first"); res.Found[0].Path != want {
		t.Errorf("dedupe kept %q, want first-walked %q", res.Found[0].Path, want)
	}
}
