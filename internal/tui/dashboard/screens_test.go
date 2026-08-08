package dashboard

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
)

// quitOnQ is a stand-in for the review TUI: a plain tea.Model that quits
// on "q", the way tui.App does once there are no unsaved changes.
type quitOnQ struct{}

func (quitOnQ) Init() tea.Cmd { return nil }
func (quitOnQ) View() string  { return "embedded-body" }
func (quitOnQ) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "q" {
		return quitOnQ{}, tea.Quit
	}
	return quitOnQ{}, nil
}

func keyMsg(s string) tea.KeyMsg {
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func testReview(pr int) *model.Review {
	return &model.Review{
		SchemaVersion: 1,
		PR:            model.PRMeta{Repo: "o/r", Number: pr, HeadSHA: "abc1234", BaseBranch: "main"},
		ReviewEvent:   model.EventComment,
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor,
				Category: model.CategoryDesign, Path: "a/x.go", Line: 10,
				Side: model.SideRight, BodyFile: "a.md"},
		},
	}
}

// newTestShell wires L0→L1→L2→L3 with fake loaders so the whole stack can
// be driven without touching ~/.revu. The L3 is quitOnQ via Embed.
func newTestShell() (*Root, *RepoList, *PRList, *PRActions) {
	l2 := NewPRActions("o/r", PRItem{Number: 5, ShortSHA: "abc1234", Path: "/dev/null/pr-5/abc1234"})
	l2.load = func() (*model.Review, error) { return testReview(5), nil }
	l2.openReview = func(r *model.Review) (Screen, error) {
		return Embed("Review #5", quitOnQ{}), nil
	}

	l1 := NewPRList("o/r")
	l1.load = func() ([]PRItem, error) {
		return []PRItem{{Number: 5, ShortSHA: "abc1234", Submitted: true}}, nil
	}
	l1.openPR = func(PRItem) Screen { return l2 }

	l0 := NewRepoList()
	l0.load = func() ([]RepoItem, error) {
		return []RepoItem{{Slug: "o/r", PRCount: 1}}, nil
	}
	l0.openRepo = func(RepoItem) Screen { return l1 }

	return NewRoot(l0), l0, l1, l2
}

// The issue's acceptance criterion: L0→L1→L2→L3→back works end to end on
// keyboard input alone, without the stack breaking.
func TestFullNavigationFlow(t *testing.T) {
	t.Parallel()
	r, _, _, _ := newTestShell()
	drive(r, tea.WindowSizeMsg{Width: 100, Height: 30})

	// L0: load rows, select the repo.
	drive(r, repoListLoadedMsg{items: []RepoItem{{Slug: "o/r", PRCount: 1}}})
	drive(r, keyMsg("enter"))
	if got := r.ActiveTitle(); got != "o/r" {
		t.Fatalf("after L0 enter, active = %q, want o/r (L1)", got)
	}

	// L1: load rows, select the PR.
	drive(r, prListLoadedMsg{items: []PRItem{{Number: 5, ShortSHA: "abc1234"}}})
	drive(r, keyMsg("enter"))
	if got := r.ActiveTitle(); got != "PR #5" {
		t.Fatalf("after L1 enter, active = %q, want PR #5 (L2)", got)
	}

	// L2: load the review, run "open review".
	drive(r, prActionsLoadedMsg{review: testReview(5)})
	drive(r, keyMsg("enter"))
	if got := r.ActiveTitle(); got != "Review #5" {
		t.Fatalf("after L2 enter, active = %q, want Review #5 (L3)", got)
	}
	if got := r.Depth(); got != 4 {
		t.Fatalf("depth at L3 = %d, want 4", got)
	}

	// L3: "q" would quit a standalone program; embedded it must pop.
	drive(r, keyMsg("q"))
	if got := r.ActiveTitle(); got != "PR #5" {
		t.Fatalf("after L3 quit, active = %q, want PR #5 (back at L2)", got)
	}

	// Walk back down to the root.
	drive(r, keyMsg("esc"))
	if got := r.ActiveTitle(); got != "o/r" {
		t.Fatalf("after L2 esc, active = %q, want o/r (L1)", got)
	}
	drive(r, keyMsg("esc"))
	if got := r.ActiveTitle(); got != "Repositories" {
		t.Fatalf("after L1 esc, active = %q, want Repositories (L0)", got)
	}
	if got := r.Depth(); got != 1 {
		t.Fatalf("final depth = %d, want 1", got)
	}
}

