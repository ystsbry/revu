package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/model"
)

// runCmdErr is runCmd's counterpart for the paths that are supposed to
// fail: it returns the error instead of failing the test.
func runCmdErr(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// chdirTemp moves out of the revu repo for the duration of the test.
// Template and guideline discovery walks up from cwd, so running inside
// the repo would pick up its own .revu/ layer and mask the fixture.
func chdirTemp(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// useDefaultSeverities pins the process-wide severity registry to the
// built-in set for the duration of the test.
//
// The registry is global and newRootCmd's PersistentPreRunE installs
// whatever the discovered config says, so any test in this package can
// leave a different set behind. Tests whose fixtures or expectations name
// built-in severities have to say so rather than hope for the ordering.
func useDefaultSeverities(t *testing.T) {
	t.Helper()
	model.SetActiveSeverityRegistry(nil)
	t.Cleanup(func() { model.SetActiveSeverityRegistry(nil) })
}

// fullReviewFixture writes a review directory that store.Load accepts, so
// the read-and-print commands have something real to work on. Its
// severities are the built-in ones, so the registry is pinned to match.
func fullReviewFixture(t *testing.T) string {
	t.Helper()
	useDefaultSeverities(t)
	dir := t.TempDir()
	files := map[string]string{
		"review.yml": `schema_version: 1
pr:
  repo: o/r
  number: 42
  head_sha: abc1234
review_event: COMMENT
summary_file: summary.md
comments:
  - id: c1
    status: pending
    severity: major
    category: bug
    path: x.go
    line: 10
    side: RIGHT
    body_file: c1.md
  - id: c2
    status: accepted
    severity: nit
    category: style
    path: y.go
    line: 20
    side: RIGHT
    body_file: c2.md
`,
		"summary.md": "overall looks fine\n",
		"c1.md":      "this line is wrong\n",
		"c2.md":      "nit: spacing\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single line is untouched", in: "boom", want: "boom"},
		{name: "multi-line is elided", in: "boom\nstack", want: "boom ..."},
		{name: "empty stays empty", in: "", want: ""},
		{name: "a leading newline elides to an empty head", in: "\nrest", want: " ..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstLine(tt.in); got != tt.want {
				t.Fatalf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDerefInt64(t *testing.T) {
	t.Parallel()
	if got := derefInt64(nil); got != 0 {
		t.Fatalf("derefInt64(nil) = %d, want 0", got)
	}
	v := int64(42)
	if got := derefInt64(&v); got != 42 {
		t.Fatalf("derefInt64(&42) = %d, want 42", got)
	}
}

// The revu:pr skill consumes `severities --json` to discover the user's
// configured set, so the field names are a contract.
func TestSeveritiesJSON(t *testing.T) {
	isolateEnv(t)
	useDefaultSeverities(t)
	out := runCmd(t, newSeveritiesCmd(), "--json")

	var got []severityJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got) == 0 {
		t.Fatal("the registry should not be empty")
	}
	for _, d := range got {
		if d.Name == "" || d.ReviewEvent == "" {
			t.Fatalf("entry %+v is missing its name or review_event", d)
		}
	}
	// Most severe first, so a consumer can take the head of the list.
	for i := 1; i < len(got); i++ {
		if got[i-1].Level < got[i].Level {
			t.Fatalf("entries are not ordered most-severe-first: %+v", got)
		}
	}
}

func TestSeveritiesTable(t *testing.T) {
	isolateEnv(t)
	useDefaultSeverities(t)
	out := runCmd(t, newSeveritiesCmd())

	for _, want := range []string{"NAME", "LEVEL", "EVENT", "COLOR", "DESCRIPTION", "major", "nit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table is missing %q:\n%s", want, out)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatal("the default output should be a table, not JSON")
	}
}

// The severity registry is user-configurable. main installs it from
// config before any command runs, so that wiring is what gets tested
// here; the command itself just renders whatever is active.
func TestInstallSeverityRegistryFromConfig(t *testing.T) {
	isolateEnv(t)
	t.Cleanup(func() { model.SetActiveSeverityRegistry(nil) })

	cfg := filepath.Join(t.TempDir(), "config.toml")
	body := `[[review.severity]]
name = "showstopper"
level = 99
review_event = "REQUEST_CHANGES"
color = "red"
description = "stops the release"
`
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfg)

	if err := installSeverityRegistry(); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, newSeveritiesCmd())
	if !strings.Contains(out, "showstopper") {
		t.Fatalf("the configured severity should appear:\n%s", out)
	}
	// Configuring severities replaces the built-in set outright.
	if strings.Contains(out, "critical") {
		t.Fatalf("the built-in set should have been replaced:\n%s", out)
	}
}

