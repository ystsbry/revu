package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/github"
)

// Interaction behaviour shared with localModel lives in harness_test.go;
// this file covers what is specific to the PR picker.

func samplePRs() []github.PRListItem {
	items := make([]github.PRListItem, 3)
	for i := range items {
		items[i].Number = 100 + i
		items[i].Title = []string{"add dashboard", "fix submit", "bump deps"}[i]
		items[i].HeadRefName = []string{"feat/dash", "fix/submit", "chore/deps"}[i]
		items[i].BaseRefName = "main"
		items[i].Author.Login = []string{"alice", "bob", "carol"}[i]
	}
	return items
}

// sizedModel builds a model that has been through a WindowSizeMsg and
// closes its zone manager when the test ends.
func sizedModel(t *testing.T) model {
	t.Helper()
	m := newModel(samplePRs())
	t.Cleanup(m.zones.Close)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(model)
}

func TestModelRecordsWidth(t *testing.T) {
	t.Parallel()
	m := newModel(samplePRs())
	t.Cleanup(m.zones.Close)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got := next.(model).width; got != 120 {
		t.Fatalf("width = %d, want 120", got)
	}
}

func TestViewShowsEveryPR(t *testing.T) {
	t.Parallel()
	out := sizedModel(t).View()

	for _, want := range []string{
		"Select a PR to review (3 awaiting)",
		"#100", "#101", "#102",
		"add dashboard", "fix submit", "bump deps",
		"feat/dash", "main", "@alice",
		"click again/enter: select",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() is missing %q\n%s", want, out)
		}
	}
}

func TestViewMarksOnlyTheCursorRow(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)
	next, _ := m.Update(key("down"))

	var marked []string
	for _, l := range strings.Split(next.View(), "\n") {
		if strings.Contains(l, "▸") {
			marked = append(marked, l)
		}
	}
	if len(marked) != 1 {
		t.Fatalf("expected exactly one cursor row, got %d: %v", len(marked), marked)
	}
	if !strings.Contains(marked[0], "#101") {
		t.Fatalf("cursor is on %q, want the #101 row", marked[0])
	}
}

func TestPickRejectsEmptyInput(t *testing.T) {
	t.Parallel()
	got, err := Pick(nil)
	if err == nil {
		t.Fatal("Pick(nil) should fail rather than open an empty list")
	}
	if got != nil {
		t.Fatalf("Pick(nil) = %v, want nil", got)
	}
	if !strings.Contains(err.Error(), "no PRs") {
		t.Fatalf("error = %q, want it to mention there are no PRs", err)
	}
}
