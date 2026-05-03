package tui

import (
	"fmt"
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

type App struct {
	km     keys.KeyMap
	list   *views.List
	review *model.Review
	saver  SaveFunc

	cmdMode    bool
	cmdInput   textinput.Model
	statusMsg  string
	statusErr  bool
	dirty      bool
	awaitForce bool // true when quit was attempted with unsaved changes

	width  int
	height int
}

func NewApp(r *model.Review, saver SaveFunc) *App {
	km := keys.DefaultKeyMap()
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 64
	return &App{
		km:       km,
		list:     views.NewList(r, km),
		review:   r,
		saver:    saver,
		cmdInput: ti,
	}
}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		// Forward, but reserve a row for the command bar.
		forwarded := tea.WindowSizeMsg{Width: m.Width, Height: m.Height - 1}
		_, cmd := a.list.Update(forwarded)
		return a, cmd

	case views.DirtyMsg:
		a.dirty = true
		a.awaitForce = false
		a.clearStatus()
		return a, nil

	case tea.KeyMsg:
		if a.cmdMode {
			return a.updateCommandMode(m)
		}
		return a.updateNormalMode(m)
	}

	_, cmd := a.list.Update(msg)
	return a, cmd
}

func (a *App) updateNormalMode(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(m, a.km.Command):
		a.enterCommandMode()
		return a, textinput.Blink
	case key.Matches(m, a.km.Save):
		return a, a.doSave()
	case key.Matches(m, a.km.Quit):
		return a.tryQuit()
	}
	model, cmd := a.list.Update(m)
	if l, ok := model.(*views.List); ok {
		a.list = l
	}
	return a, cmd
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
// Recognised: save/w, quit/q, q! (force quit).
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
	body := a.list.View()
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

// Run starts the bubbletea program.
func Run(r *model.Review, saver SaveFunc) error {
	p := tea.NewProgram(NewApp(r, saver), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