// An invalid severity block must abort rather than silently falling back
// to the defaults, or reviews would be validated against the wrong set.
func TestInstallSeverityRegistryRejectsABadConfig(t *testing.T) {
	isolateEnv(t)
	t.Cleanup(func() { model.SetActiveSeverityRegistry(nil) })

	cfg := filepath.Join(t.TempDir(), "config.toml")
	body := `[[review.severity]]
name = "broken"
level = 10
review_event = "NOT_AN_EVENT"
`
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfg)

	if err := installSeverityRegistry(); err == nil {
		t.Fatal("an invalid review_event should abort")
	}
}

func TestConfigPrintsSourcesAndEffectiveValues(t *testing.T) {
	isolateEnv(t)
	out := runCmd(t, newConfigCmd())

	for _, want := range []string{
		"Sources (lowest → highest priority):",
		"editor.command",
		"ui.code_context_lines",
		"review.default_event",
		"review.severity",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config output is missing %q:\n%s", want, out)
		}
	}
}

func TestConfigInitWritesAStarterFile(t *testing.T) {
	isolateEnv(t)

	out := runCmd(t, newConfigCmd(), "--init")
	if !strings.Contains(out, "Wrote starter config") {
		t.Fatalf("--init should report where it wrote:\n%s", out)
	}

	// The written file has to be loadable, otherwise the starter config
	// hands the user a broken setup.
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "Wrote starter config to"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the starter config should exist: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("the starter config is empty")
	}
	t.Setenv("REVU_CONFIG", path)
	if _, err := runCmdErr(t, newConfigCmd()); err != nil {
		t.Fatalf("the starter config should load cleanly: %v", err)
	}
}

// Overwriting would destroy a config the user has already tuned.
func TestConfigInitRefusesToOverwrite(t *testing.T) {
	isolateEnv(t)
	runCmd(t, newConfigCmd(), "--init")

	out, err := runCmdErr(t, newConfigCmd(), "--init")
	if err == nil {
		t.Fatal("a second --init should refuse")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("error = %q (out %q), want a refusal", err, out)
	}
}

func TestValidateReportsTheCounts(t *testing.T) {
	isolateEnv(t)
	dir := fullReviewFixture(t)

	out := runCmd(t, newValidateCmd(), dir)
	for _, want := range []string{"OK", "PR #42", "2 comments", "pending=1", "accepted=1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("validate output is missing %q:\n%s", want, out)
		}
	}
}

func TestValidateRejectsABrokenReview(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte("comments: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmdErr(t, newValidateCmd(), dir); err == nil {
		t.Fatal("a malformed review should fail validation")
	}
}

// export builds the exact body that would be POSTed, so the payload shape
// is the thing worth pinning.
func TestExportEmitsTheSubmissionPayload(t *testing.T) {
	isolateEnv(t)
	dir := fullReviewFixture(t)

	out := runCmd(t, newExportCmd(), dir)

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if payload["commit_id"] != "abc1234" {
		t.Errorf("commit_id = %v, want the review's head sha", payload["commit_id"])
	}
	if payload["event"] != "COMMENT" {
		t.Errorf("event = %v, want COMMENT", payload["event"])
	}
	if !strings.Contains(out, "overall looks fine") {
		t.Errorf("the summary should be the payload body:\n%s", out)
	}
	// Only pending/edited comments are posted; the accepted one is not.
	comments, _ := payload["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want only the pending one", len(comments))
	}
}

