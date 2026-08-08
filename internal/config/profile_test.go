package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeProfilesUpsertByName(t *testing.T) {
	base := Defaults()
	base.Profiles = []ProfileDef{
		{Name: "work", Repos: []string{"a/a"}},
	}
	base.ActiveProfile = "work"

	over := Config{
		Profiles: []ProfileDef{
			{Name: "work", Repos: []string{"a/a", "b/b"}}, // override
			{Name: "oss", Repos: []string{"c/c"}},         // new
		},
		ActiveProfile: "oss",
	}

	got, err := merge(base, over, "/layer")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("profiles = %+v", got.Profiles)
	}
	if got.Profiles[0].Name != "work" || len(got.Profiles[0].Repos) != 2 {
		t.Errorf("work profile not overridden: %+v", got.Profiles[0])
	}
	if got.ActiveProfile != "oss" {
		t.Errorf("active = %q, want oss", got.ActiveProfile)
	}
	if len(base.Profiles[0].Repos) != 1 {
		t.Errorf("merge mutated base: %+v", base.Profiles[0])
	}
}

func TestMergeProfilesValidates(t *testing.T) {
	for _, bad := range []ProfileDef{
		{Name: "", Repos: []string{"a/a"}},
		{Name: "default", Repos: []string{"a/a"}},
		{Name: "x", Repos: []string{"not-a-slug"}},
	} {
		_, err := merge(Defaults(), Config{Profiles: []ProfileDef{bad}}, "/l")
		if err == nil {
			t.Errorf("merge accepted invalid profile %+v", bad)
		}
	}
}

func TestActiveRepos(t *testing.T) {
	cfg := Defaults()
	cfg.Repos = []RepoDef{
		{Slug: "a/a", Path: "/1"},
		{Slug: "b/b", Path: "/2"},
		{Slug: "c/c", Path: "/3"},
	}
	cfg.Profiles = []ProfileDef{
		// Profile order differs from registry order on purpose.
		{Name: "work", Repos: []string{"c/c", "a/a", "gone/gone"}},
	}

	// No active profile: everything, registry order.
	repos, missing, unknown := cfg.ActiveRepos()
	if len(repos) != 3 || len(missing) != 0 || unknown != "" {
		t.Fatalf("default ActiveRepos = %+v, %v, %q", repos, missing, unknown)
	}

	// "default" behaves identically.
	cfg.ActiveProfile = DefaultProfileName
	if repos, _, _ := cfg.ActiveRepos(); len(repos) != 3 {
		t.Fatalf("explicit default should list everything")
	}

	// A named profile filters and keeps its own order.
	cfg.ActiveProfile = "work"
	repos, missing, unknown = cfg.ActiveRepos()
	if unknown != "" {
		t.Fatalf("unexpected unknown profile %q", unknown)
	}
	if len(repos) != 2 || repos[0].Slug != "c/c" || repos[1].Slug != "a/a" {
		t.Errorf("profile repos = %+v, want [c/c a/a]", repos)
	}
	if len(missing) != 1 || missing[0] != "gone/gone" {
		t.Errorf("missing = %v, want [gone/gone]", missing)
	}

	// An undeclared active profile falls back to everything but says so.
	cfg.ActiveProfile = "nope"
	repos, _, unknown = cfg.ActiveRepos()
	if len(repos) != 3 || unknown != "nope" {
		t.Errorf("unknown profile: repos=%d unknown=%q, want 3 / nope", len(repos), unknown)
	}
}

func TestSetActiveUserProfile(t *testing.T) {
	path := setUserConfig(t)
	original := `# header comment

[editor]
command = "vi"

[[repo]]
slug = "a/a"
path = "/clones/a"
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set: the key must land in the top-level region (before [editor]).
	if err := SetActiveUserProfile("work"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	got := string(raw)
	keyIdx := strings.Index(got, `active_profile = "work"`)
	headerIdx := strings.Index(got, "[editor]")
	if keyIdx < 0 || headerIdx < 0 || keyIdx > headerIdx {
		t.Fatalf("active_profile must sit above the first table header:\n%s", got)
	}
	for _, want := range []string{"# header comment", `command = "vi"`, `slug = "a/a"`} {
		if !strings.Contains(got, want) {
			t.Errorf("edit destroyed unrelated content %q:\n%s", want, got)
		}
	}

	// Replace: switching profiles rewrites the line, not adds another.
	if err := SetActiveUserProfile("oss"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Count(string(raw), "active_profile") != 1 {
		t.Fatalf("expected exactly one active_profile line:\n%s", raw)
	}
	if !strings.Contains(string(raw), `active_profile = "oss"`) {
		t.Fatalf("active_profile not replaced:\n%s", raw)
	}

	// Clear: "default" removes the key entirely.
	if err := SetActiveUserProfile("default"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "active_profile") {
		t.Fatalf("default should remove the key:\n%s", raw)
	}

	// Clearing when absent is a no-op, not an error.
	if err := SetActiveUserProfile("default"); err != nil {
		t.Fatal(err)
	}
}

func TestSetActiveUserProfileCreatesFile(t *testing.T) {
	path := setUserConfig(t)
	if err := SetActiveUserProfile("work"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `active_profile = "work"`) {
		t.Fatalf("missing key in created file:\n%s", raw)
	}
}
