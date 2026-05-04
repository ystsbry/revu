package config

import (
	"os"
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

	cfg, _, ok, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ok {
		t.Errorf("ok should be false for missing file")
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

	cfg, gotPath, ok, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok || gotPath != path {
		t.Errorf("ok=%v path=%q", ok, gotPath)
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

	_, _, _, err := Load()
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

	_, _, _, err := Load()
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestPathHonorsEnv(t *testing.T) {
	t.Setenv("REVU_CONFIG", "/tmp/custom/revu.toml")
	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/custom/revu.toml" {
		t.Errorf("Path = %q", got)
	}
}

func TestSampleTOMLParses(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(path, []byte(SampleTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)

	if _, _, _, err := Load(); err != nil {
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

	cfg, _, _, err := Load()
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

	_, _, _, err := Load()
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

	_, _, _, err := Load()
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("err = %v, want duplicate error", err)
	}
}
