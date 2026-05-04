package views

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/diff"
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
	// PreImage fetches the base-commit version of files for LEFT-side or
	// cross-side comments. Optional; when nil, NewDetail constructs a
	// git-backed source from the review's HeadSHA and BaseBranch.
	PreImage PreImageSource
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
	preImage            PreImageSource

	width  int
	height int

	// mdScroll is the line offset into the rendered markdown body for the
	// current comment. Reset to 0 whenever the focused comment changes.
	mdScroll int
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
	pi := s.PreImage
	if pi == nil {
		pi = NewGitPreImage(repoRoot, r.PR.HeadSHA, r.PR.BaseBranch)
	}
	return &Detail{
		keys:                km,
		review:              r,
		repoRoot:            repoRoot,
		index:               clampIndex(index, len(r.Comments)),
		codeContextLines:    s.CodeContextLines,
		horizontalThreshold: s.HorizontalThreshold,
		preImage:            pi,
	}
}

func (d *Detail) Init() tea.Cmd { return nil }

// SetIndex changes the focused comment. Useful when the parent app re-enters
// detail view at a different position.
func (d *Detail) SetIndex(i int) {
	d.index = clampIndex(i, len(d.review.Comments))
	d.mdScroll = 0
}

func (d *Detail) Index() int { return d.index }

