package views

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// GoToEditMsg requests the parent app to switch to the edit view at Index.
// The detail view emits this when the user presses 'm' on a comment.
type GoToEditMsg struct{ Index int }

// editField identifies which row of the form is currently selected.
// The order mirrors the visual top-to-bottom layout: severity, category,
// then the start_line / line pair (start above end so the form reads as
// the line range itself).
type editField int

const (
	fieldSeverity editField = iota
	fieldCategory
	fieldStartLine
	fieldLine
	editFieldCount
)

// allCategories is the cycle order shown in the edit view. Mirrors the
// model.Category enum so users can reach every valid value.
var allCategories = []model.Category{
	model.CategoryBug,
	model.CategoryDesign,
	model.CategoryStyle,
	model.CategoryPerf,
	model.CategorySecurity,
	model.CategoryTest,
	model.CategoryDoc,
}

// Edit is a tea.Model for editing severity / category / start_line / line
// of one comment. Mutations are applied to the underlying review immediately
// and emit DirtyMsg, mirroring how Detail handles status changes.
type Edit struct {
	keys   keys.KeyMap
	review *model.Review
	index  int

	field editField
	// lineInput / startLineInput buffer typing for their respective fields.
	// Committed to the model on Enter / field change; on Esc the buffer is
	// reverted from the persisted value so the displayed text never drifts
	// from state.
	lineInput      textinput.Model
	startLineInput textinput.Model
	// errMsg holds the most recent validation error (e.g. start_line >= line)
	// so it can be surfaced under the form. Cleared on the next successful
	// commit or field change.
	errMsg string

	width  int
	height int
}

func NewEdit(r *model.Review, km keys.KeyMap, index int) *Edit {
	mkInput := func() textinput.Model {
		ti := textinput.New()
		ti.CharLimit = 9
		ti.Prompt = ""
		ti.Width = 8
		return ti
	}
	e := &Edit{
		keys:           km,
		review:         r,
		index:          clampIndex(index, len(r.Comments)),
		lineInput:      mkInput(),
		startLineInput: mkInput(),
	}
	e.syncInputs()
	return e
}

func (e *Edit) Init() tea.Cmd { return nil }

// SetIndex re-points the form at a different comment and resets transient
// UI state (selected field, input buffers).
func (e *Edit) SetIndex(i int) {
	e.index = clampIndex(i, len(e.review.Comments))
	e.field = fieldSeverity
	e.lineInput.Blur()
	e.startLineInput.Blur()
	e.syncInputs()
}

func (e *Edit) Index() int { return e.index }

// Field is exposed for tests so they can assert focus movement without
// relying on rendered output.
func (e *Edit) Field() editField { return e.field }

// LineInputFocused reports whether the line field is in input mode.
// Exposed for tests.
func (e *Edit) LineInputFocused() bool { return e.lineInput.Focused() }

// StartLineInputFocused reports whether the start_line field is in input
// mode. Exposed for tests.
func (e *Edit) StartLineInputFocused() bool { return e.startLineInput.Focused() }

// syncInputs refreshes the buffered text inputs from the current comment's
// state. It does NOT clear errMsg — that is the caller's responsibility,
// because validation failure paths call this to revert the buffer while
// keeping the error visible.
func (e *Edit) syncInputs() {
	c := e.current()
	if c == nil {
		e.lineInput.SetValue("")
		e.startLineInput.SetValue("")
		return
	}
	e.lineInput.SetValue(strconv.Itoa(c.Line))
	if c.StartLine != nil {
		e.startLineInput.SetValue(strconv.Itoa(*c.StartLine))
	} else {
		e.startLineInput.SetValue("")
	}
}

func (e *Edit) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		e.width = m.Width
		e.height = m.Height
		return e, nil
	case tea.KeyMsg:
		// While typing into a numeric input, route keystrokes to it first
		// so digit / backspace / cursor keys behave as expected.
		if e.field == fieldLine && e.lineInput.Focused() {
			return e.updateLineInput(m)
		}
		if e.field == fieldStartLine && e.startLineInput.Focused() {
			return e.updateStartLineInput(m)
		}
		return e.updateNav(m)
	}
	return e, nil
}

