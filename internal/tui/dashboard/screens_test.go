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

// newTestShell wires Home→L2→L3 with fake loaders so the whole stack can
// be driven without touching ~/.revu or GitHub. The L3 is quitOnQ via Embed.
func newTestShell() (*Root, *Home, *PRActions) {
	l2 := NewPRActions("o/r", PRItem{Number: 5, ReviewedPath: "/dev/null/pr-5/abc1234"})
	l2.load = func() (*model.Review, error) { return testReview(5), nil }
	l2.loadJob = func() (*jobs.Job, []string) { return nil, nil }
	l2.openReview = func(r *model.Review) (Screen, error) {
		return Embed("Review #5", quitOnQ{}), nil
	}

	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }
	h.loadRepos = func() (repoListData, error) {
		return repoListData{Items: []RepoItem{{Slug: "o/r", Registered: true}}}, nil
	}
	h.loadPRs = func(slug, search string) ([]PRItem, error) {
		return []PRItem{{Number: 5, Title: "feat: 計算処理の実装", Author: "ooooo",
			ReviewedPath: "/dev/null/pr-5/abc1234"}}, nil
	}
	h.openPR = func(string, PRItem) Screen { return l2 }

	return NewRoot(h), h, l2
}

// The whole descent still works on the new layout: repos load, the first
// repo auto-selects, its PR opens L2, the review opens L3, and quits pop
// back layer by layer.
func TestFullNavigationFlow(t *testing.T) {
	t.Parallel()
	r, h, _ := newTestShell()
	drive(r, tea.WindowSizeMsg{Width: 110, Height: 40})

	// Repos land; the first repo is selected and its PRs load.
	drive(r, homeReposMsg{data: repoListData{Items: []RepoItem{{Slug: "o/r", Registered: true}}}})
	if h.selected != 0 || len(h.items) != 1 {
		t.Fatalf("initial selection/items = %d/%d, want 0/1", h.selected, len(h.items))
	}

	// Focus the PR pane and open the card.
	drive(r, keyMsg("tab"))
	drive(r, keyMsg("enter"))
	if got := r.ActiveTitle(); got != "PR #5" {
		t.Fatalf("after card open, active = %q, want PR #5 (L2)", got)
	}

	// L2: load the review, run "open review" -> L3.
	drive(r, prActionsLoadedMsg{review: testReview(5)})
	drive(r, keyMsg("enter"))
	if got := r.ActiveTitle(); got != "Review #5" {
		t.Fatalf("after L2 enter, active = %q, want Review #5 (L3)", got)
	}
	if got := r.Depth(); got != 3 {
		t.Fatalf("depth at L3 = %d, want 3", got)
	}

	// L3 quits -> L2; esc -> Home.
	drive(r, keyMsg("q"))
	if got := r.ActiveTitle(); got != "PR #5" {
		t.Fatalf("after L3 quit, active = %q, want PR #5", got)
	}
	drive(r, keyMsg("esc"))
	if got := r.ActiveTitle(); got != "Home" {
		t.Fatalf("after L2 esc, active = %q, want Home", got)
	}
	if got := r.Depth(); got != 1 {
		t.Fatalf("final depth = %d, want 1", got)
	}
}

// The selected repository carries the thick bar; selecting another repo
// loads that repo's PRs.
func TestHomeSidebarSelection(t *testing.T) {
	t.Parallel()
	var loaded []string
	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }
	h.loadPRs = func(slug, search string) ([]PRItem, error) {
		loaded = append(loaded, slug)
		return nil, nil
	}

	data := repoListData{Items: []RepoItem{
		{Slug: "a/one", Registered: true},
		{Slug: "b/two", Registered: true, Search: "label:x"},
	}}
	_, cmd := h.Update(homeReposMsg{data: data})
	h.Update(cmd()) // initial auto-select of a/one

	out := h.View()
	if !strings.Contains(out, "▌ a/one") {
		t.Errorf("selected repo must carry the thick bar:\n%s", out)
	}
	if strings.Contains(out, "▌ b/two") {
		t.Errorf("unselected repo must not carry the bar:\n%s", out)
	}

	// Move the sidebar cursor to the second repo and select it.
	h.Update(keyMsg("j"))
	_, cmd = h.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("selecting a repo should load its PRs")
	}
	h.Update(cmd())
	if len(loaded) != 2 || loaded[1] != "b/two" {
		t.Fatalf("loaded = %v, want [a/one b/two]", loaded)
	}
	if !strings.Contains(h.View(), "▌ b/two") {
		t.Errorf("bar should move to the new selection:\n%s", h.View())
	}
}