func TestExportRejectsAnUnknownFormat(t *testing.T) {
	isolateEnv(t)
	dir := fullReviewFixture(t)

	_, err := runCmdErr(t, newExportCmd(), dir, "--format", "yaml")
	if err == nil {
		t.Fatal("an unsupported format should fail")
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("error = %q, want the bad format echoed", err)
	}
}

func TestTemplatesListWithoutOverrides(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	out := runCmd(t, newTemplatesListCmd())

	if !strings.Contains(out, "Search dirs") {
		t.Fatalf("list should show where it looked:\n%s", out)
	}
	if !strings.Contains(out, "No template overrides discovered.") {
		t.Fatalf("an empty environment should say so:\n%s", out)
	}
}

func TestTemplatesListShowsAnOverrideAndItsLayer(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	dir := filepath.Join(cfgHome, "revu", "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.md.tmpl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, newTemplatesListCmd())
	if !strings.Contains(out, "summary.md.tmpl") {
		t.Fatalf("the override should be listed:\n%s", out)
	}
	if !strings.Contains(out, "Resolved templates:") {
		t.Fatalf("the resolved section should appear:\n%s", out)
	}
}

// `templates path` is consumed from shell as `P=$(revu templates path X)
// || fallback`, so a miss must exit non-zero with nothing on stdout.
func TestTemplatesPath(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	dir := filepath.Join(cfgHome, "revu", "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "summary.md.tmpl")
	if err := os.WriteFile(want, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, newTemplatesPathCmd(), "summary.md.tmpl")
	if strings.TrimSpace(out) != want {
		t.Fatalf("path = %q, want %q", strings.TrimSpace(out), want)
	}
}

func TestTemplatesPathMissIsAnError(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, err := runCmdErr(t, newTemplatesPathCmd(), "no-such.tmpl")
	if err == nil {
		t.Fatal("an unknown template should exit non-zero")
	}
	// Callers use `P=$(revu templates path X) || P=$DEFAULT`, so a miss
	// must not print a path they would then try to read.
	if strings.Contains(out, ".tmpl") && strings.Contains(out, string(filepath.Separator)) {
		t.Fatalf("a miss printed something path-shaped: %q", out)
	}
}

// Guidelines come from config (review.guidelines), not from files
// dropped in a directory. `paths` prints only the ones that exist, so the
// revu:pr skill can read them line by line without checking first.
func TestGuidelinesPathsAndList(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)

	dir := t.TempDir()
	present := filepath.Join(dir, "go.md")
	if err := os.WriteFile(present, []byte("# Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "gone.md")

	cfg := filepath.Join(t.TempDir(), "config.toml")
	body := "[review]\nguidelines = [\"" + present + "\", \"" + missing + "\"]\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfg)

	paths := runCmd(t, newGuidelinesPathsCmd())
	if !strings.Contains(paths, present) {
		t.Fatalf("the existing guideline should be printed:\n%s", paths)
	}
	if strings.Contains(paths, missing) {
		t.Fatalf("a missing guideline must be skipped:\n%s", paths)
	}

	// `list` shows both, with the missing one flagged rather than dropped,
	// so a typo in config is visible.
	list := runCmd(t, newGuidelinesListCmd())
	if !strings.Contains(list, present) || !strings.Contains(list, missing) {
		t.Fatalf("list should show every configured guideline:\n%s", list)
	}
	if !strings.Contains(list, "OK") {
		t.Fatalf("list should mark the existing one:\n%s", list)
	}
}

func TestGuidelinesListWithoutAny(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := runCmd(t, newGuidelinesListCmd())
	if strings.TrimSpace(out) == "" {
		t.Fatal("the command should still explain that nothing was found")
	}
}
