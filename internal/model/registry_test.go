package model

import (
	"strings"
	"testing"
)

func TestDefaultSeverityRegistryShape(t *testing.T) {
	t.Parallel()
	reg := DefaultSeverityRegistry()
	for _, name := range []string{"critical", "major", "minor", "nit"} {
		if !reg.Has(name) {
			t.Errorf("default registry missing %q", name)
		}
	}
	if reg.Has("blocker") {
		t.Errorf("default registry should not contain blocker")
	}
}

func TestSeverityValidConsultsActiveRegistry(t *testing.T) {
	original := ActiveSeverityRegistry()
	t.Cleanup(func() { SetActiveSeverityRegistry(original) })

	custom, err := NewSeverityRegistry([]SeverityInfo{
		{Name: "blocker", Level: 100, ReviewEvent: EventRequestChanges},
		{Name: "kudos", Level: 0, ReviewEvent: EventApprove},
	})
	if err != nil {
		t.Fatalf("NewSeverityRegistry: %v", err)
	}
	SetActiveSeverityRegistry(custom)

	if !Severity("blocker").Valid() {
		t.Errorf("blocker should be valid under custom registry")
	}
	if Severity("critical").Valid() {
		t.Errorf("critical should be invalid under custom registry")
	}
}

func TestNewSeverityRegistryRejectsDuplicateNames(t *testing.T) {
	_, err := NewSeverityRegistry([]SeverityInfo{
		{Name: "x", Level: 1, ReviewEvent: EventComment},
		{Name: "x", Level: 2, ReviewEvent: EventComment},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("err = %v, want duplicate error", err)
	}
}

func TestNewSeverityRegistryDefaultsReviewEventToComment(t *testing.T) {
	reg, err := NewSeverityRegistry([]SeverityInfo{
		{Name: "x", Level: 1},
	})
	if err != nil {
		t.Fatalf("NewSeverityRegistry: %v", err)
	}
	info, ok := reg.Info("x")
	if !ok {
		t.Fatal("missing x")
	}
	if info.ReviewEvent != EventComment {
		t.Errorf("ReviewEvent = %q, want COMMENT", info.ReviewEvent)
	}
}

func TestNewSeverityRegistryRejectsInvalidReviewEvent(t *testing.T) {
	_, err := NewSeverityRegistry([]SeverityInfo{
		{Name: "x", Level: 1, ReviewEvent: ReviewEvent("WHAT")},
	})
	if err == nil || !strings.Contains(err.Error(), "review_event") {
		t.Errorf("err = %v, want review_event error", err)
	}
}

func TestSortedByLevel(t *testing.T) {
	reg, err := NewSeverityRegistry([]SeverityInfo{
		{Name: "low", Level: 10, ReviewEvent: EventComment},
		{Name: "high", Level: 100, ReviewEvent: EventRequestChanges},
		{Name: "mid", Level: 50, ReviewEvent: EventComment},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := reg.SortedByLevel()
	want := []string{"high", "mid", "low"}
	for i, info := range got {
		if info.Name != want[i] {
			t.Errorf("[%d] = %q, want %q", i, info.Name, want[i])
		}
	}
}
