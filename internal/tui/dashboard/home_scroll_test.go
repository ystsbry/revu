package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/jobs"
)

// homeWithPRs builds a Home holding n PR cards on the PR tab, focused on
// the card list, sized so that `per` cards fit on screen.
func homeWithPRs(t *testing.T, n, height int) *Home {
	t.Helper()
	h := NewHome()
	t.Cleanup(func() { _ = h.Close() })
	h.newWatch = func() *jobsWatcher { return nil }
	h.loadRepos = func() (repoListData, error) { return repoListData{}, nil }
	h.loadPRs = func(_, _ string) ([]PRItem, error) { return nil, nil }

	items := make([]PRItem, n)
	for i := range items {
		items[i] = PRItem{Number: i + 1, Title: "pr title", Author: "alice"}
	}
	h.items = items
	h.focus = focusPRList
	h.Update(tea.WindowSizeMsg{Width: 100, Height: height})
	return h
}

func homeWithJobs(t *testing.T, n, height int) *Home {
	t.Helper()
	h := homeWithPRs(t, 0, height)
	items := make([]jobs.Job, n)
	for i := range items {
		items[i] = jobs.Job{ID: "job-" + strings.Repeat("x", i+1), State: jobs.StateDone}
	}
	h.jobItems = items
	h.tab = tabJob
	return h
}

// The scroll window must follow the cursor without ever exposing a gap
// past the end of the list — the classic off-by-one in this kind of code.
func TestEnsureCardVisible(t *testing.T) {
	t.Parallel()
	// height 3+cardHeight*3 leaves room for 3 cards.
	h := homeWithPRs(t, 10, 3+cardHeight*3)
	per := h.cardsPerPage()
	if per != 3 {
		t.Fatalf("cardsPerPage = %d, want 3 (adjust the fixture height)", per)
	}

	// Walking down past the window scrolls it by exactly one row.
	for i := 0; i < 3; i++ {
		h.Update(keyMsg("j"))
	}
	if h.prCursor != 3 || h.prOffset != 1 {
		t.Fatalf("cursor=%d offset=%d, want cursor 3 with offset 1", h.prCursor, h.prOffset)
	}

	// Jumping to the end clamps the offset so the last page stays full.
	h.prCursor = 9
	h.ensureCardVisible()
	if h.prOffset != 7 {
		t.Fatalf("offset at the end = %d, want 7 (10 items - 3 per page)", h.prOffset)
	}

	// Walking back up drags the window with the cursor.
	h.prCursor = 0
	h.ensureCardVisible()
	if h.prOffset != 0 {
		t.Fatalf("offset at the top = %d, want 0", h.prOffset)
	}
}

// A list shorter than the window must never scroll.
func TestEnsureCardVisibleWithFewerItemsThanRows(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 2, 3+cardHeight*5)

	h.prCursor = 1
	h.ensureCardVisible()
	if h.prOffset != 0 {
		t.Fatalf("offset = %d, want 0 when everything already fits", h.prOffset)
	}

	// Even a stale cursor past the end cannot push the offset negative.
	h.prCursor = 5
	h.ensureCardVisible()
	if h.prOffset < 0 {
		t.Fatalf("offset = %d, want it clamped at 0", h.prOffset)
	}
}

func TestEnsureJobVisible(t *testing.T) {
	t.Parallel()
	h := homeWithJobs(t, 10, 3+cardHeight*3)
	if per := h.cardsPerPage(); per != 3 {
		t.Fatalf("cardsPerPage = %d, want 3 (adjust the fixture height)", per)
	}

	for i := 0; i < 3; i++ {
		h.Update(keyMsg("j"))
	}
	if h.jobCursor != 3 || h.jobOffset != 1 {
		t.Fatalf("cursor=%d offset=%d, want cursor 3 with offset 1", h.jobCursor, h.jobOffset)
	}

	h.jobCursor = 9
	h.ensureJobVisible()
	if h.jobOffset != 7 {
		t.Fatalf("offset at the end = %d, want 7", h.jobOffset)
	}

	h.jobCursor = 0
	h.ensureJobVisible()
	if h.jobOffset != 0 {
		t.Fatalf("offset at the top = %d, want 0", h.jobOffset)
	}
}