func (d *Detail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = m.Width
		d.height = m.Height
	case tea.KeyMsg:
		// Bare arrow / page keys scroll the markdown pane within the
		// current comment. j/k/n/p still navigate between comments.
		switch m.String() {
		case "down":
			d.mdScroll++
			return d, nil
		case "up":
			if d.mdScroll > 0 {
				d.mdScroll--
			}
			return d, nil
		case "pgdown":
			d.mdScroll += d.markdownContentHeight() / 2
			return d, nil
		case "pgup":
			step := d.markdownContentHeight() / 2
			if d.mdScroll < step {
				d.mdScroll = 0
			} else {
				d.mdScroll -= step
			}
			return d, nil
		}
		switch {
		case key.Matches(m, d.keys.Down), m.String() == "n":
			d.index = clampIndex(d.index+1, len(d.review.Comments))
			d.mdScroll = 0
		case key.Matches(m, d.keys.Up), m.String() == "p":
			d.index = clampIndex(d.index-1, len(d.review.Comments))
			d.mdScroll = 0
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
	return style.Render("[a]ccept [r]eject [u]ndo  [n]ext [p]rev  [↑↓]scroll  [e]dit  [l]ist  [:]cmd  [q]uit")
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

	// Slice the body by mdScroll so the user can pan through long
	// comments with the arrow keys. We do the slicing here rather than
	// rely on lipgloss truncation because lipgloss only ever clips from
	// one end, which would leave the user unable to see content past the
	// pane height.
	contentHeight := height - 2 // top + bottom border
	if contentHeight < 1 {
		contentHeight = 1
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	maxScroll := len(lines) - contentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := d.mdScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + contentHeight
	if end > len(lines) {
		end = len(lines)
	}
	visible := strings.Join(lines[scroll:end], "\n")

	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width - 2).
		Height(height - 2)
	return border.Render(visible)
}

// markdownContentHeight is the number of body rows the markdown pane can
// show, mirroring the height accounting in View() / renderMarkdownPane().
// Used when pgup/pgdown need to scroll by half a page.
func (d *Detail) markdownContentHeight() int {
	bodyHeight := d.height - 2 // header + footer
	if bodyHeight < 5 {
		bodyHeight = 5
	}
	h := bodyHeight - 2 // pane border (top + bottom)
	if h < 1 {
		h = 1
	}
	return h
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
	startSide := c.Side
	if c.StartSide != nil {
		startSide = *c.StartSide
	}
	startLine, endLine := c.Line, c.Line
	if c.StartLine != nil {
		startLine = *c.StartLine
	}

	switch {
	case startSide == model.SideRight && c.Side == model.SideRight:
		// RIGHT-only: read from head SHA so the comment's line numbers
		// align with the commit the review was generated against, not
		// whatever happens to be in the user's working tree right now.
		raw, err := d.postImageContent(c.Path)
		if err != nil {
			// head SHA not set / unreachable (placeholder fixtures, shallow
			// clones, etc.). Fall back to the working tree.
			return d.degradedFallback(err, c.Path, startLine, endLine)
		}
		return render.CodeBytes(raw, c.Path, startLine, endLine, d.codeContextLines)

	case startSide == c.Side:
		// Same-side LEFT comment (single line or range). Render pre-image.
		raw, err := d.preImageContent(c.Path)
		if err != nil {
			return d.degradedFallback(err, c.Path, startLine, endLine)
		}
		return render.CodeBytes(raw, c.Path, startLine, endLine, d.codeContextLines)

	default:
		// Cross-side range. Render the underlying unified diff hunk so the
		// reader sees -/+ markers exactly as on GitHub, with the comment's
		// start and end lines anchored by ▶ in the gutter.
		out, err := d.crossSideHunk(c, startSide, startLine, endLine)
		if err != nil {
			// Diff unavailable. Show the working tree at the end line so
			// the user at least sees one side of the range.
			return d.degradedFallback(err, c.Path, endLine, endLine)
		}
		return out, nil
	}
}

// degradedFallback renders a one-line notice followed by the working tree
// at [startLine, endLine]. Used when post-image / pre-image / diff
// retrieval fails so the user gets some context instead of a wall of
// error text.
func (d *Detail) degradedFallback(cause error, path string, startLine, endLine int) (string, error) {
	notice := lipgloss.NewStyle().
		Foreground(lipgloss.Color("215")).
		Faint(true).
		Render(fmt.Sprintf("(commit から取得できないため working tree を表示: %s)", shortError(cause)))
	body, err := render.CodeRange(filepath.Join(d.repoRoot, path), startLine, endLine, d.codeContextLines)
	if err != nil {
		// Working tree also unavailable — propagate the original cause so
		// the caller can render its own placeholder.
		return "", cause
	}
	return notice + "\n\n" + body, nil
}

// shortError trims wrapped error chains down to the innermost message so
// the notice line stays readable.
func shortError(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i > 0 && i+2 < len(msg) {
		msg = msg[i+2:]
	}
	return msg
}

func (d *Detail) crossSideHunk(c *model.Comment, startSide model.Side, startLine, endLine int) (string, error) {
	if d.preImage == nil {
		return "", errors.New("pre-image source not configured")
	}
	raw, err := d.preImage.Diff(c.Path)
	if err != nil {
		return "", fmt.Errorf("fetch diff: %w", err)
	}
	hunks, err := diff.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse diff: %w", err)
	}
	h := diff.FindHunkForRange(hunks, startLine, sideByte(startSide), endLine, sideByte(c.Side))
	if h == nil {
		return "", fmt.Errorf("no hunk covers %s%d → %s%d in %s",
			startSide, startLine, c.Side, endLine, c.Path)
	}
	opts := render.DiffHunkOptions{}
	if startSide == model.SideLeft {
		opts.AnchorOldLine = startLine
	} else {
		opts.AnchorNewLine = startLine
	}
	if c.Side == model.SideLeft {
		// Endpoint LEFT — only set if it differs from start anchor.
		if opts.AnchorOldLine == 0 {
			opts.AnchorOldLine = endLine
		}
	} else {
		if opts.AnchorNewLine == 0 {
			opts.AnchorNewLine = endLine
		}
	}
	return render.DiffHunk(*h, opts), nil
}

func sideByte(s model.Side) byte {
	if s == model.SideLeft {
		return 'L'
	}
	return 'R'
}

func (d *Detail) preImageContent(path string) ([]byte, error) {
	if d.preImage == nil {
		return nil, errors.New("pre-image source not configured")
	}
	return d.preImage.Content(path)
}

func (d *Detail) postImageContent(path string) ([]byte, error) {
	if d.preImage == nil {
		return nil, errors.New("post-image source not configured")
	}
	return d.preImage.PostImage(path)
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
