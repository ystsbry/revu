package dashboard

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/jobs"
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

// prActionsJobMsg carries the PR's newest background job and its log tail.
type prActionsJobMsg struct {
	job  *jobs.Job
	tail []string
}

// jobTickMsg re-reads a running job so its state and log tail track the
// worker live.
type jobTickMsg struct{}

// jobStartedMsg reports a background-review start attempted from L2.
type jobStartedMsg struct {
	job jobs.Job
	err error
}

// execDoneMsg arrives when a terminal-handover action (foreground review,
// session resume) returns control to the dashboard.
type execDoneMsg struct{ err error }

// prAction is one selectable row of the L2 action panel.
type prAction struct {
	id    string
	label string
}

// jobLogTailLines is how many log lines the L2 screen shows.
const jobLogTailLines = 8

// PRActions is the L2 screen: one PR, its local review state, and the
// actions that can be taken on it. A PR without a local review shows what
// is known from GitHub and no actions; review generation lands here with
// WS-E.
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

	job     *jobs.Job
	logTail []string
	notice  string

	load       func() (*model.Review, error)
	loadJob    func() (*jobs.Job, []string)
	openReview func(*model.Review) (Screen, error)
	// startJob launches a background review in workDir; runExec hands the
	// terminal to an external command and returns on its exit. Both are
	// swapped out in tests.
	startJob func(workDir string) (jobs.Job, error)
	runExec  func(argv []string, dir string) tea.Cmd
}

func NewPRActions(slug string, item PRItem) *PRActions {
	m := &PRActions{slug: slug, item: item}
	m.openReview = func(r *model.Review) (Screen, error) { return embedReviewTUI(r) }
	m.loadJob = func() (*jobs.Job, []string) { return loadJobInfo(slug, item.Number) }
	m.startJob = func(workDir string) (jobs.Job, error) {
		// The config default model applies here; per-action model
		// selection stays out of scope until the UI needs it.
		model := ""
		if cfg, _, err := config.Load(); err == nil {
			model = cfg.Review.ClaudeModel
		}
		return jobs.StartReview(jobs.StartReviewOptions{
			Slug: slug, PR: item.Number, Engine: "claude", Model: model, WorkDir: workDir,
		})
	}
	m.runExec = runExternal
	if item.ReviewedPath == "" {
		// No review was known when the row was built, but one may appear
		// while this screen is up (a foreground run, a finishing job) —
		// so discovery goes through the store instead of returning nil.
		m.load = func() (*model.Review, error) {
			dir, err := store.LatestReviewDirForPR(slug, item.Number)
			if err != nil {
				return nil, nil // no review yet: a state, not an error
			}
			return store.Load(dir)
		}
		return m
	}
	m.loading = true
	m.load = func() (*model.Review, error) { return store.Load(item.ReviewedPath) }
	return m
}

// runExternal hands the terminal to argv running in dir and reports back
// when it exits. This is how foreground reviews and session resumes leave
// the dashboard: bubbletea releases the tty, the agent owns it until it
// returns, and the dashboard resumes where it was.
func runExternal(argv []string, dir string) tea.Cmd {
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err} })
}

// cloneDirFor resolves the local clone actions run in: cwd when it is the
// repo, else the registered [[repo]] path. Unlike the L3 embed (which
// tolerates a missing clone by dropping the code pane), run actions need a
// real tree, so absence is an error with the fix spelled out.
func cloneDirFor(slug string) (string, error) {
	if root, err := store.VerifyRepoMatches(slug); err == nil {
		return root, nil
	}
	cfg, _, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	def, ok := cfg.FindRepo(slug)
	if !ok {
		return "", fmt.Errorf("%s is not registered — run `revu repo add <path>` (or `revu repo scan`) first", slug)
	}
	if st, err := os.Stat(def.Path); err != nil || !st.IsDir() {
		return "", fmt.Errorf("registered clone for %s not found at %s", slug, def.Path)
	}
	return def.Path, nil
}

// loadJobInfo fetches the newest background job for slug#pr along with the
// tail of its log.
func loadJobInfo(slug string, pr int) (*jobs.Job, []string) {
	j, ok := jobs.LatestForPR(slug, pr)
	if !ok {
		return nil, nil
	}
	return &j, tailLines(j.LogPath, jobLogTailLines)
}

