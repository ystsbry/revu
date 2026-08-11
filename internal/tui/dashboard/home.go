package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/jobs"
)

// Pane focus states for the home screen.
const (
	focusSidebar = iota
	focusPRList
)

// Content tabs.
const (
	tabPR = iota
	tabJob
)

// sidebarWidth is the fixed column budget for the repository pane.
const sidebarWidth = 32

// cardHeight is the rendered height of one PR card (border + 2 lines).
const cardHeight = 4

// Home zone ids.
const (
	zoneHomeTabPR     = "home:tab:pr"
	zoneHomeTabJob    = "home:tab:job"
	zoneHomeSidebar   = "home:sidebar"
	zoneHomeRepoPref  = "home:repo:" // + sidebar index
	zoneHomeCardPref  = "home:card:" // + pr index
	zoneHomeJobPref   = "home:job:"  // + job index
	zoneHomePRListBox = "home:prlist"
)

// homeReposMsg carries the sidebar loader result.
type homeReposMsg struct {
	data repoListData
	err  error
}

// homePRsMsg carries one repo's PR list. Slug guards against stale
// responses landing after the user already switched repos.
type homePRsMsg struct {
	slug  string
	items []PRItem
	err   error
}

// homeJobsMsg carries the cross-repo background-job list (newest first).
type homeJobsMsg struct {
	jobs []jobs.Job
	err  error
}

// Home is the dashboard's root screen: a repository sidebar on the left,
// tab bar on top, and the selected repository's open PRs as cards on the
// right. Selecting a card pushes the PR-action screen (and the review TUI
// below it) exactly as the previous stacked layout did.
type Home struct {
	width  int
	height int

	zones *zone.Manager
	focus int
	tab   int

	// Sidebar.
	repoLoading bool
	repoErr     error
	repoData    repoListData
	repoCursor  int // keyboard hover
	selected    int // the repo whose PRs are shown (thick-bar marker)

	// PR list.
	prLoading bool
	prErr     error
	prSlug    string // slug the current items belong to
	items     []PRItem
	prCursor  int
	prOffset  int

	// Job list (cross-repo, newest first).
	jobsLoading bool
	jobsErr     error
	jobItems    []jobs.Job
	jobCursor   int
	jobOffset   int

	watcher  *jobsWatcher
	newWatch func() *jobsWatcher

	loadRepos func() (repoListData, error)
	loadPRs   func(slug, search string) ([]PRItem, error)
	loadJobs  func() ([]jobs.Job, error)
	openPR    func(slug string, it PRItem) Screen
}

func NewHome() *Home {
	h := &Home{
		zones:       zone.New(),
		repoLoading: true,
		selected:    -1,
		loadRepos:   loadReposFromConfig,
		loadPRs:     loadPRsFromGitHub,
		loadJobs:    jobs.List,
		openPR:      func(slug string, it PRItem) Screen { return NewPRActions(slug, it) },
	}
	h.newWatch = func() *jobsWatcher {
		w, err := newJobsWatcher()
		if err != nil {
			return nil
		}
		return w
	}
	return h
}

func (h *Home) Title() string { return "Home" }

// Close releases the job-book watcher. The root screen is never popped,
// but the io.Closer contract keeps the teardown path uniform.
func (h *Home) Close() error {
	h.zones.Close()
	return h.watcher.Close()
}

func (h *Home) Init() tea.Cmd {
	if h.watcher == nil && h.newWatch != nil {
		h.watcher = h.newWatch()
	}
	return tea.Batch(h.reloadRepos(), h.reloadJobs(), h.watcher.wait())
}

// reloadJobs refreshes the cross-repo job list (jobs.List is already
// sorted newest first).
func (h *Home) reloadJobs() tea.Cmd {
	h.jobsLoading = true
	load := h.loadJobs
	return func() tea.Msg {
		all, err := load()
		return homeJobsMsg{jobs: all, err: err}
	}
}

func (h *Home) reloadRepos() tea.Cmd {
	h.repoLoading = true
	load := h.loadRepos
	return func() tea.Msg {
		data, err := load()
		return homeReposMsg{data: data, err: err}
	}
}

