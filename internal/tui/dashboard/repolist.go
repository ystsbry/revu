package dashboard

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/store"
)

// RepoItem is one row of the L0 repository list.
type RepoItem struct {
	Slug          string // "owner/repo"
	Path          string // registered clone path ("" when unregistered)
	Search        string // per-repo PR search override ("" = default)
	ReviewedCount int    // reviewed PRs under ~/.revu
	Registered    bool   // false for store-only fallback rows
	PathMissing   bool   // registered but the clone is absent on disk
}

// repoListData is what the L0 loader produces in one shot.
type repoListData struct {
	// Profile is the active profile name ("" when showing everything).
	Profile string
	// Warnings surface non-fatal registry/profile inconsistencies
	// (undeclared active profile, profile slugs that are unregistered).
	Warnings []string
	Items    []RepoItem
}

// repoListLoadedMsg carries the result of the L0 loader.
type repoListLoadedMsg struct {
	data repoListData
	err  error
}

// RepoList is the L0 screen: the registered repositories, narrowed by the
// active profile (see `revu profile`). Selecting one pushes the L1 PR
// list with the repo's per-repo search condition.
//
// When nothing is registered yet, the repositories that already have
// reviews under ~/.revu are shown instead, so the dashboard stays usable
// before the first `revu repo scan`.
type RepoList struct {
	width  int
	height int

	loading bool
	err     error
	data    repoListData
	cursor  int

	// load fetches the rows; openRepo builds the screen a selection
	// pushes. Both are swapped out in tests.
	load     func() (repoListData, error)
	openRepo func(RepoItem) Screen
}

func NewRepoList() *RepoList {
	return &RepoList{
		loading: true,
		load:    loadReposFromConfig,
		openRepo: func(it RepoItem) Screen {
			return NewPRList(it.Slug, it.Search)
		},
	}
}

// loadReposFromConfig resolves the registry through the active profile and
// annotates each repo with its local review count. With an empty registry
// it falls back to the repos that have reviews under ~/.revu.
func loadReposFromConfig() (repoListData, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return repoListData{}, err
	}

	var data repoListData
	repos, missing, unknown := cfg.ActiveRepos()
	if unknown != "" {
		data.Warnings = append(data.Warnings,
			fmt.Sprintf("active profile %q is not declared; showing all repos", unknown))
	} else if cfg.ActiveProfile != "" && cfg.ActiveProfile != config.DefaultProfileName {
		data.Profile = cfg.ActiveProfile
	}
	for _, slug := range missing {
		data.Warnings = append(data.Warnings,
			fmt.Sprintf("profile references unregistered repo %s", slug))
	}

	for _, r := range repos {
		it := RepoItem{Slug: r.Slug, Path: r.Path, Search: r.Search, Registered: true}
		if _, statErr := os.Stat(r.Path); statErr != nil {
			it.PathMissing = true
		}
		it.ReviewedCount = reviewedCount(r.Slug)
		data.Items = append(data.Items, it)
	}

	if len(cfg.Repos) == 0 {
		reviewed, err := store.ListReviewedRepos()
		if err != nil {
			return repoListData{}, err
		}
		for _, r := range reviewed {
			data.Items = append(data.Items, RepoItem{
				Slug:          r.Slug,
				ReviewedCount: r.PRCount,
			})
		}
	}
	return data, nil
}

// reviewedCount is how many reviewed PR dirs exist for slug. Best-effort:
// lookup failures render as zero rather than failing the list.
func reviewedCount(slug string) int {
	repoDir, err := store.RepoDir(slug)
	if err != nil {
		return 0
	}
	dirs, err := store.ListReviewedPRDirs(repoDir)
	if err != nil {
		return 0
	}
	return len(dirs)
}

func (m *RepoList) Title() string { return "Repositories" }

func (m *RepoList) Init() tea.Cmd { return m.reload() }

func (m *RepoList) reload() tea.Cmd {
	m.loading = true
	return func() tea.Msg {
		data, err := m.load()
		return repoListLoadedMsg{data: data, err: err}
	}
}

func (m *RepoList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case repoListLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.data = msg.data
		if m.cursor >= len(m.data.Items) {
			m.cursor = max(0, len(m.data.Items)-1)
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
			if m.cursor < len(m.data.Items)-1 {
				m.cursor++
			}
		case "r":
			return m, m.reload()
		case "enter":
			if m.cursor < len(m.data.Items) {
				return m, Push(m.openRepo(m.data.Items[m.cursor]))
			}
		}
	}
	return m, nil
}

func (m *RepoList) View() string {
	header := "revu — review dashboard"
	if m.data.Profile != "" {
		header += fmt.Sprintf("   [profile: %s]", m.data.Profile)
	}
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(header)

	var body string
	switch {
	case m.loading:
		body = faintBox("Loading repositories ...")
	case m.err != nil:
		body = faintBox(fmt.Sprintf("Failed to list repositories: %v\n\nPress [r] to retry.", m.err))
	case len(m.data.Items) == 0:
		body = faintBox("No repositories to show.\n\n" +
			"Register clones with `revu repo scan <root>` (or `revu repo add`),\n" +
			"then press [r] to reload. Profiles (`revu profile use`) narrow\n" +
			"this list further.")
	default:
		body = m.listView()
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter] open  [j/k] move  [r] reload  [q]uit")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m *RepoList) listView() string {
	rows := make([]string, 0, len(m.data.Items)+len(m.data.Warnings))
	for i, it := range m.data.Items {
		label := it.Slug
		if it.ReviewedCount > 0 {
			label += fmt.Sprintf("  (%d reviewed)", it.ReviewedCount)
		}
		if !it.Registered {
			label += "  [unregistered]"
		} else if it.PathMissing {
			label += "  [clone missing]"
		}
		rows = append(rows, cursorRow(label, i == m.cursor))
	}
	for _, w := range m.data.Warnings {
		rows = append(rows, "", lipgloss.NewStyle().Faint(true).Render("warning: "+w))
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
