package dashboard

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// breadcrumbHeight is the number of rows Root reserves at the top for the
// breadcrumb trail. Screens are sized for the remaining space.
const breadcrumbHeight = 1

// PushMsg asks the dashboard to push Screen onto the navigation stack.
type PushMsg struct{ Screen Screen }

// PopMsg asks the dashboard to drop the active screen and reveal the one
// beneath it. Ignored at the root screen, which has nothing to fall back to.
type PopMsg struct{}

// Push returns a command that navigates into s.
func Push(s Screen) tea.Cmd { return func() tea.Msg { return PushMsg{Screen: s} } }

// Pop returns a command that navigates back to the previous screen.
func Pop() tea.Cmd { return func() tea.Msg { return PopMsg{} } }

// Root is the dashboard's top-level tea.Model. It routes messages to the
// active screen, handles push/pop navigation, and draws the breadcrumb.
type Root struct {
	stack  screenStack
	width  int
	height int
}

// NewRoot builds the dashboard around initial, which becomes the bottom of
// the stack and can never be popped away.
func NewRoot(initial Screen) *Root {
	r := &Root{}
	r.stack.push(initial)
	return r
}

func (r *Root) Init() tea.Cmd {
	if active := r.stack.top(); active != nil {
		return active.Init()
	}
	return nil
}

func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = m.Width, m.Height
		return r, r.resizeAll()

	case tea.KeyMsg:
		// Ctrl+C is the only key the dashboard reserves: it must quit from
		// anywhere, including from inside an embedded screen. Everything
		// else belongs to the active screen, which owns its own key
		// semantics — the review TUI's "q", for one, warns on unsaved
		// changes rather than quitting outright.
		if m.Type == tea.KeyCtrlC {
			return r, tea.Quit
		}

	case tea.MouseMsg:
		// Screens render below the breadcrumb row and hit-test in their
		// own coordinates (the embedded review TUI scans its own view),
		// so shift the pointer up by the rows the shell owns.
		m.Y -= breadcrumbHeight
		if m.Y < 0 {
			return r, nil // breadcrumb row itself: nothing to click yet
		}
		return r, r.updateActive(m)

	case PushMsg:
		return r, r.push(m.Screen)

	case PopMsg:
		// A popped screen is gone for good (a fresh one is built on the
		// next descent), so screens holding OS resources — the PR list's
		// job-book watcher — get to release them here.
		if popped, ok := r.stack.pop(); ok {
			if closer, isCloser := popped.(io.Closer); isCloser {
				_ = closer.Close()
			}
		}
		return r, nil
	}

	return r, r.updateActive(msg)
}

func (r *Root) View() string {
	active := r.stack.top()
	if active == nil {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, r.breadcrumbView(), active.View())
}

// Depth reports how many screens are stacked. Exposed for tests.
func (r *Root) Depth() int { return r.stack.depth() }

// ActiveTitle returns the active screen's label. Exposed for tests.
func (r *Root) ActiveTitle() string {
	if active := r.stack.top(); active != nil {
		return active.Title()
	}
	return ""
}

// push adds s to the stack, sizes it for the current terminal before its
// first View, and starts whatever work its Init kicks off.
func (r *Root) push(s Screen) tea.Cmd {
	if s == nil {
		return nil
	}
	r.stack.push(s)
	return tea.Batch(r.resizeTop(), s.Init())
}

// contentSize is the space left for a screen once the breadcrumb row is
// accounted for.
func (r *Root) contentSize() tea.WindowSizeMsg {
	h := r.height - breadcrumbHeight
	if h < 0 {
		h = 0
	}
	return tea.WindowSizeMsg{Width: r.width, Height: h}
}

// resizeAll pushes the current content size through every screen on the
// stack, not just the active one, so a screen revealed by a later pop is
// already laid out for the current terminal instead of rendering at the
// size it was last shown at.
func (r *Root) resizeAll() tea.Cmd {
	if !r.sized() {
		return nil
	}
	size := r.contentSize()
	cmds := make([]tea.Cmd, 0, len(r.stack.screens))
	for i, sc := range r.stack.screens {
		next, cmd := sc.Update(size)
		if s, ok := next.(Screen); ok {
			r.stack.screens[i] = s
		}
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// resizeTop sizes only the active screen. Used right after a push, when
// the screens below it are already current.
func (r *Root) resizeTop() tea.Cmd {
	if !r.sized() {
		return nil
	}
	return r.updateActive(r.contentSize())
}

// sized reports whether a WindowSizeMsg has arrived yet. Before that there
// is no meaningful size to hand a screen.
func (r *Root) sized() bool { return r.width > 0 || r.height > 0 }

// updateActive forwards msg to the active screen and writes the returned
// model back onto the stack.
func (r *Root) updateActive(msg tea.Msg) tea.Cmd {
	active := r.stack.top()
	if active == nil {
		return nil
	}
	next, cmd := active.Update(msg)
	if s, ok := next.(Screen); ok {
		r.stack.replaceTop(s)
	}
	return cmd
}

func (r *Root) breadcrumbView() string {
	titles := r.stack.titles()
	if len(titles) == 0 {
		return ""
	}
	var b strings.Builder
	dim := lipgloss.NewStyle().Faint(true)
	active := lipgloss.NewStyle().Bold(true)
	for i, t := range titles {
		if i > 0 {
			b.WriteString(dim.Render(" › "))
		}
		if i == len(titles)-1 {
			b.WriteString(active.Render(t))
		} else {
			b.WriteString(dim.Render(t))
		}
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}
