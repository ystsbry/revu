package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUserConfig writes the isolated user config.toml with the given
// content. isolateEnv must have been called first.
func writeUserConfig(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "revu", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const profileFixture = `[[repo]]
slug = "acme/api"
path = "/clones/api"

[[repo]]
slug = "acme/web"
path = "/clones/web"

[[repo]]
slug = "oss/lib"
path = "/clones/lib"

[[profile]]
name = "work"
repos = ["acme/api", "acme/web"]
`

func TestProfileUseFiltersRepoList(t *testing.T) {
	isolateEnv(t)
	writeUserConfig(t, profileFixture)

	// Before: everything is listed.
	out := runCmd(t, newRepoCmd(), "list")
	for _, want := range []string{"acme/api", "acme/web", "oss/lib"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unfiltered list missing %s:\n%s", want, out)
		}
	}

	// Activate the profile; the choice must persist into the next command.
	out = runCmd(t, newProfileCmd(), "use", "work")
	if !strings.Contains(out, "Active profile: work") {
		t.Fatalf("use output:\n%s", out)
	}

	out = runCmd(t, newRepoCmd(), "list")
	if !strings.Contains(out, "Profile: work (2/3 repos)") {
		t.Errorf("filtered list missing profile header:\n%s", out)
	}
	if strings.Contains(out, "oss/lib") {
		t.Errorf("profile filter leaked oss/lib:\n%s", out)
	}

	// --all bypasses the profile for one invocation.
	out = runCmd(t, newRepoCmd(), "list", "--all")
	if !strings.Contains(out, "oss/lib") {
		t.Errorf("--all should list everything:\n%s", out)
	}

	// Back to default: everything again.
	out = runCmd(t, newProfileCmd(), "use", "default")
	if !strings.Contains(out, "showing all registered repos") {
		t.Fatalf("use default output:\n%s", out)
	}
	out = runCmd(t, newRepoCmd(), "list")
	if !strings.Contains(out, "oss/lib") {
		t.Errorf("after default, everything should list:\n%s", out)
	}
}

func TestProfileUseUnknownErrors(t *testing.T) {
	isolateEnv(t)
	writeUserConfig(t, profileFixture)

	cmd := newProfileCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"use", "nope"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("unknown profile should error")
	}
	if !strings.Contains(err.Error(), "work") {
		t.Errorf("error should list available profiles, got: %v", err)
	}
}

func TestProfileListMarksActiveAndMissing(t *testing.T) {
	isolateEnv(t)
	writeUserConfig(t, `active_profile = "work"

`+profileFixture+`
[[profile]]
name = "broken"
repos = ["ghost/gone"]
`)

	out := runCmd(t, newProfileCmd(), "list")
	if !strings.Contains(out, "* work (2 repos)") {
		t.Errorf("active profile not marked:\n%s", out)
	}
	if !strings.Contains(out, "  default (all 3 repos)") {
		t.Errorf("default row missing or wrongly marked:\n%s", out)
	}
	if !strings.Contains(out, "broken (0 repos)") {
		t.Errorf("broken profile should count only registered repos:\n%s", out)
	}
	if !strings.Contains(out, "[unregistered: ghost/gone]") {
		t.Errorf("unregistered slug not surfaced:\n%s", out)
	}
}

func TestRepoListProfileFlag(t *testing.T) {
	isolateEnv(t)
	writeUserConfig(t, profileFixture)

	out := runCmd(t, newRepoCmd(), "list", "--profile", "work")
	if strings.Contains(out, "oss/lib") {
		t.Errorf("--profile work should filter:\n%s", out)
	}

	cmd := newRepoCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"list", "--profile", "nope"})
	if err := cmd.Execute(); err == nil {
		t.Error("--profile with an undeclared name should error")
	}
}