func (e *Edit) updateNav(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.String() {
	case "esc":
		return e, e.leaveToDetail()
	case "j", "down":
		e.moveField(+1)
		return e, nil
	case "k", "up":
		e.moveField(-1)
		return e, nil
	case "tab":
		e.moveField(+1)
		return e, nil
	case "shift+tab":
		e.moveField(-1)
		return e, nil
	case "h", "left":
		return e, e.cycleField(-1)
	case "l", "right":
		return e, e.cycleField(+1)
	case "d":
		// 'd' on the start_line field clears it (single-line). On other
		// fields it does nothing — kept narrow to avoid stealing keys
		// from future editable fields.
		if e.field == fieldStartLine {
			return e, e.clearStartLine()
		}
		return e, nil
	case "enter":
		switch e.field {
		case fieldLine:
			e.lineInput.Focus()
			e.lineInput.CursorEnd()
			return e, textinput.Blink
		case fieldStartLine:
			e.startLineInput.Focus()
			e.startLineInput.CursorEnd()
			return e, textinput.Blink
		}
		return e, e.cycleField(+1)
	}
	return e, nil
}

func (e *Edit) updateLineInput(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.Type {
	case tea.KeyEnter, tea.KeyTab:
		cmd := e.commitLine()
		e.lineInput.Blur()
		return e, cmd
	case tea.KeyEsc:
		e.lineInput.Blur()
		e.syncInputs()
		return e, nil
	}
	if !isDigitKey(m) {
		return e, nil
	}
	var cmd tea.Cmd
	e.lineInput, cmd = e.lineInput.Update(m)
	return e, cmd
}

func (e *Edit) updateStartLineInput(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.Type {
	case tea.KeyEnter, tea.KeyTab:
		cmd := e.commitStartLine()
		e.startLineInput.Blur()
		return e, cmd
	case tea.KeyEsc:
		e.startLineInput.Blur()
		e.syncInputs()
		return e, nil
	}
	if !isDigitKey(m) {
		return e, nil
	}
	var cmd tea.Cmd
	e.startLineInput, cmd = e.startLineInput.Update(m)
	return e, cmd
}

// isDigitKey returns true for editing keys (backspace etc.) and digit
// runes. Used to filter out alphabetic input so the buffer always parses.
func isDigitKey(m tea.KeyMsg) bool {
	if m.Type != tea.KeyRunes {
		return true
	}
	for _, r := range m.Runes {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// moveField changes the focused row. If a numeric input is currently being
// edited, its buffer is committed first so the user does not silently lose
// in-flight typing when navigating away.
func (e *Edit) moveField(delta int) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case e.field == fieldLine && e.lineInput.Focused():
		cmd = e.commitLine()
		e.lineInput.Blur()
	case e.field == fieldStartLine && e.startLineInput.Focused():
		cmd = e.commitStartLine()
		e.startLineInput.Blur()
	}
	next := int(e.field) + delta
	if next < 0 {
		next = 0
	}
	if next >= int(editFieldCount) {
		next = int(editFieldCount) - 1
	}
	if e.field != editField(next) {
		e.errMsg = ""
	}
	e.field = editField(next)
	e.syncInputs()
	return cmd
}

func (e *Edit) cycleField(delta int) tea.Cmd {
	c := e.current()
	if c == nil {
		return nil
	}
	switch e.field {
	case fieldSeverity:
		next := cycleSeverity(c.Severity, delta)
		if next == c.Severity {
			return nil
		}
		c.Severity = next
		return dirty()
	case fieldCategory:
		next := cycleCategory(c.Category, delta)
		if next == c.Category {
			return nil
		}
		c.Category = next
		return dirty()
	case fieldStartLine:
		return e.cycleStartLine(c, delta)
	case fieldLine:
		return e.cycleLine(c, delta)
	}
	return nil
}

// cycleLine moves c.Line by delta. Clamps at 1 and, when a start_line is
// set, refuses to push line down to/below it (range would be invalid). When
// the resulting line equals start_line we drop the range entirely so the
// user never observes an invalid intermediate state.
func (e *Edit) cycleLine(c *model.Comment, delta int) tea.Cmd {
	v := c.Line + delta
	if v < 1 {
		v = 1
	}
	if v == c.Line {
		return nil
	}
	c.Line = v
	if c.StartLine != nil && *c.StartLine >= c.Line {
		// Single-line collapse: range no longer makes sense.
		c.StartLine = nil
		c.StartSide = nil
	}
	e.errMsg = ""
	e.syncInputs()
	return dirty()
}

