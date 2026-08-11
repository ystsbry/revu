package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/prune"
)

// printPlan is what the user reads before agreeing to delete review
// directories, so every section and the unsubmitted warning matter.
func TestPrintPlan(t *testing.T) {
	t.Parallel()
	plan := &prune.Plan{
		Slug:    "o/r",
		RepoDir: "/home/u/.revu/o/r",
		Delete: []prune.Entry{
			{Number: 1, State: "MERGED", SHADirCount: 2},
			{Number: 2, State: "CLOSED", SHADirCount: 1, HasUnsubmitted: true},
		},
		Keep:    []prune.Entry{{Number: 5, State: "OPEN"}, {Number: 3, State: "OPEN"}},
		Errored: []prune.Entry{{Number: 9, QueryErr: errors.New("not found")}},
	}

	var buf bytes.Buffer
	printPlan(&buf, plan)
	out := buf.String()

	for _, want := range []string{
		"Inspected 5 PRs", "slug=o/r",
		"To delete (2):", "pr-1", "MERGED", "2 SHA dirs",
		"pr-2", "CLOSED", "1 SHA dir",
		"WARNING: contains unsubmitted reviews",
		"Skipped (open, 2): pr-3, pr-5", // sorted, not in plan order
		"Skipped (state query failed, 1):", "not found",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan output is missing %q:\n%s", want, out)
		}
	}
}

func TestPrintPlanWithNothingFound(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	printPlan(&buf, &prune.Plan{Slug: "o/r", RepoDir: "/home/u/.revu/o/r"})

	if !strings.Contains(buf.String(), "No reviewed PRs found") {
		t.Fatalf("an empty plan should say so:\n%s", buf.String())
	}
}

func TestPluralHelpers(t *testing.T) {
	t.Parallel()
	if got := pluralS(1); got != "" {
		t.Errorf("pluralS(1) = %q, want empty", got)
	}
	if got := pluralS(2); got != "s" {
		t.Errorf("pluralS(2) = %q, want s", got)
	}
	if got := pluralYies(1); got != "y" {
		t.Errorf("pluralYies(1) = %q, want y", got)
	}
	if got := pluralYies(2); got != "ies" {
		t.Errorf("pluralYies(2) = %q, want ies", got)
	}
}

func TestFormatPRList(t *testing.T) {
	t.Parallel()
	if got := formatPRList([]int{1, 2, 10}); got != "pr-1, pr-2, pr-10" {
		t.Fatalf("formatPRList = %q", got)
	}
	if got := formatPRList(nil); got != "" {
		t.Fatalf("formatPRList(nil) = %q, want empty", got)
	}
}

// prune deletes user data, so the confirmation must only accept a
// deliberate yes.
func TestConfirmYes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "y", input: "y\n", want: true},
		{name: "yes", input: "yes\n", want: true},
		{name: "uppercase", input: "Y\n", want: true},
		{name: "empty is a no", input: "\n", want: false},
		{name: "n", input: "n\n", want: false},
		{name: "anything else is a no", input: "sure\n", want: false},
		{name: "eof is a no", input: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			got := confirmYes(strings.NewReader(tt.input), &out, 3)
			if got != tt.want {
				t.Fatalf("confirmYes(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if !strings.Contains(out.String(), "3") {
				t.Fatalf("the prompt should say how many will be deleted: %q", out.String())
			}
		})
	}
}

// Outside a clone and without --repo there is nothing to prune against.
func TestPruneWithoutARepo(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)

	_, err := runCmdErr(t, newPruneCmd())
	if err == nil {
		t.Fatal("prune outside a clone should fail")
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Fatalf("error = %q, want it to suggest --repo", err)
	}
}

// --dry-run must not touch the filesystem even when the plan is empty of
// GitHub knowledge (no gh available in tests).
func TestPruneDryRunOnAnEmptyRepoDir(t *testing.T) {
	isolateEnv(t)
	chdirTemp(t)
	withEmptyPATH(t)

	repoDir := filepath.Join(os.Getenv("REVU_HOME"), "o", "r")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runCmdErr(t, newPruneCmd(), "--repo", "o/r", "--dry-run")
	if err != nil {
		t.Fatalf("an empty repo dir should be a clean no-op: %v\n%s", err, out)
	}
	if !strings.Contains(out, "No reviewed PRs found") {
		t.Fatalf("output should report an empty sweep:\n%s", out)
	}
}

func TestResumeRejectsDirAndRepoTogether(t *testing.T) {
	isolateEnv(t)
	dir := fullReviewFixture(t)

	_, err := runCmdErr(t, newResumeCmd(), dir, "--repo", "o/r")
	if err == nil {
		t.Fatal("[dir] and --repo together should fail")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %q, want a mutual-exclusion complaint", err)
	}
}

// A review generated before revu persisted session ids cannot be resumed;
// the message has to explain that rather than failing obscurely.
func TestResumeWithoutARecordedSession(t *testing.T) {
	isolateEnv(t)
	dir := fullReviewFixture(t)

	_, err := runCmdErr(t, newResumeCmd(), dir)
	if err == nil {
		t.Fatal("a review with no session id should fail")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Fatalf("error = %q, want it to mention the missing session", err)
	}
}

func TestResumeOnAMissingDirectory(t *testing.T) {
	isolateEnv(t)

	if _, err := runCmdErr(t, newResumeCmd(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a missing review directory should fail")
	}
}

func TestSubmitOnAMissingDirectory(t *testing.T) {
	isolateEnv(t)

	if _, err := runCmdErr(t, newSubmitCmd(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a missing review directory should fail")
	}
}

// Submitting requires gh; without it the command must fail before doing
// anything rather than half-way through.
func TestSubmitWithoutGh(t *testing.T) {
	isolateEnv(t)
	withEmptyPATH(t)
	dir := fullReviewFixture(t)

	if _, err := runCmdErr(t, newSubmitCmd(), dir, "--yes"); err == nil {
		t.Fatal("submit without gh should fail")
	}
}
