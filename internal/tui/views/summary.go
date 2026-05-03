package views

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/render"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// Summary renders the PR-level summary.md and lets the user cycle review_event.
type Summary struct {
	keys   keys.KeyMap
	review *model.Review

	width  int
	height int
}

func NewSummary(r *model.Review, km keys.KeyMap) *Summary {
	return &Summary{keys: km, review: r}
}

func (s *Summary) Init() tea.Cmd { return nil }

func (s *Summary) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.height = m.Height
	case tea.KeyMsg:
		switch {
		case m.String() == "l":
			return s, func() tea.Msg { return GoToListMsg{} }
		case m.String() == "c":
			s.cycleEvent()
			return s, dirty()
		case m.String() == "e":
			path := filepath.Join(s.review.BaseDir, s.review.SummaryFile)
			return s, func() tea.Msg { return EditMsg{Path: path} }
		case key.Matches(m, s.keys.Help):
			// no-op for now; help screen comes later
		}
	}
	return s, nil
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
	body := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(s.width - 2).
		Height(bodyHeight).
		Render(rendered)

	radio := s.eventRadio()
	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[c]ycle event  [e]dit  [l]ist  [:]cmd  [q]uit")

	return lipgloss.JoinVertical(lipgloss.Left, header, body, radio, footer)
}

func (s *Summary) eventRadio() string {
	mark := func(e model.ReviewEvent) string {
		if s.review.ReviewEvent == e {
			return "(●)"
		}
		return "( )"
	}
	line := fmt.Sprintf("Event: %s APPROVE  %s COMMENT  %s REQUEST_CHANGES",
		mark(model.EventApprove), mark(model.EventComment), mark(model.EventRequestChanges))
	return lipgloss.NewStyle().Padding(0, 1).Render(line)
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
