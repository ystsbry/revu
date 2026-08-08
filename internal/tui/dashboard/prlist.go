package dashboard

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/store"
)

// PRItem is one row of the L1 PR list: an open PR fetched from GitHub,
// annotated with the local review state.
type PRItem struct {
	Number    int
	Title     string
	Author    string
	UpdatedAt string // as reported by gh (RFC3339); "" when unknown

	// ReviewedPath is the latest local review dir for this PR under
	// ~/.revu, or "" when it has not been reviewed locally.
	ReviewedPath string
	// Submitted is true when that local review records a submitted_at.
	Submitted bool
	// JobState is the running/done/failed marker for background review
	// jobs. Always empty until WS-D wires the job store in; rendered
	// verbatim when non-empty so WS-D only has to fill the field.
	JobState string
}

// prListLoadedMsg carries the result of the L1 loader.
type prListLoadedMsg struct {
	items []PRItem
	err   error
}

// PRList is the L1 screen: one repository's open PRs from GitHub, filtered
// by a search condition. The initial condition comes from the repo's
// [[repo]].search (falling back to github.DefaultPRSearch) and can be
// changed in-screen with "/" for the lifetime of the screen.
type PRList struct {
	slug   string
	search string

	width  int
	height int

	searching   bool
	searchInput textinput.Model

	loading bool
	err     error
	items   []PRItem
	cursor  int

	// load fetches the rows for a search condition; openPR builds the
	// screen a selection pushes. Both are swapped out in tests.
	load   func(search string) ([]PRItem, error)
	openPR func(PRItem) Screen
}

// NewPRList builds the L1 screen for slug. search is the per-repo
// condition from the registry; empty falls back to github.DefaultPRSearch.
func NewPRList(slug, search string) *PRList {
	if search == "" {
		search = github.DefaultPRSearch
	}
	ti := textinput.New()
	ti.Prompt = "search: "
	ti.CharLimit = 200
	m := &PRList{slug: slug, search: search, searchInput: ti, loading: true}
	m.load = func(s string) ([]PRItem, error) { return loadPRsFromGitHub(slug, s) }
	m.openPR = func(it PRItem) Screen { return NewPRActions(slug, it) }
	return m
}

// loadPRsFromGitHub lists slug's open PRs matching search via gh, then
// overlays the local review state from ~/.revu.
func loadPRsFromGitHub(slug, search string) ([]PRItem, error) {
	prs, err := github.New().ListPRs(context.Background(), slug, search)
	if err != nil {
		return nil, err
	}

	// Local review overlay, best-effort: a store hiccup must not hide the
	// PR list itself.
	reviewed := map[int]string{}
	if repoDir, err := store.RepoDir(slug); err == nil {
		if dirs, err := store.ListReviewedPRDirs(repoDir); err == nil {
			for _, d := range dirs {
				reviewed[d.Number] = d.Path
			}
		}
	}

	items := make([]PRItem, 0, len(prs))
	for _, pr := range prs {
		it := PRItem{
			Number:    pr.Number,
			Title:     pr.Title,
			Author:    pr.Author.Login,
			UpdatedAt: pr.UpdatedAt,
		}
		if path, ok := reviewed[pr.Number]; ok {
			it.ReviewedPath = path
			if submitted, err := store.PeekSubmittedAt(path); err == nil {
				it.Submitted = submitted
			}
		}
		items = append(items, it)
	}
	return items, nil
}

func (m *PRList) Title() string { return m.slug }

func (m *PRList) Init() tea.Cmd { return m.reload() }

func (m *PRList) reload() tea.Cmd {
	m.loading = true
	search := m.search
	return func() tea.Msg {
		items, err := m.load(search)
		return prListLoadedMsg{items: items, err: err}
	}
}

func (m *PRList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case prListLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.items = msg.items
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}

	case tea.KeyMsg:
		if m.searching {
			return m.updateSearching(msg)
		}
		switch msg.String() {
		case "q", "esc":
			return m, Pop()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "r":
			return m, m.reload()
		case "/":
			m.searching = true
			m.searchInput.SetValue(m.search)
			m.searchInput.CursorEnd()
			return m, m.searchInput.Focus()
		case "enter":
			if m.cursor < len(m.items) {
				return m, Push(m.openPR(m.items[m.cursor]))
			}
		}
	}
	return m, nil
}

// updateSearching handles keys while the search input is focused: enter
// applies the new condition (for this screen only), esc cancels.
func (m *PRList) updateSearching(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searching = false
		m.searchInput.Blur()
		m.search = m.searchInput.Value()
		return m, m.reload()
	case "esc":
		m.searching = false
		m.searchInput.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m *PRList) View() string {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Render(fmt.Sprintf("%s — open PRs", m.slug))
	searchLine := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("search: " + m.search)
	if m.searching {
		searchLine = lipgloss.NewStyle().Padding(0, 1).Render(m.searchInput.View())
	}

	var body string
	switch {
	case m.loading:
		body = faintBox("Loading PRs ...")
	case m.err != nil:
		body = faintBox(fmt.Sprintf("Failed to list PRs: %v\n\nPress [r] to retry.", m.err))
	case len(m.items) == 0:
		body = faintBox("No open PRs match this search.\n\nPress [/] to change the search condition.")
	default:
		body = m.listView()
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter] open  [j/k] move  [/] search  [r] reload  [esc] back")
	if m.searching {
		footer = lipgloss.NewStyle().Faint(true).Padding(0, 1).
			Render("[enter] apply  [esc] cancel")
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, searchLine, body, footer)
}

func (m *PRList) listView() string {
	rows := make([]string, 0, len(m.items))
	for i, it := range m.items {
		label := fmt.Sprintf("#%-5d %s", it.Number, truncate(it.Title, m.width-40))
		if it.Author != "" {
			label += "  @" + it.Author
		}
		if it.ReviewedPath != "" {
			label += "  [reviewed]"
		}
		if it.Submitted {
			label += "  [submitted]"
		}
		if it.JobState != "" {
			label += "  [" + it.JobState + "]"
		}
		rows = append(rows, cursorRow(label, i == m.cursor))
	}
	return lipgloss.NewStyle().Padding(1, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// truncate shortens s to at most n runes (min 10), appending an ellipsis.
func truncate(s string, n int) string {
	if n < 10 {
		n = 10
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