// reloadPRs fetches the selected repo's PRs. No-op when nothing is
// selected yet.
func (h *Home) reloadPRs() tea.Cmd {
	if h.selected < 0 || h.selected >= len(h.repoData.Items) {
		return nil
	}
	repo := h.repoData.Items[h.selected]
	h.prLoading = true
	h.prSlug = repo.Slug
	load := h.loadPRs
	return func() tea.Msg {
		items, err := load(repo.Slug, repo.Search)
		return homePRsMsg{slug: repo.Slug, items: items, err: err}
	}
}

// selectRepo makes idx the shown repository and loads its PRs.
func (h *Home) selectRepo(idx int) tea.Cmd {
	if idx < 0 || idx >= len(h.repoData.Items) || idx == h.selected {
		h.repoCursor = max(0, min(idx, len(h.repoData.Items)-1))
		return nil
	}
	h.selected = idx
	h.repoCursor = idx
	h.prCursor, h.prOffset = 0, 0
	h.items = nil
	return h.reloadPRs()
}

func (h *Home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
		h.ensureCardVisible()

	case homeReposMsg:
		h.repoLoading = false
		h.repoErr = msg.err
		h.repoData = msg.data
		if h.repoCursor >= len(h.repoData.Items) {
			h.repoCursor = max(0, len(h.repoData.Items)-1)
		}
		// First load: show the first repo's PRs right away.
		if h.selected < 0 && len(h.repoData.Items) > 0 {
			return h, h.selectRepo(0)
		}

	case homePRsMsg:
		if msg.slug != h.prSlug {
			break // stale response for a repo we already left
		}
		h.prLoading = false
		h.prErr = msg.err
		h.items = msg.items
		if h.prCursor >= len(h.items) {
			h.prCursor = max(0, len(h.items)-1)
		}
		h.ensureCardVisible()

	case homeJobsMsg:
		h.jobsLoading = false
		h.jobsErr = msg.err
		h.jobItems = msg.jobs
		if h.jobCursor >= len(h.jobItems) {
			h.jobCursor = max(0, len(h.jobItems)-1)
		}
		h.ensureJobVisible()

	case jobsChangedMsg:
		// A job started or finished: refresh badges + the job tab and
		// re-arm the watcher.
		return h, tea.Batch(h.reloadPRs(), h.reloadJobs(), h.watcher.wait())

	case tea.MouseMsg:
		return h.updateMouse(msg)

	case tea.KeyMsg:
		return h.updateKeys(msg)
	}
	return h, nil
}

func (h *Home) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return h, tea.Quit
	case "r":
		return h, tea.Batch(h.reloadRepos(), h.reloadPRs(), h.reloadJobs())
	case "1":
		h.tab = tabPR
		return h, nil
	case "2":
		h.tab = tabJob
		return h, nil
	case "tab", "h", "l", "left", "right":
		if h.focus == focusSidebar {
			h.focus = focusPRList
		} else {
			h.focus = focusSidebar
		}
		return h, nil
	case "j", "down":
		switch {
		case h.focus == focusSidebar:
			h.repoCursor = min(h.repoCursor+1, len(h.repoData.Items)-1)
		case h.tab == tabJob:
			h.jobCursor = min(h.jobCursor+1, len(h.jobItems)-1)
			h.ensureJobVisible()
		default:
			h.prCursor = min(h.prCursor+1, len(h.items)-1)
			h.ensureCardVisible()
		}
		return h, nil
	case "k", "up":
		switch {
		case h.focus == focusSidebar:
			h.repoCursor = max(h.repoCursor-1, 0)
		case h.tab == tabJob:
			h.jobCursor = max(h.jobCursor-1, 0)
			h.ensureJobVisible()
		default:
			h.prCursor = max(h.prCursor-1, 0)
			h.ensureCardVisible()
		}
		return h, nil
	case "enter":
		if h.focus == focusSidebar {
			// Picking a repo is a PR-tab affair: switch there so the
			// selection's effect is visible.
			h.tab = tabPR
			return h, h.selectRepo(h.repoCursor)
		}
		if h.tab == tabJob {
			return h, nil // job cards have no drill-down yet
		}
		return h, h.openCursorPR()
	}
	return h, nil
}

