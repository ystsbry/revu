package dashboard

import tea "github.com/charmbracelet/bubbletea"

// NewProgram builds the dashboard's bubbletea program, rooted at the
// repository list. Separate from Run so tests and future callers can
// construct the program without starting it.
func NewProgram() *tea.Program {
	return tea.NewProgram(NewRoot(NewRepoList()), tea.WithAltScreen())
}

// Run starts the dashboard and blocks until the user exits.
func Run() error {
	_, err := NewProgram().Run()
	return err
}