// PR cards carry the number+title line and the author line, plus badges.
func TestHomeCards(t *testing.T) {
	t.Parallel()
	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }
	h.loadPRs = func(slug, search string) ([]PRItem, error) { return nil, nil }
	h.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	h.repoData = repoListData{Items: []RepoItem{{Slug: "o/r", Registered: true}}}
	h.selected = 0
	h.prSlug = "o/r"
	h.Update(homePRsMsg{slug: "o/r", items: []PRItem{
		{Number: 667, Title: "feat: 計算処理の実装", Author: "ooooo", ReviewedPath: "/p", Submitted: true},
		{Number: 668, Title: "fix: bug", Author: "alice", JobState: "running"},
	}})

	out := h.View()
	for _, want := range []string{
		"#667 feat: 計算処理の実装",
		"author: ooooo  [reviewed]  [submitted]",
		"#668 fix: bug",
		"author: alice  [running]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cards missing %q:\n%s", want, out)
		}
	}
}

// Only the PR tab is live; the future tabs render as placeholders.
func TestHomeTabBar(t *testing.T) {
	t.Parallel()
	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }
	out := h.View()
	for _, want := range []string{"PR", "job", "report", "config"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab bar missing %q:\n%s", want, out)
		}
	}
}

// Profile name and registry warnings surface in the sidebar.
func TestHomeProfileAndWarnings(t *testing.T) {
	t.Parallel()
	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }
	h.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	h.Update(homeReposMsg{data: repoListData{
		Profile:  "ksm",
		Warnings: []string{"profile references unregistered repo x/y"},
		Items:    []RepoItem{{Slug: "a/a", Registered: true}},
	}})

	out := h.View()
	for _, want := range []string{"[ksm]", "! profile references"} {
		if !strings.Contains(out, want) {
			t.Errorf("sidebar missing %q:\n%s", want, out)
		}
	}
}

// Error and empty states render usable guidance instead of a blank pane.
func TestHomeStates(t *testing.T) {
	t.Parallel()
	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }

	h.Update(homeReposMsg{err: errors.New("boom")})
	if !strings.Contains(h.View(), "boom") {
		t.Errorf("repo error not surfaced:\n%s", h.View())
	}

	h.Update(homeReposMsg{data: repoListData{}})
	if !strings.Contains(h.View(), "no repositories") {
		t.Errorf("empty registry guidance missing:\n%s", h.View())
	}

	h.repoData = repoListData{Items: []RepoItem{{Slug: "o/r", Registered: true}}}
	h.selected = 0
	h.prSlug = "o/r"
	h.Update(homePRsMsg{slug: "o/r", err: errors.New("gh down")})
	if !strings.Contains(h.View(), "gh down") {
		t.Errorf("PR error not surfaced:\n%s", h.View())
	}
	h.Update(homePRsMsg{slug: "o/r", items: nil})
	if !strings.Contains(h.View(), "no open PRs") {
		t.Errorf("empty PR guidance missing:\n%s", h.View())
	}
}

// A stale PR response for a repo the user already left must be dropped.
func TestHomeIgnoresStalePRResponses(t *testing.T) {
	t.Parallel()
	h := NewHome()
	h.newWatch = func() *jobsWatcher { return nil }
	h.repoData = repoListData{Items: []RepoItem{{Slug: "a/a", Registered: true}, {Slug: "b/b", Registered: true}}}
	h.selected = 1
	h.prSlug = "b/b"

	h.Update(homePRsMsg{slug: "a/a", items: []PRItem{{Number: 1}}})
	if len(h.items) != 0 {
		t.Errorf("stale response leaked into the list: %v", h.items)
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
