package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeReposUpsertBySlug(t *testing.T) {
	base := Defaults()
	base.Repos = []RepoDef{
		{Slug: "a/a", Path: "/clones/a"},
		{Slug: "b/b", Path: "/clones/b"},
	}

	over := Config{Repos: []RepoDef{
		{Slug: "b/b", Path: "/elsewhere/b", Search: "label:x"}, // override
		{Slug: "c/c", Path: "rel/c"},                           // new, relative
	}}

	got, err := merge(base, over, "/layer")
	if err != nil {
		t.Fatal(err)
	}
	want := []RepoDef{
		{Slug: "a/a", Path: "/clones/a"},
		{Slug: "b/b", Path: "/elsewhere/b", Search: "label:x"},
		{Slug: "c/c", Path: "/layer/rel/c"},
	}
	if len(got.Repos) != len(want) {
		t.Fatalf("repos = %+v, want %+v", got.Repos, want)
	}
	for i := range want {
		if got.Repos[i] != want[i] {
			t.Errorf("repos[%d] = %+v, want %+v", i, got.Repos[i], want[i])
		}
	}
	// merge must not mutate its input.
	if base.Repos[1].Path != "/clones/b" {
		t.Errorf("merge mutated base: %+v", base.Repos[1])
	}
}

func TestMergeReposValidates(t *testing.T) {
	for _, bad := range []RepoDef{
		{Slug: "no-slash", Path: "/x"},
		{Slug: "a/", Path: "/x"},
		{Slug: "/b", Path: "/x"},
		{Slug: "a/b", Path: ""},
	} {
		_, err := merge(Defaults(), Config{Repos: []RepoDef{bad}}, "/l")
		if err == nil {
			t.Errorf("merge accepted invalid %+v", bad)
		}
	}
}

// setUserConfig points os.UserConfigDir at a temp dir and returns the
// config.toml path inside it.
func setUserConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "revu", "config.toml")
}

func TestUpsertUserReposCreatesFile(t *testing.T) {
	path := setUserConfig(t)

	res, err := UpsertUserRepos([]RepoDef{{Slug: "o/r", Path: "/clones/r"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || len(res.Updated) != 0 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v, want 1 added", res)
	}

	repos, err := UserRepos()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != (RepoDef{Slug: "o/r", Path: "/clones/r"}) {
		t.Fatalf("UserRepos = %+v", repos)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

// The editing policy: only [[repo]] blocks change; every other line of the
// file — comments, other tables, formatting quirks — survives byte for byte.
func TestUpsertUserReposPreservesEverythingElse(t *testing.T) {
	path := setUserConfig(t)
	original := `# my precious comment
[editor]
command = "code --wait"   # trailing comment

[[repo]]
slug = "keep/me"
path = "/clones/keep"

[ui]
code_context_lines = 7
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Update keep/me's path and add a new repo.
	res, err := UpsertUserRepos([]RepoDef{
		{Slug: "keep/me", Path: "/moved/keep"},
		{Slug: "new/repo", Path: "/clones/new", Search: "is:open"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || len(res.Added) != 1 {
		t.Fatalf("result = %+v, want 1 updated + 1 added", res)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		"# my precious comment",
		`command = "code --wait"   # trailing comment`,
		"code_context_lines = 7",
		`path = "/moved/keep"`,
		`slug = "new/repo"`,
		`search = "is:open"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("edited config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/clones/keep") {
		t.Errorf("stale path survived the update:\n%s", got)
	}

	// Idempotence: re-upserting identical entries must not touch the file.
	before, _ := os.Stat(path)
	res, err = UpsertUserRepos([]RepoDef{{Slug: "keep/me", Path: "/moved/keep"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(res.Added)+len(res.Updated) != 0 {
		t.Fatalf("re-upsert result = %+v, want 1 skipped", res)
	}
	after, _ := os.Stat(path)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("identical upsert rewrote the file")
	}
}

func TestRemoveUserRepo(t *testing.T) {
	path := setUserConfig(t)
	original := `# header comment

[[repo]]
slug = "a/a"
path = "/clones/a"

[[repo]]
slug = "b/b"
path = "/clones/b"

[editor]
command = "vi"
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := RemoveUserRepo("a/a")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected a/a to be removed")
	}

	raw, _ := os.ReadFile(path)
	got := string(raw)
	if strings.Contains(got, "a/a") {
		t.Errorf("removed block still present:\n%s", got)
	}
	for _, want := range []string{"# header comment", `slug = "b/b"`, `command = "vi"`} {
		if !strings.Contains(got, want) {
			t.Errorf("remove destroyed unrelated content %q:\n%s", want, got)
		}
	}

	// Removing an unknown slug reports false without error.
	removed, err = RemoveUserRepo("nope/nope")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("unknown slug reported as removed")
	}
}

func TestLoadMergesReposAcrossLayers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `[[repo]]
slug = "o/r"
path = "/clones/r"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfgPath)

	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := cfg.FindRepo("o/r")
	if !ok || def.Path != "/clones/r" {
		t.Fatalf("FindRepo = %+v, %v", def, ok)
	}
}
