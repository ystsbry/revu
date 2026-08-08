package dashboard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/store"
	"github.com/ystsbry/revu/internal/tui"
)

// prActionsLoadedMsg carries the fully loaded review for the L2 screen.
type prActionsLoadedMsg struct {
	review *model.Review
	err    error
}

// prActionsErrMsg surfaces a failed action (e.g. opening the review TUI).
type prActionsErrMsg struct{ err error }

// PRActions is the L2 screen: one reviewed PR, its metadata, and the
// actions that can be taken on it. Today the only action is opening the
// existing review TUI (L3); WS-E adds review generation and background
// jobs here.
type PRActions struct {
	slug string
	item PRItem

	width  int
	height int

	loading   bool
	err       error
	actionErr error
	review    *model.Review
	cursor    int

	load       func() (*model.Review, error)
	openReview func(*model.Review) (Screen, error)
}

func NewPRActions(slug string, item PRItem) *PRActions {
	m := &PRActions{slug: slug, item: item, loading: true}
	m.load = func() (*model.Review, error) { return store.Load(item.Path) }
	m.openReview = func(r *model.Review) (Screen, error) { return embedReviewTUI(r) }
	return m
}

// embedReviewTUI builds the existing review TUI (L3) for r and wraps it as
// a Screen. It mirrors what `revu open` wires up, with one difference: the
// dashboard is cwd-independent, so the local clone is only attached when
// cwd happens to be the review's repo. Until WS-A supplies a slug→clone
// registry, reviews opened from elsewhere just lose the code-context pane.
func embedReviewTUI(r *model.Review) (Screen, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	repoRoot := ""
	if root, err := store.VerifyRepoMatches(r.PR.Repo); err == nil {
		repoRoot = root
	}
	app := tui.NewApp(tui.Config{
		Review:   r,
		Saver:    store.SaveStatuses,
		Reloader: reloadSubmissionMeta,
		RepoRoot: repoRoot,
		Settings: tui.Settings{
			EditorCommand:       cfg.Editor.Command,
			CodeContextLines:    cfg.UI.CodeContextLines,
			HorizontalThreshold: cfg.UI.HorizontalThreshold,
		},
	})
	return Embed(fmt.Sprintf("Review #%d", r.PR.Number), app), nil
}

// reloadSubmissionMeta re-reads review.yml and copies SubmittedAt/ReviewID
// onto the in-memory Review, mirroring the reloader `revu open` installs.
func reloadSubmissionMeta(r *model.Review) error {
	newR, err := store.Load(r.BaseDir)
	if err != nil {
		return err
	}
	r.SubmittedAt = newR.SubmittedAt
	r.ReviewID = newR.ReviewID
	return nil
}

// actions returns the selectable rows. A slice even with one entry, so
// WS-E can append without reshaping the screen.
func (m *PRActions) actions() []string {
	return []string{"Open review (TUI)"}
}

func (m *PRActions) Title() string { return fmt.Sprintf("PR #%d", m.item.Number) }

func (m *PRActions) Init() tea.Cmd { return m.reload() }

func (m *PRActions) reload() tea.Cmd {
	m.loading = true
	return func() tea.Msg {
		r, err := m.load()
		return prActionsLoadedMsg{review: r, err: err}
	}
}

func (m *PRActions) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case prActionsLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.review = msg.review

	case prActionsErrMsg:
		m.actionErr = msg.err

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, Pop()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.actions())-1 {
				m.cursor++
			}
		case "r":
			return m, m.reload()
		case "enter":
			return m, m.runAction()
		}
	}
	return m, nil
}

// runAction executes the selected action. Only "open review" exists today.
func (m *PRActions) runAction() tea.Cmd {
	if m.review == nil {
		return nil
	}
	review := m.review
	open := m.openReview
	return func() tea.Msg {
		screen, err := open(review)
		if err != nil {
			return prActionsErrMsg{err: err}
		}
		return PushMsg{Screen: screen}
	}
}

func (m *PRActions) View() string {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Render(fmt.Sprintf("%s — PR #%d @ %s", m.slug, m.item.Number, m.item.ShortSHA))

	var body string
	switch {
	case m.loading:
		body = faintBox("Loading review ...")
	case m.err != nil:
		body = faintBox(fmt.Sprintf("Failed to load review: %v\n\nPress [r] to retry.", m.err))
	default:
		body = lipgloss.JoinVertical(lipgloss.Left, m.metaView(), m.actionView())
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter] run  [j/k] move  [r] reload  [esc] back")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m *PRActions) metaView() string {
	r := m.review
	counts := r.Counts()
	lines := []string{
		fmt.Sprintf("review_event: %s", r.ReviewEvent),
		fmt.Sprintf("comments:     %d (pending=%d accepted=%d rejected=%d edited=%d)",
			len(r.Comments),
			counts[model.StatusPending], counts[model.StatusAccepted],
			counts[model.StatusRejected], counts[model.StatusEdited]),
	}
	if r.SubmittedAt != nil {
		lines = append(lines, fmt.Sprintf("submitted:    %s", r.SubmittedAt.Format("2006-01-02 15:04")))
	} else {
		lines = append(lines, "submitted:    (not yet)")
	}
	lines = append(lines, fmt.Sprintf("dir:          %s", m.item.Path))
	return lipgloss.NewStyle().Faint(true).Padding(1, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m *PRActions) actionView() string {
	rows := make([]string, 0, len(m.actions())+1)
	for i, a := range m.actions() {
		rows = append(rows, cursorRow(a, i == m.cursor))
	}
	if m.actionErr != nil {
		rows = append(rows, "", lipgloss.NewStyle().Faint(true).
			Render(fmt.Sprintf("action failed: %v", m.actionErr)))
	}
	return lipgloss.NewStyle().Padding(0, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
