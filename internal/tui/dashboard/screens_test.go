package dashboard

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/jobs"
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
	l2 := NewPRActions("o/r", PRItem{Number: 5, ReviewedPath: "/dev/null/pr-5/abc1234"})
	l2.load = func() (*model.Review, error) { return testReview(5), nil }
	l2.loadJob = func() (*jobs.Job, []string) { return nil, nil }
	l2.openReview = func(r *model.Review) (Screen, error) {
		return Embed("Review #5", quitOnQ{}), nil
	}

	l1 := NewPRList("o/r", "")
	l1.newWatch = func() *jobsWatcher { return nil }
	l1.load = func(search string) ([]PRItem, error) {
		return []PRItem{{Number: 5, ReviewedPath: "/dev/null/pr-5/abc1234", Submitted: true}}, nil
	}
	l1.openPR = func(PRItem) Screen { return l2 }

	l0 := NewRepoList()
	l0.load = func() (repoListData, error) {
		return repoListData{Items: []RepoItem{{Slug: "o/r", Registered: true, ReviewedCount: 1}}}, nil
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
	drive(r, repoListLoadedMsg{data: repoListData{Items: []RepoItem{{Slug: "o/r", Registered: true, ReviewedCount: 1}}}})
	drive(r, keyMsg("enter"))
	if got := r.ActiveTitle(); got != "o/r" {
		t.Fatalf("after L0 enter, active = %q, want o/r (L1)", got)
	}

	// L1: load rows, select the PR.
	drive(r, prListLoadedMsg{items: []PRItem{{Number: 5, ReviewedPath: "/dev/null/pr-5/abc1234"}}})
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
	drive(r, repoListLoadedMsg{})

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

	m.Update(repoListLoadedMsg{})
	if !strings.Contains(m.View(), "No repositories to show") {
		t.Errorf("empty view should explain how to get started:\n%s", m.View())
	}

	m.Update(repoListLoadedMsg{data: repoListData{Items: []RepoItem{
		{Slug: "o/r", Registered: true, ReviewedCount: 3},
	}}})
	out := m.View()
	if !strings.Contains(out, "o/r") || !strings.Contains(out, "(3 reviewed)") {
		t.Errorf("loaded view should list the repo with its review count:\n%s", out)
	}
}

func TestRepoListCursorMoves(t *testing.T) {
	t.Parallel()
	m := NewRepoList()
	m.Update(repoListLoadedMsg{data: repoListData{Items: []RepoItem{{Slug: "a/a"}, {Slug: "b/b"}}}})

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

func TestPRListShowsBadges(t *testing.T) {
	t.Parallel()
	m := NewPRList("o/r", "")
	m.Update(prListLoadedMsg{items: []PRItem{
		{Number: 5, Title: "add feature", ReviewedPath: "/p", Submitted: true},
		{Number: 3, Title: "fix bug", JobState: "running"},
	}})

	out := m.View()
	if got := strings.Count(out, "[reviewed]"); got != 1 {
		t.Errorf("[reviewed] count = %d, want 1:\n%s", got, out)
	}
	if got := strings.Count(out, "[submitted]"); got != 1 {
		t.Errorf("[submitted] count = %d, want 1:\n%s", got, out)
	}
	if !strings.Contains(out, "[running]") {
		t.Errorf("job state badge missing:\n%s", out)
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

// The acceptance criterion "per-repo 検索条件が反映され、画面内で切替できる":
// "/" opens the search input seeded with the current condition; enter
// applies it and reloads with the new value; esc cancels.
func TestPRListSearchSwitch(t *testing.T) {
	t.Parallel()
	var got []string
	m := NewPRList("o/r", "label:x")
	m.load = func(search string) ([]PRItem, error) {
		got = append(got, search)
		return nil, nil
	}

	// Initial load uses the per-repo condition.
	m.newWatch = func() *jobsWatcher { return nil }
	m.Update(m.reload()())
	if len(got) != 1 || got[0] != "label:x" {
		t.Fatalf("initial searches = %v, want [label:x]", got)
	}

	// "/" → replace the query → enter reloads with it.
	m.Update(keyMsg("/"))
	if !m.searching {
		t.Fatal("/ should enter search mode")
	}
	m.searchInput.SetValue("author:alice")
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("applying a search should reload")
	}
	m.Update(cmd())
	if len(got) != 2 || got[1] != "author:alice" {
		t.Fatalf("searches = %v, want author:alice applied", got)
	}
	if m.search != "author:alice" {
		t.Errorf("search = %q, want author:alice", m.search)
	}

	// esc cancels without touching the applied condition.
	m.Update(keyMsg("/"))
	m.searchInput.SetValue("junk")
	m.Update(keyMsg("esc"))
	if m.searching || m.search != "author:alice" {
		t.Errorf("esc should cancel edit; searching=%v search=%q", m.searching, m.search)
	}
}

// While the search input is focused, list keys (q, j, ...) must go to the
// input instead of popping the screen or moving the cursor.
func TestPRListSearchModeCapturesKeys(t *testing.T) {
	t.Parallel()
	m := NewPRList("o/r", "")
	m.Update(prListLoadedMsg{items: []PRItem{{Number: 1}, {Number: 2}}})

	m.Update(keyMsg("/"))
	m.searchInput.SetValue("")
	_, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		if _, popped := cmd().(PopMsg); popped {
			t.Fatal("q while searching must not pop the screen")
		}
	}
	if m.cursor != 0 {
		t.Errorf("cursor moved while typing: %d", m.cursor)
	}
}

// The default search comes from github.DefaultPRSearch when the repo has
// no per-repo override.
func TestPRListDefaultSearch(t *testing.T) {
	t.Parallel()
	m := NewPRList("o/r", "")
	if m.search != "review-requested:@me" {
		t.Errorf("default search = %q", m.search)
	}
	m = NewPRList("o/r", "label:y")
	if m.search != "label:y" {
		t.Errorf("per-repo search = %q, want label:y", m.search)
	}
}

// A PR without a local review shows GitHub metadata plus the run actions,
// but neither [open] nor [resume] (there is nothing to open or resume).
func TestPRActionsWithoutLocalReview(t *testing.T) {
	t.Parallel()
	l2 := NewPRActions("o/r", PRItem{Number: 9, Title: "new feature", Author: "alice"})
	l2.loadJob = func() (*jobs.Job, []string) { return nil, nil }

	l2.Update(l2.reloadJob()())
	out := l2.View()
	for _, want := range []string{"local review: (none yet", "@alice", "PR #9",
		"Run review (background job)", "Run review (foreground"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	for _, absent := range []string{"Open review (TUI)", "Resume"} {
		if strings.Contains(out, absent) {
			t.Errorf("view should not offer %q without a review:\n%s", absent, out)
		}
	}
}

// L0 shows the active profile in the header and surfaces registry/profile
// inconsistencies as warnings instead of dropping them silently.
func TestRepoListShowsProfileAndWarnings(t *testing.T) {
	t.Parallel()
	m := NewRepoList()
	m.Update(repoListLoadedMsg{data: repoListData{
		Profile:  "work",
		Warnings: []string{"profile references unregistered repo x/y"},
		Items: []RepoItem{
			{Slug: "a/a", Registered: true},
			{Slug: "b/b", Registered: true, PathMissing: true},
			{Slug: "c/c"},
		},
	}})

	out := m.View()
	for _, want := range []string{
		"[profile: work]",
		"warning: profile references unregistered repo x/y",
		"[clone missing]",
		"[unregistered]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}