func (h *Home) openCursorPR() tea.Cmd {
	if h.prCursor < 0 || h.prCursor >= len(h.items) {
		return nil
	}
	return Push(h.openPR(h.prSlug, h.items[h.prCursor]))
}

// updateMouse: the wheel scrolls whichever pane it hovers; clicks select
// (and re-select opens), matching the review TUI's contract.
func (h *Home) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp, msg.Button == tea.MouseButtonWheelDown:
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		switch {
		case hit(h.zones, zoneHomeSidebar, msg):
			h.focus = focusSidebar
			h.repoCursor = max(0, min(h.repoCursor+delta, len(h.repoData.Items)-1))
		case h.tab == tabJob:
			h.focus = focusPRList
			h.jobCursor = max(0, min(h.jobCursor+delta, len(h.jobItems)-1))
			h.ensureJobVisible()
		default:
			h.focus = focusPRList
			h.prCursor = max(0, min(h.prCursor+delta, len(h.items)-1))
			h.ensureCardVisible()
		}
		return h, nil
	case !leftClickHome(msg):
		return h, nil
	}

	if hit(h.zones, zoneHomeTabPR, msg) {
		h.tab = tabPR
		return h, nil
	}
	if hit(h.zones, zoneHomeTabJob, msg) {
		h.tab = tabJob
		return h, nil
	}
	for i := range h.repoData.Items {
		if hit(h.zones, fmt.Sprintf("%s%d", zoneHomeRepoPref, i), msg) {
			h.focus = focusSidebar
			h.tab = tabPR
			return h, h.selectRepo(i)
		}
	}
	end := min(h.prOffset+h.cardsPerPage(), len(h.items))
	for i := h.prOffset; i < end; i++ {
		if !hit(h.zones, fmt.Sprintf("%s%d", zoneHomeCardPref, i), msg) {
			continue
		}
		h.focus = focusPRList
		if h.prCursor == i {
			return h, h.openCursorPR()
		}
		h.prCursor = i
		return h, nil
	}
	return h, nil
}

// hit / leftClick mirror the views package helpers; the dashboard keeps
// its own tiny copies to avoid an import that exists only for two
// one-liners.
func hit(z *zone.Manager, id string, msg tea.MouseMsg) bool {
	if z == nil {
		return false
	}
	info := z.Get(id)
	return info != nil && !info.IsZero() && info.InBounds(msg)
}

func leftClickHome(msg tea.MouseMsg) bool {
	return msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress
}

// --- layout ---------------------------------------------------------------

// cardsPerPage is how many PR cards fit in the content column.
func (h *Home) cardsPerPage() int {
	// Tab bar (1) + padding rows around the list area.
	usable := h.height - 3
	if usable < cardHeight {
		return 1
	}
	return usable / cardHeight
}

func (h *Home) ensureJobVisible() {
	per := h.cardsPerPage()
	if h.jobCursor < h.jobOffset {
		h.jobOffset = h.jobCursor
	}
	if h.jobCursor >= h.jobOffset+per {
		h.jobOffset = h.jobCursor - per + 1
	}
	if maxOff := max(0, len(h.jobItems)-per); h.jobOffset > maxOff {
		h.jobOffset = maxOff
	}
	if h.jobOffset < 0 {
		h.jobOffset = 0
	}
}

func (h *Home) ensureCardVisible() {
	per := h.cardsPerPage()
	if h.prCursor < h.prOffset {
		h.prOffset = h.prCursor
	}
	if h.prCursor >= h.prOffset+per {
		h.prOffset = h.prCursor - per + 1
	}
	if maxOff := max(0, len(h.items)-per); h.prOffset > maxOff {
		h.prOffset = maxOff
	}
	if h.prOffset < 0 {
		h.prOffset = 0
	}
}

