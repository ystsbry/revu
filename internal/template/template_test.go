package template

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo bootstraps a real git repo at a temp path and chdirs into it.
// Required because the underlying config.LayerDirs uses
// `git rev-parse --show-toplevel`.
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

func writeTmpl(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func setupIsolatedEnv(t *testing.T) (xdg, root string) {
	t.Helper()
	t.Setenv("REVU_CONFIG", "")
	t.Setenv("REVU_TEMPLATES", "")
	xdg = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	root = initRepo(t)
	return xdg, root
}

func TestResolveLocalWinsOverShared(t *testing.T) {
	xdg, root := setupIsolatedEnv(t)
	writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "summary.md.tmpl", "user\n")
	writeTmpl(t, filepath.Join(root, ".revu", "templates"), "summary.md.tmpl", "shared\n")
	wantPath := writeTmpl(t, filepath.Join(root, ".revu-local", "templates"), "summary.md.tmpl", "local\n")

	got, err := Resolve("summary.md.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPath {
		t.Errorf("Resolve = %q want %q", got, wantPath)
	}
}

func TestResolveSharedWinsOverUser(t *testing.T) {
	xdg, root := setupIsolatedEnv(t)
	writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "summary.md.tmpl", "user\n")
	wantPath := writeTmpl(t, filepath.Join(root, ".revu", "templates"), "summary.md.tmpl", "shared\n")

	got, err := Resolve("summary.md.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPath {
		t.Errorf("Resolve = %q want %q", got, wantPath)
	}
}

func TestResolveFallsBackToUser(t *testing.T) {
	xdg, _ := setupIsolatedEnv(t)
	wantPath := writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "summary.md.tmpl", "user\n")

	got, err := Resolve("summary.md.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPath {
		t.Errorf("Resolve = %q want %q", got, wantPath)
	}
}

func TestResolveEnvDirInsertedAboveUser(t *testing.T) {
	xdg, _ := setupIsolatedEnv(t)
	writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "summary.md.tmpl", "user\n")
	envDir := t.TempDir()
	t.Setenv("REVU_TEMPLATES", envDir)
	wantPath := writeTmpl(t, envDir, "summary.md.tmpl", "env\n")

	got, err := Resolve("summary.md.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPath {
		t.Errorf("Resolve = %q want %q", got, wantPath)
	}
}

func TestResolveRepoLocalBeatsEnv(t *testing.T) {
	xdg, root := setupIsolatedEnv(t)
	writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "summary.md.tmpl", "user\n")
	envDir := t.TempDir()
	t.Setenv("REVU_TEMPLATES", envDir)
	writeTmpl(t, envDir, "summary.md.tmpl", "env\n")
	wantPath := writeTmpl(t, filepath.Join(root, ".revu-local", "templates"), "summary.md.tmpl", "local\n")

	got, err := Resolve("summary.md.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPath {
		t.Errorf("Resolve = %q want %q", got, wantPath)
	}
}

func TestResolveNotFound(t *testing.T) {
	setupIsolatedEnv(t)
	_, err := Resolve("nope.md.tmpl")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveRejectsBadName(t *testing.T) {
	setupIsolatedEnv(t)
	for _, name := range []string{"", "../escape", "sub/dir.md.tmpl"} {
		if _, err := Resolve(name); err == nil {
			t.Errorf("Resolve(%q) should error", name)
		}
	}
}

func TestListReportsWinningLayer(t *testing.T) {
	xdg, root := setupIsolatedEnv(t)
	writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "summary.md.tmpl", "u-summary")
	writeTmpl(t, filepath.Join(xdg, "revu", "templates"), "shared-only.md.tmpl", "u-shared")
	writeTmpl(t, filepath.Join(root, ".revu", "templates"), "summary.md.tmpl", "s-summary")
	writeTmpl(t, filepath.Join(root, ".revu-local", "templates"), "inline.md.tmpl", "l-inline")

	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	wantLayers := map[string]string{
		"summary.md.tmpl":     "repo-shared",
		"shared-only.md.tmpl": "user",
		"inline.md.tmpl":      "repo-local",
	}
	for name, layer := range wantLayers {
		s, ok := got[name]
		if !ok {
			t.Errorf("%s missing from List", name)
			continue
		}
		if s.Layer != layer {
			t.Errorf("%s.Layer = %q want %q", name, s.Layer, layer)
		}
	}
}
