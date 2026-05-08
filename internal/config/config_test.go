package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	t.Parallel()
	c := Defaults()
	if c.UI.CodeContextLines != 5 {
		t.Errorf("CodeContextLines = %d, want 5", c.UI.CodeContextLines)
	}
	if c.UI.HorizontalThreshold != 100 {
		t.Errorf("HorizontalThreshold = %d, want 100", c.UI.HorizontalThreshold)
	}
	if c.Review.DefaultEvent != "COMMENT" {
		t.Errorf("DefaultEvent = %q", c.Review.DefaultEvent)
	}
	if c.Editor.Command != "" {
		t.Errorf("Editor.Command should be empty, got %q", c.Editor.Command)
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("REVU_CONFIG", filepath.Join(tmp, "absent.toml"))

	cfg, sources, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(sources) != 1 || sources[0].Loaded {
		t.Errorf("expected one not-loaded source, got %+v", sources)
	}
	if cfg.UI.CodeContextLines != 5 {
		t.Errorf("missing file should yield defaults; got %+v", cfg)
	}
}

func TestLoadOverridesAndMerge(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := `
[editor]
command = "code --wait"

[ui]
code_context_lines = 10

[review]
default_event = "REQUEST_CHANGES"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	cfg, sources, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(sources) != 1 || !sources[0].Loaded || sources[0].Path != path {
		t.Errorf("sources = %+v", sources)
	}
	if cfg.Editor.Command != "code --wait" {
		t.Errorf("Editor.Command = %q", cfg.Editor.Command)
	}
	if cfg.UI.CodeContextLines != 10 {
		t.Errorf("CodeContextLines = %d", cfg.UI.CodeContextLines)
	}
	// Unset key falls back to default.
	if cfg.UI.HorizontalThreshold != 100 {
		t.Errorf("HorizontalThreshold should default; got %d", cfg.UI.HorizontalThreshold)
	}
	if cfg.Review.DefaultEvent != "REQUEST_CHANGES" {
		t.Errorf("DefaultEvent = %q", cfg.Review.DefaultEvent)
	}
}

func TestLoadInvalidEvent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(path, []byte(`[review]
default_event = "BLOCK"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	_, _, err := Load()
	if err == nil || !strings.Contains(err.Error(), "default_event") {
		t.Errorf("err = %v, want default_event error", err)
	}
}

func TestLoadMalformedTOML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(path, []byte("this is not toml ="), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	_, _, err := Load()
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestSourcesHonorsEnv(t *testing.T) {
	t.Setenv("REVU_CONFIG", "/tmp/custom/revu.toml")
	got, err := Sources()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/tmp/custom/revu.toml" {
		t.Errorf("Sources = %v, want exactly the env path", got)
	}
}