func (h *Home) View() string {
	sidebar := h.sidebarView()
	main := h.prListView()
	if h.tab == tabJob {
		main = h.jobListView()
	}
	content := lipgloss.JoinVertical(lipgloss.Left, h.tabBarView(), main)
	// Scan at the screen's outermost point: the shell offsets mouse
	// coordinates for the breadcrumb row, so positions stay Home-relative.
	return h.zones.Scan(lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content))
}

// tabBarView renders the PR and job tabs (clickable) and the future tabs
// (report / config) as visibly disabled placeholders.
func (h *Home) tabBarView() string {
	activeSt := lipgloss.NewStyle().Bold(true).Padding(0, 2).
		Background(lipgloss.Color("57")).Foreground(lipgloss.Color("229"))
	inactiveSt := lipgloss.NewStyle().Padding(0, 2)
	disabled := lipgloss.NewStyle().Padding(0, 2).Faint(true)

	tab := func(id, label string, on bool) string {
		st := inactiveSt
		if on {
			st = activeSt
		}
		return h.zones.Mark(id, st.Render(label))
	}
	tabs := []string{
		tab(zoneHomeTabPR, "PR", h.tab == tabPR),
		tab(zoneHomeTabJob, "job", h.tab == tabJob),
		disabled.Render("report"),
		disabled.Render("config"),
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(tabs, " "))
}

