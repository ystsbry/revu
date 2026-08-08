package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/store"
)

// PRItem is one row of the L1 PR list: the latest reviewed dir for one PR.
type PRItem struct {
	Number      int
	ShortSHA    string
	Path        string    // absolute review dir (pr-N/{sha})
	GeneratedAt time.Time // review.yml mtime; zero when unknown
	Submitted   bool      // submitted_at recorded in review.yml
}

// prListLoadedMsg carries the result of the L1 loader.
type prListLoadedMsg struct {
	items []PRItem
	err   error
}

// PRList is the L1 screen: the reviewed PRs of one repository. Selecting
// one pushes the L2 action screen for that review.
type PRList struct {
	slug string

	width  int
	height int

	loading bool
	err     error
	items   []PRItem
	cursor  int

	load   func() ([]PRItem, error)
	openPR func(PRItem) Screen
}

func NewPRList(slug string) *PRList {
	m := &PRList{slug: slug, loading: true}
	m.load = func() ([]PRItem, error) { return loadPRsFromStore(slug) }
	m.openPR = func(it PRItem) Screen { return NewPRActions(slug, it) }
	return m
}

// loadPRsFromStore lists the repo's reviewed PR dirs, newest PR first, and
// annotates each with its submission state.
func loadPRsFromStore(slug string) ([]PRItem, error) {
	repoDir, err := store.RepoDir(slug)
	if err != nil {
		return nil, err
	}
	dirs, err := store.ListReviewedPRDirs(repoDir)
	if err != nil {
		return nil, err
	}
	items := make([]PRItem, 0, len(dirs))
	for _, d := range dirs {
		it := PRItem{Number: d.Number, ShortSHA: d.ShortSHA, Path: d.Path}
		if st, err := os.Stat(filepath.Join(d.Path, "review.yml")); err == nil {
			it.GeneratedAt = st.ModTime()
		}
		// Submission state is cosmetic here; a peek failure just leaves
		// the badge off rather than failing the whole list.
		if submitted, err := store.PeekSubmittedAt(d.Path); err == nil {
			it.Submitted = submitted
		}
		items = append(items, it)
	}
	return items, nil
}

func (m *PRList) Title() string { return m.slug }

func (m *PRList) Init() tea.Cmd { return m.reload() }

func (m *PRList) reload() tea.Cmd {
	m.loading = true
	return func() tea.Msg {
		items, err := m.load()
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
		case "enter":
			if m.cursor < len(m.items) {
				return m, Push(m.openPR(m.items[m.cursor]))
			}
		}
	}
	return m, nil
}

func (m *PRList) View() string {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Render(m.slug + " — reviewed PRs")

	var body string
	switch {
	case m.loading:
		body = faintBox("Loading PRs ...")
	case m.err != nil:
		body = faintBox(fmt.Sprintf("Failed to list PRs: %v\n\nPress [r] to retry.", m.err))
	case len(m.items) == 0:
		body = faintBox("No reviewed PRs for this repository yet.")
	default:
		body = m.listView()
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter] open  [j/k] move  [r] reload  [esc] back")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m *PRList) listView() string {
	rows := make([]string, 0, len(m.items))
	for i, it := range m.items {
		label := fmt.Sprintf("PR #%-5d %s", it.Number, it.ShortSHA)
		if !it.GeneratedAt.IsZero() {
			label += "  " + it.GeneratedAt.Format("2006-01-02 15:04")
		}
		if it.Submitted {
			label += "  [submitted]"
		}
		rows = append(rows, cursorRow(label, i == m.cursor))
	}
	return lipgloss.NewStyle().Padding(1, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
