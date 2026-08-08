package dashboard

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui"
)

// testScreen records what the shell hands it so tests can assert on
// routing and sizing without rendering real screens.
type testScreen struct {
	title string
	sizes []tea.WindowSizeMsg
	keys  []string
	inits int
}

func newTestScreen(title string) *testScreen { return &testScreen{title: title} }

func (s *testScreen) Title() string { return s.title }
func (s *testScreen) Init() tea.Cmd { s.inits++; return nil }
func (s *testScreen) View() string  { return "view:" + s.title }

func (s *testScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.sizes = append(s.sizes, m)
	case tea.KeyMsg:
		s.keys = append(s.keys, m.String())
	}
	return s, nil
}

func (s *testScreen) lastSize() (tea.WindowSizeMsg, bool) {
	if len(s.sizes) == 0 {
		return tea.WindowSizeMsg{}, false
	}
	return s.sizes[len(s.sizes)-1], true
}

// drive runs msg through the root and, when a command comes back, executes
// it once and feeds the result back in — mirroring what the bubbletea
// runtime does, so Push/Pop commands actually navigate.
//
// The command runs on its own goroutine, as the real runtime would: a
// screen's Init may legitimately block forever (the review TUI's file
// watcher parks until a file changes), and running it inline would hang
// the suite instead of failing it. Only the result is fed back here, so
// the model is still touched from a single goroutine.
func drive(r *Root, msg tea.Msg) {
	_, cmd := r.Update(msg)
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case out := <-done:
		if out != nil {
			r.Update(out)
		}
	case <-time.After(2 * time.Second):
		// A long-running command; the runtime would deliver its message
		// later. Nothing under test depends on one.
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestStackPopRefusesToEmptyItself(t *testing.T) {
	t.Parallel()
	var s screenStack
	root := newTestScreen("root")
	s.push(root)

	if _, ok := s.pop(); ok {
		t.Fatalf("popping the only screen should fail")
	}
	if got := s.depth(); got != 1 {
		t.Errorf("depth = %d, want 1 (stack must never empty)", got)
	}

	child := newTestScreen("child")
	s.push(child)
	popped, ok := s.pop()
	if !ok {
		t.Fatalf("popping a pushed screen should succeed")
	}
	if popped != child {
		t.Errorf("popped %v, want the child screen", popped.Title())
	}
	if s.top() != root {
		t.Errorf("top = %q, want root", s.top().Title())
	}
}

func TestRootPushPopNavigation(t *testing.T) {
	t.Parallel()
	root := newTestScreen("L0")
	r := NewRoot(root)

	if got := r.Depth(); got != 1 {
		t.Fatalf("initial depth = %d, want 1", got)
	}

	child := newTestScreen("L1")
	drive(r, PushMsg{Screen: child})

	if got := r.Depth(); got != 2 {
		t.Errorf("depth after push = %d, want 2", got)
	}
	if got := r.ActiveTitle(); got != "L1" {
		t.Errorf("active = %q, want L1", got)
	}
	if child.inits != 1 {
		t.Errorf("pushed screen Init called %d times, want 1", child.inits)
	}

	drive(r, PopMsg{})

	if got := r.Depth(); got != 1 {
		t.Errorf("depth after pop = %d, want 1", got)
	}
	if got := r.ActiveTitle(); got != "L0" {
		t.Errorf("active after pop = %q, want L0", got)
	}
}

func TestRootPopAtRootIsNoOp(t *testing.T) {
	t.Parallel()
	r := NewRoot(newTestScreen("L0"))

	drive(r, PopMsg{})

	if got := r.Depth(); got != 1 {
		t.Errorf("depth = %d, want 1; popping the root must not empty the stack", got)
	}
	if got := r.ActiveTitle(); got != "L0" {
		t.Errorf("active = %q, want L0", got)
	}
}

// Resizes must reach screens buried in the stack, not just the visible
// one: a screen revealed by a later pop would otherwise still be laid out
// for the terminal size it was last shown at.
func TestRootResizeBroadcastsToEveryScreen(t *testing.T) {
	t.Parallel()
	root := newTestScreen("L0")
	r := NewRoot(root)
	child := newTestScreen("L1")
	drive(r, PushMsg{Screen: child})

	drive(r, tea.WindowSizeMsg{Width: 120, Height: 40})

	for _, s := range []*testScreen{root, child} {
		got, ok := s.lastSize()
		if !ok {
			t.Fatalf("%s never received a size", s.title)
		}
		if got.Width != 120 {
			t.Errorf("%s width = %d, want 120", s.title, got.Width)
		}
		// One row is reserved for the breadcrumb trail.
		if got.Height != 39 {
			t.Errorf("%s height = %d, want 39 (40 minus the breadcrumb row)", s.title, got.Height)
		}
	}
}

// A screen pushed after the terminal size is known must be sized before
// its first View, or it renders once at zero width.
func TestRootSizesNewlyPushedScreen(t *testing.T) {
	t.Parallel()
	r := NewRoot(newTestScreen("L0"))
	drive(r, tea.WindowSizeMsg{Width: 100, Height: 30})

	child := newTestScreen("L1")
	drive(r, PushMsg{Screen: child})

	got, ok := child.lastSize()
	if !ok {
		t.Fatalf("pushed screen was never sized")
	}
	if got.Width != 100 || got.Height != 29 {
		t.Errorf("size = %dx%d, want 100x29", got.Width, got.Height)
	}
}

func TestRootCtrlCQuitsFromAnyScreen(t *testing.T) {
	t.Parallel()
	r := NewRoot(newTestScreen("L0"))
	drive(r, PushMsg{Screen: newTestScreen("L1")})

	_, cmd := r.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if !isQuit(cmd) {
		t.Errorf("ctrl+c should quit from a nested screen")
	}
}

// Every key except ctrl+c belongs to the active screen: the review TUI's
// "q" warns about unsaved changes, and the shell must not preempt that.
func TestRootDelegatesKeysToActiveScreen(t *testing.T) {
	t.Parallel()
	root := newTestScreen("L0")
	r := NewRoot(root)
	child := newTestScreen("L1")
	drive(r, PushMsg{Screen: child})

	drive(r, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if len(child.keys) != 1 || child.keys[0] != "q" {
		t.Errorf("active screen keys = %v, want [q]", child.keys)
	}
	if len(root.keys) != 0 {
		t.Errorf("buried screen received keys %v, want none", root.keys)
	}
}

func TestRootViewShowsBreadcrumbAndActiveScreen(t *testing.T) {
	t.Parallel()
	r := NewRoot(newTestScreen("L0"))
	drive(r, PushMsg{Screen: newTestScreen("L1")})

	out := r.View()

	for _, want := range []string{"L0", "L1", "view:L1"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "view:L0") {
		t.Errorf("only the active screen should render its body:\n%s", out)
	}
}

// Embed is the seam the existing review TUI joins the stack through. This
// pins that an unmodified *tui.App can be hosted: sized, rendered, and
// keyed without the dashboard knowing what it is.
func TestEmbedHostsTheReviewTUI(t *testing.T) {
	t.Parallel()
	review := &model.Review{
		SchemaVersion: 1,
		PR:            model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "abc1234", BaseBranch: "main"},
		ReviewEvent:   model.EventComment,
		// Left empty on purpose: a BaseDir would make App.Init start a
		// filesystem watcher, which this test has no use for.
		BaseDir: "",
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor,
				Category: model.CategoryDesign, Path: "a/x.go", Line: 10,
				Side: model.SideRight, BodyFile: "a.md"},
		},
	}
	app := tui.NewApp(tui.Config{Review: review})

	r := NewRoot(newTestScreen("L0"))
	drive(r, tea.WindowSizeMsg{Width: 120, Height: 40})
	drive(r, PushMsg{Screen: Embed("Review", app)})

	if got := r.ActiveTitle(); got != "Review" {
		t.Fatalf("active = %q, want Review", got)
	}
	out := r.View()
	if !strings.Contains(out, "Review") {
		t.Errorf("breadcrumb missing the embedded screen:\n%s", out)
	}
	// The app renders its own list header once it has been sized.
	if !strings.Contains(out, "o/r") {
		t.Errorf("embedded review TUI did not render:\n%s", out)
	}

	// Keys must reach the embedded model: 'j' moves off the summary row.
	drive(r, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	drive(r, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !app.Dirty() {
		t.Errorf("keys did not reach the embedded review TUI")
	}
}

func TestRepoListQuitsOnQAndEsc(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
	} {
		m := NewRepoList()
		_, cmd := m.Update(key)
		if !isQuit(cmd) {
			t.Errorf("key %q should quit the dashboard", key.String())
		}
	}
}

