package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/render"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// Summary renders the PR-level summary.md and lets the user set the
// review_event — by cycling with "c" or clicking an event directly.
type Summary struct {
	keys   keys.KeyMap
	review *model.Review
	zones  *zone.Manager

	// scroll is the line offset into the rendered summary body; maxScroll
	// is refreshed by every View render.
	scroll    int
	maxScroll int

	width  int
	height int
}

func NewSummary(r *model.Review, km keys.KeyMap) *Summary {
	return &Summary{keys: km, review: r}
}

// AttachZones enables mouse hit-testing through the app's zone manager.
func (s *Summary) AttachZones(z *zone.Manager) { s.zones = z }

func (s *Summary) Init() tea.Cmd { return nil }

func (s *Summary) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.height = m.Height
	case tea.MouseMsg:
		return s.updateMouse(m)
	case tea.KeyMsg:
		switch {
		case m.String() == "down":
			s.scrollBy(+1)
		case m.String() == "up":
			s.scrollBy(-1)
		case m.String() == "l":
			return s, func() tea.Msg { return GoToListMsg{} }
		case m.String() == "c":
			s.cycleEvent()
			return s, dirty()
		case m.String() == "e":
			return s, s.editSummary()
		case key.Matches(m, s.keys.Help):
			// handled by the app-level help overlay
		}
	}
	return s, nil
}

func (s *Summary) updateMouse(m tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.Button == tea.MouseButtonWheelUp:
		s.scrollBy(-3)
		return s, nil
	case m.Button == tea.MouseButtonWheelDown:
		s.scrollBy(+3)
		return s, nil
	case !leftClick(m):
		return s, nil
	}

	for _, ev := range []model.ReviewEvent{model.EventApprove, model.EventComment, model.EventRequestChanges} {
		if hit(s.zones, ZoneSummaryEventPrefix+string(ev), m) {
			if s.review.ReviewEvent != ev {
				s.review.ReviewEvent = ev
				return s, dirty()
			}
			return s, nil
		}
	}
	switch {
	case hit(s.zones, ZoneSummaryEdit, m):
		return s, s.editSummary()
	case hit(s.zones, ZoneSummaryList, m):
		return s, func() tea.Msg { return GoToListMsg{} }
	}
	return s, nil
}

func (s *Summary) scrollBy(delta int) {
	s.scroll += delta
	if s.scroll > s.maxScroll {
		s.scroll = s.maxScroll
	}
	if s.scroll < 0 {
		s.scroll = 0
	}
}

func (s *Summary) editSummary() tea.Cmd {
	path := filepath.Join(s.review.BaseDir, s.review.SummaryFile)
	return func() tea.Msg { return EditMsg{Path: path} }
}

func (s *Summary) View() string {
	header := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Render(fmt.Sprintf("Summary — %s #%d", s.review.PR.Repo, s.review.PR.Number))

	bodyWidth := s.width - 4
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	rendered, err := render.Markdown(s.review.SummaryBody, bodyWidth)
	if err != nil {
		rendered = s.review.SummaryBody
	}

	bodyHeight := s.height - 5
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	// Scrollable window over the rendered body, mirroring the detail
	// view's markdown pane: wrap first so scroll units are real rows.
	contentWidth := s.width - 6 // border (2) + padding (2) + slack
	if contentWidth < 20 {
		contentWidth = 20
	}
	wrapped := lipgloss.NewStyle().Width(contentWidth).Render(rendered)
	lines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	s.maxScroll = max(0, len(lines)-bodyHeight)
	if s.scroll > s.maxScroll {
		s.scroll = s.maxScroll
	}
	end := min(s.scroll+bodyHeight, len(lines))
	visible := strings.Join(lines[s.scroll:end], "\n")

	body := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(s.width - 2).
		Height(bodyHeight).
		Render(visible)

	radio := s.eventRadio()
	footer := s.footerView()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, radio, footer)
}

// eventRadio renders the review-event selector; each option is a click
// target that sets the event directly.
func (s *Summary) eventRadio() string {
	option := func(e model.ReviewEvent) string {
		radio := "( )"
		if s.review.ReviewEvent == e {
			radio = "(●)"
		}
		return mark(s.zones, ZoneSummaryEventPrefix+string(e), fmt.Sprintf("%s %s", radio, e))
	}
	line := "Event: " + option(model.EventApprove) + "  " + option(model.EventComment) + "  " + option(model.EventRequestChanges)
	return lipgloss.NewStyle().Padding(0, 1).Render(line)
}

func (s *Summary) footerView() string {
	btn := lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("252"))
	bar := []string{
		mark(s.zones, ZoneSummaryEdit, btn.Render("✎ edit (e)")),
		mark(s.zones, ZoneSummaryList, btn.Render("≡ list (l)")),
	}
	hint := lipgloss.NewStyle().Faint(true).Render("  click event to set  [c]ycle  wheel/[↑↓]scroll  [:]cmd [?]help")
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(bar, " ") + hint)
}

func (s *Summary) cycleEvent() {
	switch s.review.ReviewEvent {
	case model.EventApprove:
		s.review.ReviewEvent = model.EventComment
	case model.EventComment:
		s.review.ReviewEvent = model.EventRequestChanges
	case model.EventRequestChanges:
		s.review.ReviewEvent = model.EventApprove
	default:
		s.review.ReviewEvent = model.EventComment
	}
}
