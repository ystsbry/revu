package dashboard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/store"
)

// RepoItem is one row of the L0 repository list.
type RepoItem struct {
	Slug    string // "owner/repo"
	PRCount int    // reviewed PRs under ~/.revu
}

// repoListLoadedMsg carries the result of the L0 loader.
type repoListLoadedMsg struct {
	items []RepoItem
	err   error
}

// RepoList is the L0 screen: the repositories that have reviews under
// ~/.revu. Selecting one pushes the L1 PR list.
//
// Until WS-A lands its config-registered repository registry, the review
// store is the only repository source; the loader seam is where the
// registry will be merged in.
type RepoList struct {
	width  int
	height int

	loading bool
	err     error
	items   []RepoItem
	cursor  int

	// load fetches the rows; openRepo builds the screen a selection
	// pushes. Both are swapped out in tests.
	load     func() ([]RepoItem, error)
	openRepo func(RepoItem) Screen
}

func NewRepoList() *RepoList {
	return &RepoList{
		loading: true,
		load:    loadReposFromStore,
		openRepo: func(it RepoItem) Screen {
			return NewPRList(it.Slug)
		},
	}
}

// loadReposFromStore adapts store.ListReviewedRepos to the screen's row type.
func loadReposFromStore() ([]RepoItem, error) {
	repos, err := store.ListReviewedRepos()
	if err != nil {
		return nil, err
	}
	items := make([]RepoItem, 0, len(repos))
	for _, r := range repos {
		items = append(items, RepoItem{Slug: r.Slug, PRCount: r.PRCount})
	}
	return items, nil
}

func (m *RepoList) Title() string { return "Repositories" }

func (m *RepoList) Init() tea.Cmd { return m.reload() }

func (m *RepoList) reload() tea.Cmd {
	m.loading = true
	return func() tea.Msg {
		items, err := m.load()
		return repoListLoadedMsg{items: items, err: err}
	}
}

func (m *RepoList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case repoListLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.items = msg.items
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}

	case tea.KeyMsg:
		switch msg.String() {
		// The root screen has nothing to go back to, so q/Esc leave the
		// dashboard entirely. Deeper screens pop instead.
		case "q", "esc":
			return m, tea.Quit
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
		case "enter":
			if m.cursor < len(m.items) {
				return m, Push(m.openRepo(m.items[m.cursor]))
			}
		}
	}
	return m, nil
}

func (m *RepoList) View() string {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Render("revu — review dashboard")

	var body string
	switch {
	case m.loading:
		body = faintBox("Loading repositories ...")
	case m.err != nil:
		body = faintBox(fmt.Sprintf("Failed to list repositories: %v\n\nPress [r] to retry.", m.err))
	case len(m.items) == 0:
		body = faintBox("No reviewed repositories yet.\n\n" +
			"Generate one with `revu review <PR>` (or /revu:pr) inside a\n" +
			"clone, then come back — reviewed repos show up here.")
	default:
		body = m.listView()
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter] open  [j/k] move  [r] reload  [q]uit")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m *RepoList) listView() string {
	rows := make([]string, 0, len(m.items))
	for i, it := range m.items {
		label := fmt.Sprintf("%s  (%d PR)", it.Slug, it.PRCount)
		rows = append(rows, cursorRow(label, i == m.cursor))
	}
	return lipgloss.NewStyle().Padding(1, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// faintBox renders body text in the dimmed, padded style every screen's
// non-list states (loading, error, empty) share.
func faintBox(s string) string {
	return lipgloss.NewStyle().Faint(true).Padding(1, 1).Render(s)
}

// cursorRow renders one selectable line, bolding and marking the cursor.
func cursorRow(label string, selected bool) string {
	if selected {
		return lipgloss.NewStyle().Bold(true).Render("▸ " + label)
	}
	return "  " + label
}
