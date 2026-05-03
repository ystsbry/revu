package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
	"github.com/ystsbry/revu/internal/tui/views"
)

// SaveFunc persists status changes back to disk. The app uses a function
// rather than a concrete dependency so tests can stub it without spinning
// up a filesystem fixture.
type SaveFunc func(*model.Review) error

type viewState int

const (
	viewList viewState = iota
	viewDetail
	viewSummary
)

type App struct {
	km       keys.KeyMap
	list     *views.List
	detail   *views.Detail
	summary  *views.Summary
	state    viewState
	review   *model.Review
	saver    SaveFunc
	repoRoot string

	cmdMode    bool
	cmdInput   textinput.Model
	statusMsg  string
	statusErr  bool
	dirty      bool
	awaitForce bool
	showHelp   bool

	width  int
	height int
}

// Config carries the inputs NewApp needs from the caller.
type Config struct {
	Review   *model.Review
	Saver    SaveFunc
	RepoRoot string
}

func NewApp(cfg Config) *App {
	km := keys.DefaultKeyMap()
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 64
	return &App{
		km:       km,
		list:     views.NewList(cfg.Review, km),
		detail:   views.NewDetail(cfg.Review, cfg.RepoRoot, km, 0),
		summary:  views.NewSummary(cfg.Review, km),
		state:    viewList,
		review:   cfg.Review,
		saver:    cfg.Saver,
		repoRoot: cfg.RepoRoot,
		cmdInput: ti,
	}
}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		// Reserve one row for the bottom command/status bar.
		fwd := tea.WindowSizeMsg{Width: m.Width, Height: m.Height - 1}
		a.forwardToActive(fwd)
		return a, nil

	case views.DirtyMsg:
		a.dirty = true
		a.awaitForce = false
		a.clearStatus()
		return a, nil

	case views.GoToDetailMsg:
		a.detail.SetIndex(m.Index)
		a.state = viewDetail
		a.forwardSize()
		return a, nil

	case views.GoToSummaryMsg:
		a.state = viewSummary
		a.forwardSize()
		return a, nil

	case views.GoToListMsg:
		a.state = viewList
		a.forwardSize()
		return a, nil

	case views.EditMsg:
		return a, openEditor(m.Path)

	case editorDoneMsg:
		if m.err != nil {
			a.setError(fmt.Sprintf("editor: %v", m.err))
			return a, nil
		}
		if err := a.reloadBody(m.path); err != nil {
			a.setError(fmt.Sprintf("reload: %v", err))
		} else {
			a.setInfo("reloaded " + filepath.Base(m.path))
		}
		return a, nil

	case tea.KeyMsg:
		if a.cmdMode {
			return a.updateCommandMode(m)
		}
		return a.updateNormalMode(m)
	}

	return a.delegateToActive(msg)
}

func (a *App) updateNormalMode(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.showHelp {
		// Any of ?, Esc, q dismisses help; everything else is swallowed.
		switch {
		case key.Matches(m, a.km.Help), m.Type == tea.KeyEsc, key.Matches(m, a.km.Quit):
			a.showHelp = false
		}
		return a, nil
	}
	switch {
	case key.Matches(m, a.km.Help):
		a.showHelp = true
		return a, nil
	case key.Matches(m, a.km.Command):
		a.enterCommandMode()
		return a, textinput.Blink
	case key.Matches(m, a.km.Save):
		return a, a.doSave()
	case key.Matches(m, a.km.Quit):
		return a.tryQuit()
	}
	return a.delegateToActive(m)
}

func (a *App) delegateToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch a.state {
	case viewDetail:
		_, cmd := a.detail.Update(msg)
		return a, cmd
	case viewSummary:
		_, cmd := a.summary.Update(msg)
		return a, cmd
	default:
		_, cmd := a.list.Update(msg)
		return a, cmd
	}
}