// The root screen's quit must survive the trip through the shell: Root
// delegates the key and has to hand the screen's tea.Quit back to the
// runtime rather than swallowing it.
func TestRootPropagatesQuitFromTheRootScreen(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
	} {
		r := NewRoot(NewRepoList())
		if _, cmd := r.Update(key); !isQuit(cmd) {
			t.Errorf("key %q on the root screen should quit the dashboard", key.String())
		}
	}
}

func TestRepoListRendersWithoutASize(t *testing.T) {
	t.Parallel()
	// The first View happens before bubbletea delivers a WindowSizeMsg.
	if out := NewRoot(NewRepoList()).View(); out == "" {
		t.Errorf("dashboard rendered nothing before its first resize")
	}
}

// mouseRecorder captures mouse messages a screen receives.
type mouseRecorder struct {
	*testScreen
	mice []tea.MouseMsg
}

func (m *mouseRecorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm, ok := msg.(tea.MouseMsg); ok {
		m.mice = append(m.mice, mm)
	}
	return m, nil
}

// Screens hit-test in their own coordinates, so the shell must shift the
// pointer up past the breadcrumb row before forwarding — and swallow
// clicks on the breadcrumb itself.
func TestRootTranslatesMouseCoordinates(t *testing.T) {
	t.Parallel()
	rec := &mouseRecorder{testScreen: newTestScreen("L0")}
	r := NewRoot(rec)

	r.Update(tea.MouseMsg{X: 3, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(rec.mice) != 0 {
		t.Fatalf("breadcrumb click must not reach the screen, got %v", rec.mice)
	}

	r.Update(tea.MouseMsg{X: 3, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(rec.mice) != 1 {
		t.Fatalf("mouse not forwarded: %v", rec.mice)
	}
	if got := rec.mice[0].Y; got != 4 {
		t.Errorf("Y = %d, want 4 (shifted past the breadcrumb)", got)
	}
	if got := rec.mice[0].X; got != 3 {
		t.Errorf("X = %d, want 3 (unchanged)", got)
	}
}
