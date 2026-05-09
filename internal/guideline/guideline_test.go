package guideline

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo bootstraps a real git repo at a temp path and chdirs into it.
// config.RepoRoot uses `git rev-parse --show-toplevel`, which needs a real
// .git/.
func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	abs, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = abs
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(abs); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return abs
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupIsolatedEnv(t *testing.T) (xdg, root string) {
	t.Helper()
	t.Setenv("REVU_CONFIG", "")
	xdg = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	root = initRepo(t)
	return xdg, root
}

func TestListConcatenatesAcrossLayers(t *testing.T) {
	xdg, root := setupIsolatedEnv(t)

	// User layer: one guideline (relative to ~/.config/revu/config.toml).
	writeFile(t, filepath.Join(xdg, "revu", "config.toml"), `
[review]
guidelines = ["guidelines/style.md"]
`)
	// User-side guideline file exists.
	userGL := filepath.Join(xdg, "revu", "guidelines", "style.md")
	writeFile(t, userGL, "# Style\n")

	// Project-shared layer: another guideline, relative to its config.
	writeFile(t, filepath.Join(root, ".revu", "config.toml"), `
[review]
guidelines = ["docs/security.md"]
`)
	sharedGL := filepath.Join(root, "docs", "security.md")
	// Note: docs/ is relative to .revu/config.toml, which sits in
	// <root>/.revu/. So the resolved path is <root>/.revu/docs/security.md.
	resolvedSharedGL := filepath.Join(root, ".revu", "docs", "security.md")
	_ = sharedGL
	writeFile(t, resolvedSharedGL, "# Security\n")

	// Per-clone local layer: third guideline, missing on disk.
	writeFile(t, filepath.Join(root, ".revu-local", "config.toml"), `
[review]
guidelines = ["my-rules.md"]
`)
	missingGL := filepath.Join(root, ".revu-local", "my-rules.md")

	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len(List)=%d want 3 (got %+v)", len(got), got)
	}
	wantPaths := []string{userGL, resolvedSharedGL, missingGL}
	wantExists := []bool{true, true, false}
	for i, r := range got {
		if r.Path != wantPaths[i] {
			t.Errorf("[%d] Path=%q want %q", i, r.Path, wantPaths[i])
		}
		if r.Exists != wantExists[i] {
			t.Errorf("[%d] Exists=%v want %v", i, r.Exists, wantExists[i])
		}
	}
}

func TestPathsFiltersMissing(t *testing.T) {
	xdg, _ := setupIsolatedEnv(t)
	writeFile(t, filepath.Join(xdg, "revu", "config.toml"), `
[review]
guidelines = ["a.md", "b.md"]
`)
	writeFile(t, filepath.Join(xdg, "revu", "a.md"), "a")
	// b.md is intentionally absent.

	got, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Paths)=%d want 1 (missing should be filtered): %v", len(got), got)
	}
	if filepath.Base(got[0]) != "a.md" {
		t.Errorf("got %q, want a.md", got[0])
	}
}

func TestListEmptyWhenNoConfig(t *testing.T) {
	setupIsolatedEnv(t)
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("len(List)=%d want 0 (no config)", len(got))
	}
}

func TestListAcceptsAbsolutePaths(t *testing.T) {
	xdg, _ := setupIsolatedEnv(t)
	tmp := t.TempDir()
	abs := filepath.Join(tmp, "absolute.md")
	writeFile(t, abs, "abs")

	writeFile(t, filepath.Join(xdg, "revu", "config.toml"), `
[review]
guidelines = ["`+abs+`"]
`)
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != abs || !got[0].Exists {
		t.Errorf("got %+v, want one resolved abs entry", got)
	}
}

func TestListDedupesAcrossLayers(t *testing.T) {
	xdg, root := setupIsolatedEnv(t)
	tmp := t.TempDir()
	abs := filepath.Join(tmp, "shared.md")
	writeFile(t, abs, "x")

	writeFile(t, filepath.Join(xdg, "revu", "config.toml"), `
[review]
guidelines = ["`+abs+`"]
`)
	writeFile(t, filepath.Join(root, ".revu", "config.toml"), `
[review]
guidelines = ["`+abs+`"]
`)
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("dedupe failed: %+v", got)
	}
}
