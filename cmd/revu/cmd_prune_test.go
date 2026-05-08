package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/prune"
)

func TestPrintPlanEmpty(t *testing.T) {
	var buf bytes.Buffer
	printPlan(&buf, &prune.Plan{RepoDir: "/tmp/revu", Slug: "o/r"})
	if !strings.Contains(buf.String(), "No reviewed PRs found") {
		t.Errorf("missing empty marker:\n%s", buf.String())
	}
}

func TestPrintPlanSections(t *testing.T) {
	plan := &prune.Plan{
		RepoDir: "/tmp/revu",
		Slug:    "o/r",
		Delete: []prune.Entry{
			{Number: 3, State: github.PRStateMerged, SHADirCount: 1, HasUnsubmitted: false},
			{Number: 9, State: github.PRStateClosed, SHADirCount: 2, HasUnsubmitted: true},
		},
		Keep: []prune.Entry{
			{Number: 12, State: github.PRStateOpen},
			{Number: 14, State: github.PRStateOpen},
		},
		Errored: []prune.Entry{
			{Number: 2, QueryErr: errString("not found")},
		},
	}
	var buf bytes.Buffer
	printPlan(&buf, plan)
	got := buf.String()

	for _, want := range []string{
		"To delete (2)",
		"pr-3",
		"MERGED",
		"pr-9",
		"WARNING: contains unsubmitted reviews",
		"Skipped (open, 2): pr-12, pr-14",
		"Skipped (state query failed, 1)",
		"pr-2",
		"not found",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestConfirmYesAccepts(t *testing.T) {
	for _, in := range []string{"y\n", "Y\n", "yes\n", " YES \n"} {
		if !confirmYes(strings.NewReader(in), &bytes.Buffer{}, 1) {
			t.Errorf("input %q should accept", in)
		}
	}
}

func TestConfirmYesRejects(t *testing.T) {
	for _, in := range []string{"\n", "n\n", "no\n", "yepp\n"} {
		if confirmYes(strings.NewReader(in), &bytes.Buffer{}, 1) {
			t.Errorf("input %q should reject", in)
		}
	}
}

// errString is a tiny error type so test fixtures can produce predictable
// error strings without needing fmt/errors imports.
type errString string

func (e errString) Error() string { return string(e) }
