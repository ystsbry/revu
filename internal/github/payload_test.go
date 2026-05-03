package github

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/model"
)

func sampleReview() *model.Review {
	startLine := 8
	return &model.Review{
		SchemaVersion: 1,
		PR:            model.PRMeta{Repo: "o/r", Number: 7, HeadSHA: "abc1234"},
		ReviewEvent:   model.EventRequestChanges,
		SummaryFile:   "summary.md",
		SummaryBody:   "全体所感",
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusAccepted, Severity: model.SeverityMajor, Category: model.CategoryDesign,
				Path: "a.go", Line: 10, Side: model.SideRight, Body: "fix this"},
			{ID: "c2", Status: model.StatusRejected, Severity: model.SeverityNit, Category: model.CategoryStyle,
				Path: "b.go", Line: 20, Side: model.SideRight, Body: "n/a"},
			{ID: "c3", Status: model.StatusPending, Severity: model.SeverityMinor, Category: model.CategoryPerf,
				Path: "c.go", Line: 30, Side: model.SideRight, Body: "?"},
			{ID: "c4", Status: model.StatusAccepted, Severity: model.SeverityMajor, Category: model.CategoryBug,
				Path: "d.go", Line: 40, Side: model.SideRight, Body: "bug", StartLine: &startLine},
			{ID: "c5", Status: model.StatusEdited, Severity: model.SeverityMinor, Category: model.CategoryDoc,
				Path: "e.go", Line: 50, Side: model.SideRight, Body: "edited"},
		},
	}
}

func TestBuildPayloadAcceptedOnly(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	p, counts, err := BuildPayload(r)
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}
	if p.CommitID != "abc1234" {
		t.Errorf("CommitID = %q", p.CommitID)
	}
	if p.Body != "全体所感" {
		t.Errorf("Body = %q", p.Body)
	}
	if p.Event != "REQUEST_CHANGES" {
		t.Errorf("Event = %q", p.Event)
	}

	// Expect c1, c4, c5 (accepted + edited).
	if len(p.Comments) != 3 {
		t.Fatalf("Comments len = %d, want 3", len(p.Comments))
	}
	wantPaths := []string{"a.go", "d.go", "e.go"}
	for i, want := range wantPaths {
		if p.Comments[i].Path != want {
			t.Errorf("Comments[%d].Path = %q want %q", i, p.Comments[i].Path, want)
		}
	}
	if p.Comments[1].StartLine == nil || *p.Comments[1].StartLine != 8 {
		t.Errorf("c4 StartLine = %v", p.Comments[1].StartLine)
	}
	if p.Comments[1].StartSide != "RIGHT" {
		t.Errorf("c4 StartSide = %q", p.Comments[1].StartSide)
	}

	if counts.Accepted != 3 || counts.Rejected != 1 || counts.Pending != 1 {
		t.Errorf("counts = %+v", counts)
	}
}

func TestBuildPayloadJSONShape(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	p, _, err := BuildPayload(r)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		`"commit_id":"abc1234"`,
		`"event":"REQUEST_CHANGES"`,
		`"path":"a.go"`,
		`"side":"RIGHT"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in JSON: %s", want, got)
		}
	}
	// Comments without StartLine must NOT emit the field.
	if strings.Contains(got, `"start_line":null`) {
		t.Errorf("StartLine should be omitempty: %s", got)
	}
}

func TestBuildPayloadValidation(t *testing.T) {
	t.Parallel()
	if _, _, err := BuildPayload(nil); err == nil {
		t.Error("nil review should error")
	}

	noSHA := sampleReview()
	noSHA.PR.HeadSHA = ""
	if _, _, err := BuildPayload(noSHA); err == nil {
		t.Error("empty HeadSHA should error")
	}

	noSummary := sampleReview()
	noSummary.SummaryBody = ""
	if _, _, err := BuildPayload(noSummary); err == nil {
		t.Error("empty summary should error")
	}

	badEvent := sampleReview()
	badEvent.ReviewEvent = "BLOCK"
	if _, _, err := BuildPayload(badEvent); err == nil {
		t.Error("invalid event should error")
	}
}

func TestBuildPayloadAllRejected(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	for i := range r.Comments {
		r.Comments[i].Status = model.StatusRejected
	}
	p, counts, err := BuildPayload(r)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(p.Comments) != 0 {
		t.Errorf("should have no inline comments, got %d", len(p.Comments))
	}
	if counts.Accepted != 0 || counts.Rejected != 5 {
		t.Errorf("counts = %+v", counts)
	}
}