// Quitting at the root leaves the dashboard entirely — L0 has nothing to
// pop back to.
func TestRepoListQuitsAtRoot(t *testing.T) {
	t.Parallel()
	r, _, _, _ := newTestShell()
	drive(r, repoListLoadedMsg{items: nil})

	_, cmd := r.Update(keyMsg("q"))
	if !isQuit(cmd) {
		t.Errorf("q at L0 should quit the dashboard")
	}
}

func TestRepoListStates(t *testing.T) {
	t.Parallel()
	m := NewRepoList()

	if !strings.Contains(m.View(), "Loading") {
		t.Errorf("initial view should show the loading state:\n%s", m.View())
	}

	m.Update(repoListLoadedMsg{err: errors.New("boom")})
	if !strings.Contains(m.View(), "boom") {
		t.Errorf("error view should surface the loader error:\n%s", m.View())
	}

	m.Update(repoListLoadedMsg{items: nil})
	if !strings.Contains(m.View(), "No reviewed repositories") {
		t.Errorf("empty view should explain how to get started:\n%s", m.View())
	}

	m.Update(repoListLoadedMsg{items: []RepoItem{{Slug: "o/r", PRCount: 3}}})
	out := m.View()
	if !strings.Contains(out, "o/r") || !strings.Contains(out, "(3 PR)") {
		t.Errorf("loaded view should list the repo with its PR count:\n%s", out)
	}
}

func TestRepoListCursorMoves(t *testing.T) {
	t.Parallel()
	m := NewRepoList()
	m.Update(repoListLoadedMsg{items: []RepoItem{{Slug: "a/a"}, {Slug: "b/b"}}})

	var pushed Screen
	m.openRepo = func(it RepoItem) Screen {
		s := newTestScreen(it.Slug)
		pushed = s
		return s
	}

	m.Update(keyMsg("j"))
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter on a row should push")
	}
	if msg, ok := cmd().(PushMsg); !ok || msg.Screen != pushed {
		t.Fatalf("enter should push the opened repo screen, got %T", cmd())
	}
	if pushed.Title() != "b/b" {
		t.Errorf("cursor should have moved to the second repo, opened %q", pushed.Title())
	}

	// Cursor must clamp at both ends.
	m.Update(keyMsg("j"))
	m.Update(keyMsg("j"))
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want clamped at 1", m.cursor)
	}
	m.Update(keyMsg("k"))
	m.Update(keyMsg("k"))
	m.Update(keyMsg("k"))
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want clamped at 0", m.cursor)
	}
}

func TestPRListShowsSubmittedBadge(t *testing.T) {
	t.Parallel()
	m := NewPRList("o/r")
	m.Update(prListLoadedMsg{items: []PRItem{
		{Number: 5, ShortSHA: "abc1234", Submitted: true},
		{Number: 3, ShortSHA: "def5678"},
	}})

	out := m.View()
	if !strings.Contains(out, "[submitted]") {
		t.Errorf("submitted PR should carry a badge:\n%s", out)
	}
	if got := strings.Count(out, "[submitted]"); got != 1 {
		t.Errorf("badge count = %d, want 1", got)
	}
}

func TestPRActionsOpenFailureStaysOnScreen(t *testing.T) {
	t.Parallel()
	l2 := NewPRActions("o/r", PRItem{Number: 5})
	l2.load = func() (*model.Review, error) { return testReview(5), nil }
	l2.openReview = func(*model.Review) (Screen, error) {
		return nil, errors.New("no clone")
	}

	r := NewRoot(l2)
	drive(r, prActionsLoadedMsg{review: testReview(5)})
	drive(r, keyMsg("enter"))

	if got := r.Depth(); got != 1 {
		t.Fatalf("failed open must not push, depth = %d", got)
	}
	if !strings.Contains(r.View(), "no clone") {
		t.Errorf("action error should be shown:\n%s", r.View())
	}
}

// quitToPop must see through batches: a model may batch its quit with
// other commands, and the quit still has to become a pop.
func TestQuitToPopUnwrapsBatches(t *testing.T) {
	t.Parallel()
	noop := func() tea.Msg { return nil }
	cmd := quitToPop(tea.Batch(noop, tea.Quit))

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected BatchMsg, got %T", msg)
	}
	var sawPop bool
	for _, c := range batch {
		if c == nil {
			continue
		}
		if _, ok := c().(PopMsg); ok {
			sawPop = true
		}
	}
	if !sawPop {
		t.Errorf("quit inside a batch should map to PopMsg")
	}
}