// cycleStartLine handles -1/+1 with two boundary transitions:
//   - delta = -1 from start_line=1  -> clear (single-line)
//   - delta = +1 from no start_line -> initialize at max(1, line-1)
func (e *Edit) cycleStartLine(c *model.Comment, delta int) tea.Cmd {
	if c.StartLine == nil {
		if delta <= 0 {
			return nil
		}
		init := c.Line - 1
		if init < 1 {
			return nil
		}
		c.StartLine = &init
		e.errMsg = ""
		e.syncInputs()
		return dirty()
	}
	v := *c.StartLine + delta
	if v < 1 {
		// Convert range -> single-line.
		c.StartLine = nil
		c.StartSide = nil
		e.errMsg = ""
		e.syncInputs()
		return dirty()
	}
	if v >= c.Line {
		e.errMsg = fmt.Sprintf("start_line must be < line (%d)", c.Line)
		return nil
	}
	c.StartLine = &v
	e.errMsg = ""
	e.syncInputs()
	return dirty()
}

func (e *Edit) clearStartLine() tea.Cmd {
	c := e.current()
	if c == nil || c.StartLine == nil {
		return nil
	}
	c.StartLine = nil
	c.StartSide = nil
	e.errMsg = ""
	e.syncInputs()
	return dirty()
}

func (e *Edit) commitLine() tea.Cmd {
	c := e.current()
	if c == nil {
		return nil
	}
	raw := strings.TrimSpace(e.lineInput.Value())
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		e.errMsg = fmt.Sprintf("invalid line %q (must be a positive integer)", raw)
		e.lineInput.SetValue(strconv.Itoa(c.Line))
		return nil
	}
	if c.StartLine != nil && v <= *c.StartLine {
		e.errMsg = fmt.Sprintf("line must be > start_line (%d)", *c.StartLine)
		e.lineInput.SetValue(strconv.Itoa(c.Line))
		return nil
	}
	e.errMsg = ""
	if v == c.Line {
		return nil
	}
	c.Line = v
	return dirty()
}

func (e *Edit) commitStartLine() tea.Cmd {
	c := e.current()
	if c == nil {
		return nil
	}
	raw := strings.TrimSpace(e.startLineInput.Value())
	if raw == "" {
		// Empty input means "no start_line" — drop the range.
		if c.StartLine == nil {
			e.errMsg = ""
			return nil
		}
		c.StartLine = nil
		c.StartSide = nil
		e.errMsg = ""
		return dirty()
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		e.errMsg = fmt.Sprintf("invalid start_line %q (must be a positive integer or empty)", raw)
		e.syncInputs()
		return nil
	}
	if v >= c.Line {
		e.errMsg = fmt.Sprintf("start_line must be < line (%d)", c.Line)
		e.syncInputs()
		return nil
	}
	if c.StartLine != nil && *c.StartLine == v {
		e.errMsg = ""
		return nil
	}
	c.StartLine = &v
	e.errMsg = ""
	return dirty()
}

func (e *Edit) leaveToDetail() tea.Cmd {
	// Commit any pending numeric edit so the user does not lose typed input
	// just because they hit Esc to return to detail.
	var cmd tea.Cmd
	switch {
	case e.field == fieldLine && e.lineInput.Focused():
		cmd = e.commitLine()
		e.lineInput.Blur()
	case e.field == fieldStartLine && e.startLineInput.Focused():
		cmd = e.commitStartLine()
		e.startLineInput.Blur()
	}
	idx := e.index
	leave := func() tea.Msg { return GoToDetailMsg{Index: idx} }
	if cmd == nil {
		return leave
	}
	return tea.Batch(cmd, leave)
}

