package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
)

func TestParseSubmitArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		want     bool
		wantDry  bool
		wantErr  bool
		errSubst string
	}{
		{"", true, false, false, ""},
		{"--dry-run", true, true, false, ""},
		{"-n", true, true, false, ""},
		{"  --dry-run  ", true, true, false, ""},
		{"--bogus", false, false, true, "unknown submit args"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			recognized, dry, errMsg := parseSubmitArgs(tc.in)
			if recognized != tc.want {
				t.Errorf("recognized = %v, want %v", recognized, tc.want)
			}
			if dry != tc.wantDry {
				t.Errorf("dry = %v, want %v", dry, tc.wantDry)
			}
			if tc.wantErr && !strings.Contains(errMsg, tc.errSubst) {
				t.Errorf("errMsg = %q, want substring %q", errMsg, tc.errSubst)
			}
		})
	}
}

func TestStripPrefixWord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input  string
		prefix string
		rest   string
		ok     bool
	}{
		{"submit", "submit", "", true},
		{"submit --dry-run", "submit", "--dry-run", true},
		{"submit-foo", "submit", "", false},
		{"save", "submit", "", false},
		{"", "submit", "", false},
	}
	for _, tc := range cases {
		rest, ok := stripPrefixWord(tc.input, tc.prefix)
		if ok != tc.ok || rest != tc.rest {
			t.Errorf("stripPrefixWord(%q, %q) = (%q, %v), want (%q, %v)",
				tc.input, tc.prefix, rest, ok, tc.rest, tc.ok)
		}
	}
}

func TestSubmitDoneMsgReloadsOnSuccess(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	r.BaseDir = "/tmp/x"
	called := false
	a := NewApp(Config{
		Review: r,
		Saver:  func(*model.Review) error { return nil },
		Reloader: func(rr *model.Review) error {
			called = true
			ts := time.Date(2026, 5, 3, 14, 30, 0, 0, time.UTC)
			rr.SubmittedAt = &ts
			id := int64(42)
			rr.ReviewID = &id
			return nil
		},
	})
	a.Update(submitDoneMsg{dryRun: false, err: nil})
	if !called {
		t.Errorf("reloader not called")
	}
	if r.SubmittedAt == nil || r.ReviewID == nil || *r.ReviewID != 42 {
		t.Errorf("review not updated: SubmittedAt=%v ReviewID=%v", r.SubmittedAt, r.ReviewID)
	}
	if !strings.Contains(a.View(), "submit complete") {
		t.Errorf("status missing; view:\n%s", a.View())
	}
}

func TestSubmitDoneMsgDryRunSkipsReload(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{
		Review:   r,
		Saver:    func(*model.Review) error { return nil },
		Reloader: func(*model.Review) error { t.Fatal("must not reload on dry-run"); return nil },
	})
	a.Update(submitDoneMsg{dryRun: true, err: nil})
	if !strings.Contains(a.View(), "dry-run complete") {
		t.Errorf("status missing; view:\n%s", a.View())
	}
}

func TestSubmitDoneMsgError(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{
		Review:   r,
		Saver:    func(*model.Review) error { return nil },
		Reloader: func(*model.Review) error { t.Fatal("must not reload on error"); return nil },
	})
	a.Update(submitDoneMsg{dryRun: false, err: errors.New("boom")})
	if !strings.Contains(a.View(), "submit:") {
		t.Errorf("error not surfaced; view:\n%s", a.View())
	}
}

func TestSubmitCommandRefusedWhenDirty(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	// Make dirty by accepting the first comment; runCmds drives DirtyMsg
	// through the App so the dirty flag actually flips.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	runCmds(a, cmd)
	if !a.Dirty() {
		t.Fatal("setup: expected dirty")
	}

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	for _, c := range "submit" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(a.View(), "save status changes first") {
		t.Errorf("expected dirty guard message; view:\n%s", a.View())
	}
}