// A terminal too short for even one card still has to show one.
func TestCardsPerPageNeverDropsBelowOne(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 5, 1)
	if got := h.cardsPerPage(); got != 1 {
		t.Fatalf("cardsPerPage on a tiny terminal = %d, want 1", got)
	}
}

func TestHomeTabKeysSwitchTabs(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 3, 40)

	h.Update(keyMsg("2"))
	if h.tab != tabJob {
		t.Errorf("key 2 should select the job tab, got %v", h.tab)
	}
	h.Update(keyMsg("1"))
	if h.tab != tabPR {
		t.Errorf("key 1 should select the PR tab, got %v", h.tab)
	}
}

func TestHomeFocusTogglesBetweenPanes(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"tab", "h", "l", "left", "right"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			h := homeWithPRs(t, 3, 40)
			h.focus = focusSidebar

			h.Update(keyMsg(k))
			if h.focus != focusPRList {
				t.Fatalf("%q should move focus to the card list", k)
			}
			h.Update(keyMsg(k))
			if h.focus != focusSidebar {
				t.Fatalf("%q should move focus back to the sidebar", k)
			}
		})
	}
}

// Cursor keys act on whichever pane has focus, and stop at the ends.
func TestHomeCursorKeysRespectFocusAndBounds(t *testing.T) {
	t.Parallel()

	t.Run("sidebar", func(t *testing.T) {
		t.Parallel()
		h := homeWithPRs(t, 3, 40)
		h.focus = focusSidebar
		h.repoData = repoListData{Items: []RepoItem{{Slug: "a/one"}, {Slug: "b/two"}}}

		h.Update(keyMsg("k"))
		if h.repoCursor != 0 {
			t.Fatalf("up at the top: repoCursor = %d, want 0", h.repoCursor)
		}
		h.Update(keyMsg("j"))
		h.Update(keyMsg("j"))
		if h.repoCursor != 1 {
			t.Fatalf("down past the end: repoCursor = %d, want 1", h.repoCursor)
		}
		if h.prCursor != 0 {
			t.Errorf("sidebar navigation must not move the card cursor")
		}
	})

	t.Run("job tab", func(t *testing.T) {
		t.Parallel()
		h := homeWithJobs(t, 2, 40)

		h.Update(keyMsg("k"))
		if h.jobCursor != 0 {
			t.Fatalf("up at the top: jobCursor = %d, want 0", h.jobCursor)
		}
		for i := 0; i < 5; i++ {
			h.Update(keyMsg("j"))
		}
		if h.jobCursor != 1 {
			t.Fatalf("down past the end: jobCursor = %d, want 1", h.jobCursor)
		}
	})
}

// Enter on the job tab is deliberately inert — job cards have no
// drill-down yet, and it must not open the PR behind them.
func TestHomeEnterOnJobTabDoesNothing(t *testing.T) {
	t.Parallel()
	h := homeWithJobs(t, 2, 40)
	h.items = []PRItem{{Number: 1}}
	h.openPR = func(string, PRItem) Screen {
		t.Fatal("enter on the job tab must not open a PR")
		return nil
	}

	if _, cmd := h.Update(keyMsg("enter")); cmd != nil {
		t.Fatalf("enter on the job tab produced %T, want nothing", cmd())
	}
}

// Enter on the sidebar switches to the PR tab so the selection's effect is
// visible.
func TestHomeEnterOnSidebarSwitchesToThePRTab(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 0, 40)
	h.focus = focusSidebar
	h.tab = tabJob
	h.repoData = repoListData{Items: []RepoItem{{Slug: "a/one"}}}

	_, cmd := h.Update(keyMsg("enter"))
	if h.tab != tabPR {
		t.Errorf("tab = %v, want the PR tab", h.tab)
	}
	if cmd == nil {
		t.Fatal("selecting a repo should load its PRs")
	}
}

// An empty card list has nothing to open; enter must be a no-op rather
// than indexing past the end.
func TestHomeEnterWithoutCards(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 0, 40)
	h.openPR = func(string, PRItem) Screen {
		t.Fatal("there is no PR to open")
		return nil
	}

	if _, cmd := h.Update(keyMsg("enter")); cmd != nil {
		t.Fatalf("enter with no cards produced %T, want nothing", cmd())
	}
}

func TestHomeReloadKeyRefreshesEverything(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 1, 40)

	_, cmd := h.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("r should trigger a reload")
	}
}