// tailLines returns the last n lines of the file at path, reading at most
// the trailing 32KB so a chatty agent log stays cheap to poll.
func tailLines(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	const window = 32 * 1024
	var offset int64
	if st, err := f.Stat(); err == nil && st.Size() > window {
		offset = st.Size() - window
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:] // first line is likely cut mid-way by the window
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// embedReviewTUI builds the existing review TUI (L3) for r and wraps it as
// a Screen. It mirrors what `revu open` wires up. The local clone is
// resolved like `revu open` too: cwd when it matches the review's repo,
// else the registered [[repo]] path; with neither, the code-context pane
// is simply absent.
func embedReviewTUI(r *model.Review) (Screen, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	repoRoot := ""
	if root, err := store.VerifyRepoMatches(r.PR.Repo); err == nil {
		repoRoot = root
	} else if def, ok := cfg.FindRepo(r.PR.Repo); ok {
		if st, statErr := os.Stat(def.Path); statErr == nil && st.IsDir() {
			repoRoot = def.Path
		}
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

// actions returns the selectable rows for the current state. Labels for
// foreground/session actions spell out that they hand the terminal to the
// agent — the dashboard is gone until that process exits.
func (m *PRActions) actions() []prAction {
	var out []prAction
	if m.review != nil {
		out = append(out, prAction{id: "open", label: "Open review (TUI)"})
	}
	if !m.jobRunning() {
		out = append(out,
			prAction{id: "bg", label: "Run review (background job)"},
			prAction{id: "fg", label: "Run review (foreground — hands the terminal to claude until it exits)"},
		)
	}
	if m.review != nil && m.review.GeneratedBy.SessionID != "" {
		tool := "claude"
		if m.review.GeneratedBy.Tool == "codex" {
			tool = "codex"
		}
		out = append(out, prAction{
			id:    "resume",
			label: fmt.Sprintf("Resume %s session — hands the terminal to %s until it exits", tool, tool),
		})
	}
	return out
}

// clampCursor keeps the cursor inside the (state-dependent) action list.
func (m *PRActions) clampCursor() {
	if n := len(m.actions()); m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

func (m *PRActions) Title() string { return fmt.Sprintf("PR #%d", m.item.Number) }

func (m *PRActions) Init() tea.Cmd { return m.reload() }

func (m *PRActions) reload() tea.Cmd {
	load := m.load
	if m.item.ReviewedPath != "" {
		m.loading = true
	}
	return tea.Batch(m.reloadJob(), func() tea.Msg {
		r, err := load()
		return prActionsLoadedMsg{review: r, err: err}
	})
}

func (m *PRActions) reloadJob() tea.Cmd {
	load := m.loadJob
	return func() tea.Msg {
		j, tail := load()
		return prActionsJobMsg{job: j, tail: tail}
	}
}

// jobTick schedules the next live refresh of a running job.
func jobTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return jobTickMsg{} })
}

func (m *PRActions) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case prActionsLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.review = msg.review
		m.clampCursor()

	case prActionsErrMsg:
		m.actionErr = msg.err

	case jobStartedMsg:
		if msg.err != nil {
			m.actionErr = msg.err
			break
		}
		m.actionErr = nil
		m.notice = fmt.Sprintf("Started background job %s", msg.job.ID)
		m.clampCursor()
		return m, m.reloadJob()

	case execDoneMsg:
		// The terminal is ours again; whatever the agent did (a fresh
		// review, an edited one) is on disk, so re-read everything.
		m.actionErr = msg.err
		m.notice = ""
		return m, m.reload()

	case prActionsJobMsg:
		m.job, m.logTail = msg.job, msg.tail
		m.clampCursor()
		if m.job == nil {
			break
		}
		switch st, _ := m.job.Effective(time.Now()); {
		case st == jobs.StateRunning:
			// Keep the state line and log tail live while the worker runs.
			return m, jobTick()
		case st == jobs.StateDone && m.review == nil && m.job.OutDir != "":
			// The job finished while we watched: pull in the review it
			// produced so [Open review] appears without re-entering L1.
			m.loading = true
			outDir := m.job.OutDir
			return m, func() tea.Msg {
				r, err := store.Load(outDir)
				return prActionsLoadedMsg{review: r, err: err}
			}
		}

	case jobTickMsg:
		return m, m.reloadJob()

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

// runAction executes the selected action.
func (m *PRActions) runAction() tea.Cmd {
	acts := m.actions()
	if m.cursor >= len(acts) {
		return nil
	}
	switch acts[m.cursor].id {
	case "open":
		review := m.review
		open := m.openReview
		return func() tea.Msg {
			screen, err := open(review)
			if err != nil {
				return prActionsErrMsg{err: err}
			}
			return PushMsg{Screen: screen}
		}

	case "bg":
		slug := m.slug
		start := m.startJob
		return func() tea.Msg {
			dir, err := cloneDirFor(slug)
			if err != nil {
				return jobStartedMsg{err: err}
			}
			j, err := start(dir)
			return jobStartedMsg{job: j, err: err}
		}

	case "fg":
		return m.execRevu("review", strconv.Itoa(m.item.Number))

	case "resume":
		if m.review == nil {
			return nil
		}
		return m.execRevu("resume", m.review.BaseDir)
	}
	return nil
}

// execRevu hands the terminal to `revu <args...>` running inside the PR's
// clone. Errors resolving the binary or the clone surface in the action
// panel instead of tearing the screen down.
func (m *PRActions) execRevu(args ...string) tea.Cmd {
	bin, err := os.Executable()
	if err != nil {
		return func() tea.Msg { return prActionsErrMsg{err: fmt.Errorf("locate revu binary: %w", err)} }
	}
	dir, err := cloneDirFor(m.slug)
	if err != nil {
		return func() tea.Msg { return prActionsErrMsg{err: err} }
	}
	return m.runExec(append([]string{bin}, args...), dir)
}

func (m *PRActions) View() string {
	header := fmt.Sprintf("%s — PR #%d", m.slug, m.item.Number)
	if m.item.Title != "" {
		header += "  " + truncate(m.item.Title, m.width-30)
	}
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(header)

	var body string
	switch {
	case m.loading:
		body = faintBox("Loading review ...")
	case m.err != nil:
		body = faintBox(fmt.Sprintf("Failed to load review: %v\n\nPress [r] to retry.", m.err))
	default:
		body = lipgloss.JoinVertical(lipgloss.Left, m.metaView(), m.jobView(), m.actionView())
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter] run  [j/k] move  [r] reload  [esc] back")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m *PRActions) metaView() string {
	var lines []string
	if m.item.Author != "" {
		lines = append(lines, fmt.Sprintf("author:       @%s", m.item.Author))
	}

	if r := m.review; r != nil {
		counts := r.Counts()
		lines = append(lines,
			fmt.Sprintf("review_event: %s", r.ReviewEvent),
			fmt.Sprintf("comments:     %d (pending=%d accepted=%d rejected=%d edited=%d)",
				len(r.Comments),
				counts[model.StatusPending], counts[model.StatusAccepted],
				counts[model.StatusRejected], counts[model.StatusEdited]),
		)
		if r.SubmittedAt != nil {
			lines = append(lines, fmt.Sprintf("submitted:    %s", r.SubmittedAt.Format("2006-01-02 15:04")))
		} else {
			lines = append(lines, "submitted:    (not yet)")
		}
		dir := m.item.ReviewedPath
		if dir == "" {
			dir = r.BaseDir // review discovered after the row was built
		}
		lines = append(lines, fmt.Sprintf("dir:          %s", dir))
	} else if m.jobRunning() {
		lines = append(lines, "local review: (generating — background job running)")
	} else {
		lines = append(lines, "local review: (none yet — run a review below)")
	}
	return lipgloss.NewStyle().Faint(true).Padding(1, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// jobRunning reports whether the PR's newest job is effectively running.
func (m *PRActions) jobRunning() bool {
	if m.job == nil {
		return false
	}
	st, _ := m.job.Effective(time.Now())
	return st == jobs.StateRunning
}

// jobView renders the background-job section: state line, failure cause,
// and the log tail.
func (m *PRActions) jobView() string {
	if m.job == nil {
		return ""
	}
	st, reason := m.job.Effective(time.Now())
	lines := []string{fmt.Sprintf("job:          %s  (%s)", st, m.job.ID)}
	if st == jobs.StateFailed {
		cause := m.job.Err
		if cause == "" {
			cause = reason
		}
		if cause != "" {
			lines = append(lines, "job error:    "+truncate(firstLineOf(cause), m.width-20))
		}
	}
	if len(m.logTail) > 0 {
		lines = append(lines, "", "log tail:")
		for _, l := range m.logTail {
			lines = append(lines, "  "+truncate(l, m.width-6))
		}
	}
	return lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// firstLineOf keeps multi-line errors to their first line for the state row.
func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}

func (m *PRActions) actionView() string {
	acts := m.actions()
	rows := make([]string, 0, len(acts)+2)
	for i, a := range acts {
		rows = append(rows, cursorRow(a.label, i == m.cursor))
	}
	if m.notice != "" {
		rows = append(rows, "", lipgloss.NewStyle().Faint(true).Render(m.notice))
	}
	if m.actionErr != nil {
		rows = append(rows, "", lipgloss.NewStyle().Faint(true).
			Render(fmt.Sprintf("action failed: %v", m.actionErr)))
	}
	if len(rows) == 0 {
		return ""
	}
	return lipgloss.NewStyle().Padding(0, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