func (a *App) forwardToActive(msg tea.Msg) {
	switch a.state {
	case viewDetail:
		a.detail.Update(msg)
	case viewSummary:
		a.summary.Update(msg)
	default:
		a.list.Update(msg)
	}
}

func (a *App) forwardSize() {
	if a.width == 0 || a.height == 0 {
		return
	}
	a.forwardToActive(tea.WindowSizeMsg{Width: a.width, Height: a.height - 1})
}

func (a *App) updateCommandMode(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.Type {
	case tea.KeyEsc:
		a.exitCommandMode()
		return a, nil
	case tea.KeyEnter:
		input := strings.TrimSpace(a.cmdInput.Value())
		a.exitCommandMode()
		return a.runCommand(input)
	}
	var cmd tea.Cmd
	a.cmdInput, cmd = a.cmdInput.Update(m)
	return a, cmd
}

func (a *App) enterCommandMode() {
	a.cmdMode = true
	a.cmdInput.SetValue("")
	a.cmdInput.Focus()
	a.clearStatus()
}

func (a *App) exitCommandMode() {
	a.cmdMode = false
	a.cmdInput.Blur()
	a.cmdInput.SetValue("")
}

// runCommand dispatches a typed ":foo" command.
func (a *App) runCommand(input string) (tea.Model, tea.Cmd) {
	switch input {
	case "":
		return a, nil
	case "save", "w":
		return a, a.doSave()
	case "quit", "q":
		return a.tryQuit()
	case "q!", "quit!":
		return a, tea.Quit
	default:
		a.setError(fmt.Sprintf("unknown command: %s", input))
		return a, nil
	}
}

func (a *App) doSave() tea.Cmd {
	if a.saver == nil {
		a.setError("no saver configured")
		return nil
	}
	if err := a.saver(a.review); err != nil {
		a.setError(fmt.Sprintf("save failed: %v", err))
		return nil
	}
	a.dirty = false
	a.awaitForce = false
	a.setInfo("saved")
	return nil
}

func (a *App) tryQuit() (tea.Model, tea.Cmd) {
	if a.dirty && !a.awaitForce {
		a.awaitForce = true
		a.setError("unsaved changes — press q again to discard, or :save first")
		return a, nil
	}
	return a, tea.Quit
}

func (a *App) View() string {
	if a.showHelp {
		return helpView(a.width, a.height)
	}
	var body string
	switch a.state {
	case viewDetail:
		body = a.detail.View()
	case viewSummary:
		body = a.summary.View()
	default:
		body = a.list.View()
	}
	bar := a.bottomBar()
	return lipgloss.JoinVertical(lipgloss.Left, body, bar)
}

func (a *App) bottomBar() string {
	if a.cmdMode {
		return a.cmdInput.View()
	}
	left := ""
	if a.dirty {
		left = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[modified]")
	} else {
		left = lipgloss.NewStyle().Faint(true).Render("[saved]")
	}
	right := a.statusMsg
	if a.statusErr {
		right = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(right)
	} else if right != "" {
		right = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(right)
	}
	if right == "" {
		return left
	}
	return left + "  " + right
}

func (a *App) setInfo(s string)  { a.statusMsg = s; a.statusErr = false }
func (a *App) setError(s string) { a.statusMsg = s; a.statusErr = true }
func (a *App) clearStatus()      { a.statusMsg = ""; a.statusErr = false }

// Dirty reports whether unsaved changes exist. Exposed for tests.
func (a *App) Dirty() bool { return a.dirty }

// State returns the current view state. Exposed for tests.
func (a *App) State() viewState { return a.state }

// State accessors for tests (since viewState is unexported).
func (a *App) IsList() bool    { return a.state == viewList }
func (a *App) IsDetail() bool  { return a.state == viewDetail }
func (a *App) IsSummary() bool { return a.state == viewSummary }

// Run starts the bubbletea program.
func Run(cfg Config) error {
	p := tea.NewProgram(NewApp(cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
