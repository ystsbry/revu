package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/filter"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
	"github.com/ystsbry/revu/internal/tui/views"
)

// SaveFunc persists status changes back to disk. The app uses a function
// rather than a concrete dependency so tests can stub it without spinning
// up a filesystem fixture.
type SaveFunc func(*model.Review) error

// ReloadFunc re-reads review.yml and copies SubmittedAt/ReviewID back into
// the given Review. Used after `:submit` finishes in a subprocess so the
// in-memory view reflects the new submission record.
type ReloadFunc func(*model.Review) error

type viewState int

const (
	viewList viewState = iota
	viewDetail
	viewSummary
	viewEdit
)

type App struct {
	km       keys.KeyMap
	list     *views.List
	detail   *views.Detail
	summary  *views.Summary
	edit     *views.Edit
	state    viewState
	review   *model.Review
	saver    SaveFunc
	reloader ReloadFunc
	repoRoot string
	settings Settings
	watcher  *watcher

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
	Reloader ReloadFunc
	RepoRoot string
	Settings Settings
}

// Settings carries user-configurable knobs from config.toml. The zero value
// is fine; views fall back to their built-in defaults.
type Settings struct {
	EditorCommand       string
	CodeContextLines    int
	HorizontalThreshold int
}

func NewApp(cfg Config) *App {
	km := keys.DefaultKeyMap()
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 64
	return &App{
		km:   km,
		list: views.NewList(cfg.Review, km),
		detail: views.NewDetail(cfg.Review, cfg.RepoRoot, km, 0, views.DetailSettings{
			CodeContextLines:    cfg.Settings.CodeContextLines,
			HorizontalThreshold: cfg.Settings.HorizontalThreshold,
		}),
		summary:  views.NewSummary(cfg.Review, km),
		edit:     views.NewEdit(cfg.Review, km, 0),
		state:    viewList,
		review:   cfg.Review,
		saver:    cfg.Saver,
		reloader: cfg.Reloader,
		repoRoot: cfg.RepoRoot,
		settings: cfg.Settings,
		cmdInput: ti,
	}
}

func (a *App) Init() tea.Cmd {
	if a.review == nil || a.review.BaseDir == "" {
		return nil
	}
	w, err := newWatcher(a.review.BaseDir)
	if err != nil {
		return func() tea.Msg { return fsWatcherErrMsg{err: err} }
	}
	a.watcher = w
	return w.listenForChange()
}

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

	case views.GoToEditMsg:
		a.edit.SetIndex(m.Index)
		a.state = viewEdit
		a.forwardSize()
		return a, nil

	case views.EditMsg:
		return a, openEditor(m.Path, a.settings.EditorCommand)

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

	case views.FilterErrMsg:
		a.setError(fmt.Sprintf("filter: %v", m.Err))
		return a, nil

	case fsChangeMsg:
		if err := a.reloadBody(m.path); err != nil {
			// Path is outside the review (e.g. unrelated .md edited). Stay silent.
		} else {
			a.setInfo("reloaded " + filepath.Base(m.path))
		}
		if a.watcher != nil {
			return a, a.watcher.listenForChange()
		}
		return a, nil

	case fsWatcherErrMsg:
		a.setError("watcher unavailable; use :reload after editing files externally")
		return a, nil

	case submitDoneMsg:
		if m.err != nil {
			a.setError(fmt.Sprintf("submit: %v", m.err))
			return a, nil
		}
		if m.dryRun {
			a.setInfo("dry-run complete")
			return a, nil
		}
		if a.reloader != nil {
			if err := a.reloader(a.review); err != nil {
				a.setError(fmt.Sprintf("reload after submit: %v", err))
				return a, nil
			}
		}
		a.setInfo("submit complete")
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
	// When the list view is capturing keys for its filter input, let it
	// consume everything (including ":" and "?") until it exits filter mode.
	if a.state == viewList && a.list.IsFilterMode() {
		return a.delegateToActive(m)
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
	case viewEdit:
		_, cmd := a.edit.Update(msg)
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
	case viewEdit:
		a.edit.Update(msg)
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
	case "reload":
		return a, a.runReload()
	}
	if rest, ok := stripPrefixWord(input, "submit"); ok {
		recognized, dryRun, errMsg := parseSubmitArgs(rest)
		if !recognized {
			a.setError(errMsg)
			return a, nil
		}
		if a.dirty {
			a.setError("save status changes first (:save), then :submit")
			return a, nil
		}
		return a, a.runSubmit(dryRun)
	}
	if rest, ok := stripPrefixWord(input, "filter"); ok {
		return a.runFilterCommand(rest)
	}
	a.setError(fmt.Sprintf("unknown command: %s", input))
	return a, nil
}

// runReload re-reads summary.md and every comment body file from disk into
// the in-memory Review. Used as a manual fallback when fsnotify is not
// available. Status fields are not touched, so user-pending state is safe.
func (a *App) runReload() tea.Cmd {
	r := a.review
	if r == nil || r.BaseDir == "" {
		a.setError("reload: review has no BaseDir")
		return nil
	}
	if r.SummaryFile != "" {
		summaryPath := filepath.Join(r.BaseDir, r.SummaryFile)
		if err := a.reloadBody(summaryPath); err != nil {
			a.setError(fmt.Sprintf("reload summary: %v", err))
			return nil
		}
	}
	for i := range r.Comments {
		c := &r.Comments[i]
		if c.BodyFile == "" {
			continue
		}
		bodyPath := filepath.Join(r.BaseDir, c.BodyFile)
		if err := a.reloadBody(bodyPath); err != nil {
			a.setError(fmt.Sprintf("reload %s: %v", c.ID, err))
			return nil
		}
	}
	a.setInfo(fmt.Sprintf("reloaded %d comment(s) + summary", len(r.Comments)))
	return nil
}

// runFilterCommand implements ":filter <expr>" and ":filter clear".
func (a *App) runFilterCommand(rest string) (tea.Model, tea.Cmd) {
	rest = strings.TrimSpace(rest)
	if rest == "" || rest == "clear" {
		a.list.ClearFilter()
		a.setInfo("filter cleared")
		return a, nil
	}
	f, err := filter.Parse(rest)
	if err != nil {
		a.setError(fmt.Sprintf("filter: %v", err))
		return a, nil
	}
	a.list.SetFilter(f)
	if a.list.VisibleCount() == 0 {
		a.setInfo("filter applied (no matches)")
	} else {
		a.setInfo(fmt.Sprintf("filter applied (%d visible)", a.list.VisibleCount()))
	}
	return a, nil
}

// stripPrefixWord returns the remainder after the first whitespace-delimited
// word if it equals prefix. ("submit --dry-run", "submit") -> ("--dry-run", true)
// ("submit", "submit") -> ("", true). Otherwise returns ("", false).
func stripPrefixWord(input, prefix string) (string, bool) {
	if input == prefix {
		return "", true
	}
	if strings.HasPrefix(input, prefix+" ") {
		return input[len(prefix)+1:], true
	}
	return "", false
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
	case viewEdit:
		body = a.edit.View()
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
func (a *App) IsEdit() bool    { return a.state == viewEdit }

// Run starts the bubbletea program.
func Run(cfg Config) error {
	p := tea.NewProgram(NewApp(cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
