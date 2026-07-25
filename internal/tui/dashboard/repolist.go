package dashboard

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RepoList is the L0 screen: the registered repositories to review from.
//
// It is a placeholder. WS-A supplies the repository registry and WS-B
// turns this into a real list that pushes the PR list; for now it renders
// a stub so the shell can be booted and exited end to end.
type RepoList struct {
	width  int
	height int
}

func NewRepoList() *RepoList { return &RepoList{} }

func (m *RepoList) Title() string { return "Repositories" }

func (m *RepoList) Init() tea.Cmd { return nil }

func (m *RepoList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		// The root screen has nothing to go back to, so q/Esc leave the
		// dashboard entirely. Deeper screens pop instead.
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *RepoList) View() string {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Render("revu — review dashboard")
	body := lipgloss.NewStyle().Faint(true).Padding(1, 1).Render(
		"No repositories registered yet.\n\n" +
			"Repository registration (`revu repo scan`) arrives with WS-A,\n" +
			"and this list becomes navigable in WS-B.")
	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[q]uit")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}
