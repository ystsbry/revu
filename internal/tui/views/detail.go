package views

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/render"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// DefaultHorizontalThreshold is the minimum terminal width at which the
// detail view uses a side-by-side split when no Settings override is given.
const DefaultHorizontalThreshold = 100

// DefaultCodeContextLines is how many lines of source surrounding the target
// line the code pane shows when no Settings override is given.
const DefaultCodeContextLines = 5

// DetailSettings tunes the detail view rendering. Zero values fall back to
// the package-level defaults.
type DetailSettings struct {
	CodeContextLines    int
	HorizontalThreshold int
}

// Detail is a tea.Model rendering one comment: code excerpt + markdown body.
// It mutates the underlying Review.Comments[*].Status and emits DirtyMsg
// when the user accepts/rejects/unflags.
type Detail struct {
	keys                keys.KeyMap
	review              *model.Review
	repoRoot            string
	index               int
	codeContextLines    int
	horizontalThreshold int

	width  int
	height int
}

// GoToListMsg requests the parent app to return to the list view.
type GoToListMsg struct{}

// EditMsg requests the parent app to open the body_file in $EDITOR.
type EditMsg struct {
	Path string // absolute path to the file to edit
}

func NewDetail(r *model.Review, repoRoot string, km keys.KeyMap, index int, s DetailSettings) *Detail {
	if s.CodeContextLines <= 0 {
		s.CodeContextLines = DefaultCodeContextLines
	}
	if s.HorizontalThreshold <= 0 {
		s.HorizontalThreshold = DefaultHorizontalThreshold
	}
	return &Detail{
		keys:                km,
		review:              r,
		repoRoot:            repoRoot,
		index:               clampIndex(index, len(r.Comments)),
		codeContextLines:    s.CodeContextLines,
		horizontalThreshold: s.HorizontalThreshold,
	}
}

func (d *Detail) Init() tea.Cmd { return nil }

// SetIndex changes the focused comment. Useful when the parent app re-enters
// detail view at a different position.
func (d *Detail) SetIndex(i int) {
	d.index = clampIndex(i, len(d.review.Comments))
}

func (d *Detail) Index() int { return d.index }

func (d *Detail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = m.Width
		d.height = m.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(m, d.keys.Down), m.String() == "n":
			d.index = clampIndex(d.index+1, len(d.review.Comments))
		case key.Matches(m, d.keys.Up), m.String() == "p":
			d.index = clampIndex(d.index-1, len(d.review.Comments))
		case key.Matches(m, d.keys.Accept):
			if c := d.current(); c != nil {
				c.Status = model.StatusAccepted
				return d, dirty()
			}
		case key.Matches(m, d.keys.Reject):
			if c := d.current(); c != nil {
				c.Status = model.StatusRejected
				return d, dirty()
			}
		case key.Matches(m, d.keys.Pending):
			if c := d.current(); c != nil {
				c.Status = model.StatusPending
				return d, dirty()
			}
		case m.String() == "l":
			return d, func() tea.Msg { return GoToListMsg{} }
		case m.String() == "e":
			if c := d.current(); c != nil {
				path := filepath.Join(d.review.BaseDir, c.BodyFile)
				return d, func() tea.Msg { return EditMsg{Path: path} }
			}
		}
	}
	return d, nil
}

func (d *Detail) View() string {
	c := d.current()
	if c == nil {
		return "no comments"
	}

	header := d.headerView(c)
	footer := d.footerView()

	// Reserve 1 line for header and 1 for footer.
	bodyHeight := d.height - 2
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	codePane := d.renderCodePane(c, bodyHeight)
	mdPane := d.renderMarkdownPane(c, bodyHeight)

	var body string
	if d.width >= d.horizontalThreshold {
		body = lipgloss.JoinHorizontal(lipgloss.Top, codePane, mdPane)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, codePane, mdPane)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (d *Detail) headerView(c *model.Comment) string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	pos := fmt.Sprintf("%d / %d", d.index+1, len(d.review.Comments))
	return style.Render(fmt.Sprintf("%s — %s   %s:%s   %s / %s   [%s]",
		c.ID, pos, c.Path, c.LineLabel(), c.Severity, c.Category, c.Status))
}

func (d *Detail) footerView() string {
	style := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	return style.Render("[a]ccept [r]eject [u]ndo  [n]ext [p]rev  [e]dit  [l]ist  [:]cmd  [q]uit")
}

func (d *Detail) renderCodePane(c *model.Comment, height int) string {
	width := d.paneWidth()

	body, err := d.codeContent(c)
	if err != nil {
		body = lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("(code unavailable: %v)", err))
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width - 2).
		Height(height - 2)
	return border.Render(body)
}

func (d *Detail) renderMarkdownPane(c *model.Comment, height int) string {
	width := d.paneWidth()
	body, err := render.Markdown(c.Body, width-4) // -4 for borders + padding
	if err != nil {
		body = c.Body
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width - 2).
		Height(height - 2)
	return border.Render(body)
}

func (d *Detail) paneWidth() int {
	if d.width >= d.horizontalThreshold {
		return d.width / 2
	}
	return d.width
}

func (d *Detail) codeContent(c *model.Comment) (string, error) {
	if d.repoRoot == "" {
		return "", errors.New("repo root not configured")
	}
	abs := filepath.Join(d.repoRoot, c.Path)
	return render.Code(abs, c.Line, d.codeContextLines)
}

func (d *Detail) current() *model.Comment {
	if d.index < 0 || d.index >= len(d.review.Comments) {
		return nil
	}
	return &d.review.Comments[d.index]
}

func clampIndex(i, n int) int {
	if n == 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}
