package dashboard

import (
	"testing"
)

// The stack is defensive on purpose: a nil screen would render as a blank
// pane with no way back, so it is dropped instead of pushed.
func TestStackIgnoresNilScreens(t *testing.T) {
	t.Parallel()
	var s screenStack

	s.push(nil)
	if s.depth() != 0 {
		t.Fatalf("depth = %d, want 0 after pushing nil", s.depth())
	}
	if s.top() != nil {
		t.Fatal("an empty stack has no active screen")
	}

	// replaceTop on an empty stack must not grow it.
	s.replaceTop(newTestScreen("ghost"))
	if s.depth() != 0 {
		t.Fatalf("depth = %d, want 0; replaceTop must not push", s.depth())
	}

	s.push(newTestScreen("root"))
	s.replaceTop(nil)
	if got := s.top(); got == nil || got.Title() != "root" {
		t.Fatalf("top = %v, want the original root screen kept", got)
	}
}

// selectRepo is a no-op when the index cannot resolve, but it still keeps
// the visible cursor inside the list.
func TestSelectRepoOutOfRange(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 0, 40)
	h.repoData = repoListData{Items: []RepoItem{{Slug: "a/one"}, {Slug: "b/two"}}}

	if cmd := h.selectRepo(-1); cmd != nil {
		t.Fatalf("a negative index should load nothing, got %T", cmd())
	}
	if h.repoCursor != 0 {
		t.Errorf("repoCursor = %d, want it clamped to 0", h.repoCursor)
	}

	if cmd := h.selectRepo(99); cmd != nil {
		t.Fatalf("an index past the end should load nothing, got %T", cmd())
	}
	if h.repoCursor != 1 {
		t.Errorf("repoCursor = %d, want it clamped to the last row", h.repoCursor)
	}
}

// Re-selecting the repo already shown must not refetch: the dashboard
// would otherwise hit gh every time the cursor lands back on it.
func TestSelectRepoIsIdempotent(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 0, 40)
	h.repoData = repoListData{Items: []RepoItem{{Slug: "a/one"}}}

	if cmd := h.selectRepo(0); cmd == nil {
		t.Fatal("the first selection should load PRs")
	}
	if cmd := h.selectRepo(0); cmd != nil {
		t.Fatalf("re-selecting the same repo should not reload, got %T", cmd())
	}
}

// Switching repos clears the previous repo's cards so stale PRs never show
// under the new selection.
func TestSelectRepoResetsTheCardList(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 5, 40)
	h.repoData = repoListData{Items: []RepoItem{{Slug: "a/one"}, {Slug: "b/two"}}}
	h.selected = 0
	h.prCursor, h.prOffset = 3, 2

	if cmd := h.selectRepo(1); cmd == nil {
		t.Fatal("switching repos should load the new repo's PRs")
	}
	if h.items != nil {
		t.Errorf("items = %+v, want the previous repo's cards dropped", h.items)
	}
	if h.prCursor != 0 || h.prOffset != 0 {
		t.Errorf("cursor=%d offset=%d, want both reset", h.prCursor, h.prOffset)
	}
}