// sidebarView renders the repository pane. The selected repository (the
// one whose PRs are shown) carries a thick bar on the left; the keyboard
// cursor is a subtle highlight.
func (h *Home) sidebarView() string {
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("repository")
	if h.repoData.Profile != "" {
		title += lipgloss.NewStyle().Faint(true).Render(" [" + h.repoData.Profile + "]")
	}

	rows := []string{title}
	switch {
	case h.repoLoading:
		rows = append(rows, faintBox("loading ..."))
	case h.repoErr != nil:
		rows = append(rows, faintBox(fmt.Sprintf("error: %v\n[r] retry", h.repoErr)))
	case len(h.repoData.Items) == 0:
		rows = append(rows, faintBox("no repositories\n`revu repo scan <root>`"))
	default:
		cursorSt := lipgloss.NewStyle().Background(lipgloss.Color("237"))
		selectedSt := lipgloss.NewStyle().Bold(true)
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("▌")
		for i, it := range h.repoData.Items {
			name := truncate(it.Slug, sidebarWidth-4)
			if it.PathMissing {
				name += " !"
			}
			row := "  " + name
			if i == h.selected {
				row = bar + " " + selectedSt.Render(name)
			}
			if i == h.repoCursor && h.focus == focusSidebar {
				row = cursorSt.Render(row)
			}
			rows = append(rows, h.zones.Mark(fmt.Sprintf("%s%d", zoneHomeRepoPref, i), row))
		}
	}
	for _, w := range h.repoData.Warnings {
		rows = append(rows, "", lipgloss.NewStyle().Faint(true).Render(truncate("! "+w, sidebarWidth-2)))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	pane := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(max(h.height-1, 1)).
		Padding(0, 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderForeground(lipgloss.Color("240")).
		Render(body)
	return h.zones.Mark(zoneHomeSidebar, pane)
}

// prListView renders the selected repository's open PRs as cards.
func (h *Home) prListView() string {
	width := h.width - sidebarWidth - 4
	if width < 30 {
		width = 30
	}

	var body string
	switch {
	case h.selected < 0 && h.repoLoading:
		body = faintBox("loading repositories ...")
	case h.selected < 0:
		body = faintBox("select a repository")
	case h.prLoading:
		body = faintBox(fmt.Sprintf("loading PRs for %s ...", h.prSlug))
	case h.prErr != nil:
		body = faintBox(fmt.Sprintf("failed to list PRs: %v\n\n[r] retry", h.prErr))
	case len(h.items) == 0:
		body = faintBox("no open PRs match this repository's search condition")
	default:
		body = h.cardsView(width)
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[enter/click] open  [j/k/wheel] move  [tab] switch pane  [r] reload  [q]uit")
	return lipgloss.JoinVertical(lipgloss.Left, h.zones.Mark(zoneHomePRListBox, body), footer)
}

// jobListView renders the cross-repo background jobs as cards, newest
// first (jobs.List order).
func (h *Home) jobListView() string {
	width := h.width - sidebarWidth - 4
	if width < 30 {
		width = 30
	}

	var body string
	switch {
	case h.jobsLoading && len(h.jobItems) == 0:
		body = faintBox("loading jobs ...")
	case h.jobsErr != nil:
		body = faintBox(fmt.Sprintf("failed to list jobs: %v\n\n[r] retry", h.jobsErr))
	case len(h.jobItems) == 0:
		body = faintBox("no background jobs yet\nstart one from a PR card (background job) or `revu review <PR> --bg`")
	default:
		body = h.jobCardsView(width)
	}

	footer := lipgloss.NewStyle().Faint(true).Padding(0, 1).
		Render("[j/k/wheel] move  [1] PR tab  [r] reload  [q]uit")
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (h *Home) jobCardsView(width int) string {
	normal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width)
	selected := normal.BorderForeground(lipgloss.Color("205"))

	now := time.Now()
	end := min(h.jobOffset+h.cardsPerPage(), len(h.jobItems))
	cards := make([]string, 0, end-h.jobOffset)
	for i := h.jobOffset; i < end; i++ {
		j := h.jobItems[i]
		state, _ := j.Effective(now)
		prefix := fmt.Sprintf("%s %s #%d ", jobStateLabel(state), j.Slug, j.PR)
		title := prefix + truncateCells(j.PRTitle, width-4-lipgloss.Width(prefix))
		meta := fmt.Sprintf("workflow: %s  %s", j.ID, j.StartedAt.Format("2006-01-02 15:04"))
		st := normal
		if i == h.jobCursor {
			st = selected
			title = lipgloss.NewStyle().Bold(true).Render(title)
		}
		card := st.Render(title + "\n" + lipgloss.NewStyle().Faint(true).Render(truncateCells(meta, width-4)))
		cards = append(cards, h.zones.Mark(fmt.Sprintf("%s%d", zoneHomeJobPref, i), card))
	}
	if h.jobOffset+h.cardsPerPage() < len(h.jobItems) || h.jobOffset > 0 {
		pos := lipgloss.NewStyle().Faint(true).Padding(0, 1).
			Render(fmt.Sprintf("%d-%d / %d", h.jobOffset+1, end, len(h.jobItems)))
		cards = append(cards, pos)
	}
	return lipgloss.JoinVertical(lipgloss.Left, cards...)
}

// jobStateLabel renders 成功/失敗/実行中 as a compact colored marker.
func jobStateLabel(st jobs.State) string {
	switch st {
	case jobs.StateDone:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("✓ done")
	case jobs.StateFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("✗ failed")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("● running")
	}
}

func (h *Home) cardsView(width int) string {
	normal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width)
	selected := normal.
		BorderForeground(lipgloss.Color("205"))

	end := min(h.prOffset+h.cardsPerPage(), len(h.items))
	cards := make([]string, 0, end-h.prOffset)
	for i := h.prOffset; i < end; i++ {
		it := h.items[i]
		title := fmt.Sprintf("#%d %s", it.Number, truncateCells(it.Title, width-12))
		author := "author: " + it.Author + it.badges()
		st := normal
		if i == h.prCursor {
			st = selected
			title = lipgloss.NewStyle().Bold(true).Render(title)
		}
		card := st.Render(title + "\n" + lipgloss.NewStyle().Faint(true).Render(author))
		cards = append(cards, h.zones.Mark(fmt.Sprintf("%s%d", zoneHomeCardPref, i), card))
	}
	if h.prOffset+h.cardsPerPage() < len(h.items) || h.prOffset > 0 {
		pos := lipgloss.NewStyle().Faint(true).Padding(0, 1).
			Render(fmt.Sprintf("%d-%d / %d", h.prOffset+1, end, len(h.items)))
		cards = append(cards, pos)
	}
	return lipgloss.JoinVertical(lipgloss.Left, cards...)
}