func cycleSeverity(current model.Severity, delta int) model.Severity {
	defs := model.ActiveSeverityRegistry().All()
	if len(defs) == 0 {
		return current
	}
	idx := 0
	for i, d := range defs {
		if d.Name == string(current) {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(defs)) % len(defs)
	return model.Severity(defs[idx].Name)
}

func cycleCategory(current model.Category, delta int) model.Category {
	idx := 0
	for i, c := range allCategories {
		if c == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(allCategories)) % len(allCategories)
	return allCategories[idx]
}

func (e *Edit) View() string {
	c := e.current()
	if c == nil {
		return "no comments"
	}

	header := lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(
		fmt.Sprintf("Edit — %s   %s:%s   [%s]", c.ID, c.Path, c.LineLabel(), c.Status),
	)

	rows := []string{
		e.renderSeverityRow(c),
		e.renderCategoryRow(c),
		e.renderStartLineRow(c),
		e.renderLineRow(c),
	}
	form := lipgloss.JoinVertical(lipgloss.Left, rows...)

	formBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Render(form)

	hint := lipgloss.NewStyle().Faint(true).Padding(0, 1).Render(
		"[j/k]field  [h/l/←→]cycle / -1+1  [Enter]edit  [d]rop start  [Esc]back  [?]help",
	)

	if e.errMsg != "" {
		errLine := lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Padding(0, 1).
			Render(e.errMsg)
		return lipgloss.JoinVertical(lipgloss.Left, header, formBox, errLine, hint)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, formBox, hint)
}

func (e *Edit) renderSeverityRow(c *model.Comment) string {
	defs := model.ActiveSeverityRegistry().All()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return e.renderEnumRow("severity", string(c.Severity), names, e.field == fieldSeverity)
}

func (e *Edit) renderCategoryRow(c *model.Comment) string {
	names := make([]string, 0, len(allCategories))
	for _, cat := range allCategories {
		names = append(names, string(cat))
	}
	return e.renderEnumRow("category", string(c.Category), names, e.field == fieldCategory)
}

// renderEnumRow shows label + every option, with the selected option boxed
// and the focused row prefixed by a marker so the user can see at a glance
// which row arrow keys will affect.
func (e *Edit) renderEnumRow(label, current string, options []string, focused bool) string {
	marker := "  "
	if focused {
		marker = "▶ "
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Width(12)
	parts := []string{marker + labelStyle.Render(label)}
	for _, opt := range options {
		if opt == current {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("57")).
				Padding(0, 1).
				Render(opt))
		} else {
			parts = append(parts, lipgloss.NewStyle().
				Faint(true).
				Padding(0, 1).
				Render(opt))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (e *Edit) renderStartLineRow(c *model.Comment) string {
	marker := "  "
	if e.field == fieldStartLine {
		marker = "▶ "
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Width(12)
	var valueRender string
	switch {
	case e.field == fieldStartLine && e.startLineInput.Focused():
		valueRender = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1).
			Render(e.startLineInput.View())
	case c.StartLine == nil:
		valueRender = lipgloss.NewStyle().
			Faint(true).
			Padding(0, 1).
			Render("(none)")
	default:
		valueRender = lipgloss.NewStyle().
			Padding(0, 1).
			Render(strconv.Itoa(*c.StartLine))
	}
	hint := lipgloss.NewStyle().Faint(true).Padding(0, 1).Render(
		"(range start; empty = single-line, 'd' to drop)",
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, marker+labelStyle.Render("start_line"), valueRender, hint)
}

func (e *Edit) renderLineRow(c *model.Comment) string {
	marker := "  "
	if e.field == fieldLine {
		marker = "▶ "
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Width(12)
	var valueRender string
	if e.field == fieldLine && e.lineInput.Focused() {
		valueRender = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1).
			Render(e.lineInput.View())
	} else {
		valueRender = lipgloss.NewStyle().
			Padding(0, 1).
			Render(strconv.Itoa(c.Line))
	}
	hint := lipgloss.NewStyle().Faint(true).Padding(0, 1).Render(
		"(side=" + string(c.Side) + "; range end / single line)",
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, marker+labelStyle.Render("line"), valueRender, hint)
}

func (e *Edit) current() *model.Comment {
	if e.index < 0 || e.index >= len(e.review.Comments) {
		return nil
	}
	return &e.review.Comments[e.index]
}