func TestSampleTOMLParses(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(path, []byte(SampleTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	if _, _, err := Load(); err != nil {
		t.Errorf("SampleTOML failed to parse: %v", err)
	}
}

func TestDefaultsIncludesBuiltInSeverities(t *testing.T) {
	t.Parallel()
	c := Defaults()
	names := make([]string, 0, len(c.Review.Severities))
	for _, s := range c.Review.Severities {
		names = append(names, s.Name)
	}
	want := []string{"critical", "major", "minor", "nit"}
	if len(names) != len(want) {
		t.Fatalf("default severities = %v, want %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("severities[%d] = %q, want %q", i, names[i], n)
		}
	}
}

func TestLoadCustomSeveritiesReplaces(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := `
[[review.severity]]
name = "critical"
level = 100
description = "critical"
review_event = "REQUEST_CHANGES"
color = "red"

[[review.severity]]
name = "suggestion"
level = 40
description = "suggestion"
review_event = "COMMENT"
color = "cyan"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(cfg.Review.Severities); got != 2 {
		t.Fatalf("severities len = %d, want 2 (built-ins must be replaced, not merged)", got)
	}
	if cfg.Review.Severities[1].Name != "suggestion" {
		t.Errorf("severities[1].Name = %q, want suggestion", cfg.Review.Severities[1].Name)
	}
}

func TestLoadInvalidSeverityReviewEvent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(path, []byte(`[[review.severity]]
name = "bad"
level = 1
review_event = "BOGUS"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	_, _, err := Load()
	if err == nil || !strings.Contains(err.Error(), "review_event") {
		t.Errorf("err = %v, want review_event error", err)
	}
}

func TestLoadDuplicateSeverityName(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(path, []byte(`[[review.severity]]
name = "x"
level = 1
review_event = "COMMENT"

[[review.severity]]
name = "x"
level = 2
review_event = "COMMENT"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	_, _, err := Load()
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("err = %v, want duplicate error", err)
	}
}

// initRepo creates a real git repo at root and chdirs there for the duration
// of the test. Required because Sources() uses `git rev-parse` for repo-root
// detection. Returns the absolute root.
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
		cmd := gitCmd(t, abs, args...)
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

func gitCmd(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSourcesDiscoversRepoFiles(t *testing.T) {
	root := initRepo(t)
	t.Setenv("REVU_CONFIG", "")
	// Pin XDG_CONFIG_HOME so os.UserConfigDir() is deterministic and isolated.
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := Sources()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(xdg, "revu", "config.toml"),
		filepath.Join(root, ".revu"),
		filepath.Join(root, ".revu-local"),
	}
	if len(got) != len(want) {
		t.Fatalf("Sources len=%d want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Sources[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestLoadLocalOverridesShared(t *testing.T) {
	root := initRepo(t)
	t.Setenv("REVU_CONFIG", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Global: long context lines + a custom editor.
	userPath := filepath.Join(xdg, "revu", "config.toml")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, userPath, `
[editor]
command = "vi"
[ui]
code_context_lines = 8
horizontal_threshold = 200
`)
	// Project-shared: bumps editor only.
	writeFile(t, filepath.Join(root, ".revu"), `
[editor]
command = "code --wait"
`)
	// Per-clone local: overrides editor again, leaves UI keys alone.
	writeFile(t, filepath.Join(root, ".revu-local"), `
[editor]
command = "zed --wait"
`)

	cfg, sources, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Editor.Command != "zed --wait" {
		t.Errorf(".revu-local must win editor; got %q", cfg.Editor.Command)
	}
	// .revu-local did not set UI keys → values from the user config remain.
	if cfg.UI.CodeContextLines != 8 {
		t.Errorf("CodeContextLines should come from user config; got %d", cfg.UI.CodeContextLines)
	}
	if cfg.UI.HorizontalThreshold != 200 {
		t.Errorf("HorizontalThreshold should come from user config; got %d", cfg.UI.HorizontalThreshold)
	}
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources, got %d (%v)", len(sources), sources)
	}
	for i, s := range sources {
		if !s.Loaded {
			t.Errorf("sources[%d] (%s) should be loaded", i, s.Path)
		}
	}
}

func TestLoadOnlySharedRepoFile(t *testing.T) {
	root := initRepo(t)
	t.Setenv("REVU_CONFIG", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	writeFile(t, filepath.Join(root, ".revu"), `
[ui]
code_context_lines = 12
`)
	cfg, sources, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UI.CodeContextLines != 12 {
		t.Errorf("CodeContextLines = %d, want 12", cfg.UI.CodeContextLines)
	}
	// User config and .revu-local missing, .revu loaded.
	if len(sources) != 3 {
		t.Fatalf("sources len=%d want 3", len(sources))
	}
	if sources[0].Loaded {
		t.Errorf("user config should be not-loaded")
	}
	if !sources[1].Loaded {
		t.Errorf(".revu should be loaded")
	}
	if sources[2].Loaded {
		t.Errorf(".revu-local should be not-loaded")
	}
}

func TestLoadEnvOverrideSkipsRepoFiles(t *testing.T) {
	root := initRepo(t)

	// Even though .revu-local exists in the repo, $REVU_CONFIG should win
	// outright when set.
	writeFile(t, filepath.Join(root, ".revu-local"), `
[editor]
command = "zed --wait"
`)
	envPath := filepath.Join(t.TempDir(), "env-config.toml")
	writeFile(t, envPath, `
[editor]
command = "vi"
`)
	t.Setenv("REVU_CONFIG", envPath)

	cfg, sources, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Editor.Command != "vi" {
		t.Errorf("$REVU_CONFIG must take precedence; got %q", cfg.Editor.Command)
	}
	if len(sources) != 1 || sources[0].Path != envPath {
		t.Errorf("sources should be only envPath; got %+v", sources)
	}
}

func TestSourcesOutsideGitRepoOmitsRepoEntries(t *testing.T) {
	t.Setenv("REVU_CONFIG", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	// Chdir into a non-repo temp dir.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	abs, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(abs); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	got, err := Sources()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Sources len=%d want 1 (no repo, just user config); got %v", len(got), got)
	}
	if got[0] != filepath.Join(xdg, "revu", "config.toml") {
		t.Errorf("Sources[0]=%q want user config path", got[0])
	}
}
