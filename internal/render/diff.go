package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/diff"
)

// Diff palette. Backgrounds are kept off so the gutter still reads against
// the surrounding panel; we only color the prefix + foreground.
var (
	diffAddStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("114")) // green
	diffDeleteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red
	diffHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true)
	diffGutterStyle = lipgloss.NewStyle().Faint(true)
	diffMarkerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("228")) // yellow
)

// DiffHunkOptions tunes hunk rendering. Anchor* fields, when non-zero,
// cause matching lines to be marked with a ▶ in the gutter so the reader
// can see where the comment's start and end land within the hunk.
type DiffHunkOptions struct {
	AnchorOldLine int  // pre-image line to highlight, 0 to disable
	AnchorNewLine int  // post-image line to highlight, 0 to disable
}

// DiffHunk renders a single hunk in unified-diff style with `+`/`-`/context
// prefixes, lipgloss-driven colors, and anchor markers. Output lines look
// like:
//
//	@@ -1,5 +1,6 @@ func Bar()
//	  1   1   package foo
//	▶ 3   -   func Old() {}
//	▶     3 + func New() {}
//	      4 + var Added int
//	  5   6   // tail
//
// The two leading number columns are old / new line numbers (blank when
// the line does not exist on that side). The third column is the +/-/space
// prefix.
func DiffHunk(h diff.Hunk, opts DiffHunkOptions) string {
	var out strings.Builder

	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	if h.Header != "" {
		header += " " + h.Header
	}
	out.WriteString(diffHeaderStyle.Render(header))
	out.WriteByte('\n')

	for _, line := range h.Lines {
		writeDiffLine(&out, line, opts)
	}
	return out.String()
}

func writeDiffLine(out *strings.Builder, l diff.Line, opts DiffHunkOptions) {
	marker := "  "
	switch {
	case opts.AnchorOldLine != 0 && l.Kind == diff.LineDelete && l.OldLine == opts.AnchorOldLine:
		marker = diffMarkerStyle.Render("▶ ")
	case opts.AnchorNewLine != 0 && l.Kind == diff.LineAdd && l.NewLine == opts.AnchorNewLine:
		marker = diffMarkerStyle.Render("▶ ")
	case opts.AnchorOldLine != 0 && l.Kind == diff.LineContext && l.OldLine == opts.AnchorOldLine:
		marker = diffMarkerStyle.Render("▶ ")
	case opts.AnchorNewLine != 0 && l.Kind == diff.LineContext && l.NewLine == opts.AnchorNewLine:
		marker = diffMarkerStyle.Render("▶ ")
	}

	gutter := diffGutterStyle.Render(fmt.Sprintf("%4s %4s ",
		formatLineNum(l.OldLine), formatLineNum(l.NewLine)))

	body := fmt.Sprintf("%c %s", byte(l.Kind), l.Text)
	switch l.Kind {
	case diff.LineAdd:
		body = diffAddStyle.Render(body)
	case diff.LineDelete:
		body = diffDeleteStyle.Render(body)
	}

	out.WriteString(marker)
	out.WriteString(gutter)
	out.WriteString(body)
	out.WriteByte('\n')
}

func formatLineNum(n int) string {
	if n == 0 {
		return "    "
	}
	return fmt.Sprintf("%4d", n)
}
