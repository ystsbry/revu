package dashboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/jobs"
	"github.com/ystsbry/revu/internal/model"
)

// seedTestJob writes one job record for slug#pr into the isolated book.
func seedTestJob(t *testing.T, slug string, pr int, state jobs.State, mutate func(*jobs.Job)) jobs.Job {
	t.Helper()
	j, err := jobs.New(slug, pr, "claude", "bg", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	j.State = state
	if mutate != nil {
		mutate(&j)
	}
	if err := jobs.Save(j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestJobBadgeState(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	now := time.Now()

	if got := jobBadgeState("o/r", 1, now); got != "" {
		t.Errorf("no job should mean no badge, got %q", got)
	}

	seedTestJob(t, "o/r", 1, jobs.StateRunning, func(j *jobs.Job) { j.PID = os.Getpid() })
	if got := jobBadgeState("o/r", 1, now); got != "running" {
		t.Errorf("live job badge = %q, want running", got)
	}

	seedTestJob(t, "o/r", 2, jobs.StateFailed, nil)
	if got := jobBadgeState("o/r", 2, now); got != "failed" {
		t.Errorf("failed job badge = %q, want failed", got)
	}

	// A done job stays silent: its outcome shows as [reviewed] already.
	seedTestJob(t, "o/r", 3, jobs.StateDone, nil)
	if got := jobBadgeState("o/r", 3, now); got != "" {
		t.Errorf("done job badge = %q, want none", got)
	}

	// A crashed worker (running record, dead pid) surfaces as failed —
	// the acceptance criterion for failures reaching L1.
	seedTestJob(t, "o/r", 4, jobs.StateRunning, func(j *jobs.Job) { j.PID = deadTestPID(t) })
	if got := jobBadgeState("o/r", 4, now); got != "failed" {
		t.Errorf("crashed job badge = %q, want failed", got)
	}
}

func deadTestPID(t *testing.T) int {
	t.Helper()
	// A PID from a reaped child cannot be alive.
	cmd := newTrueCmd()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// A job-book change must refresh the PR list (badges) and re-arm the
// watcher for the next transition.
func TestPRListReloadsOnJobsChanged(t *testing.T) {
	t.Parallel()
	loads := 0
	m := NewPRList("o/r", "")
	m.newWatch = func() *jobsWatcher { return nil }
	m.load = func(search string) ([]PRItem, error) {
		loads++
		return nil, nil
	}

	m.Update(m.reload()())
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}

	_, cmd := m.Update(jobsChangedMsg{})
	if cmd == nil {
		t.Fatal("jobsChangedMsg should trigger a reload")
	}
	// The returned batch contains the reload; executing its parts must
	// bump the load counter (the nil watcher contributes nothing).
	if msg := cmd(); msg != nil {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					m.Update(c())
				}
			}
		} else {
			m.Update(msg)
		}
	}
	if loads != 2 {
		t.Errorf("loads = %d, want 2 after job change", loads)
	}
}

func TestPRActionsShowsRunningJobAndTicks(t *testing.T) {
	t.Parallel()
	m := NewPRActions("o/r", PRItem{Number: 9, Title: "wip"})
	running := jobs.Job{
		ID: "job-1", Slug: "o/r", PR: 9, State: jobs.StateRunning,
		PID: os.Getpid(), StartedAt: time.Now(),
	}

	_, cmd := m.Update(prActionsJobMsg{job: &running, tail: []string{"relay line one", "relay line two"}})
	if cmd == nil {
		t.Error("a running job must schedule a live-refresh tick")
	}

	out := m.View()
	for _, want := range []string{"job:", "running", "job-1", "log tail:", "relay line two",
		"(generating — background job running)"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}

	// The tick re-reads the job through the injected loader.
	reloaded := false
	m.loadJob = func() (*jobs.Job, []string) {
		reloaded = true
		return &running, nil
	}
	_, cmd = m.Update(jobTickMsg{})
	if cmd == nil {
		t.Fatal("tick should produce a job reload")
	}
	cmd()
	if !reloaded {
		t.Error("tick did not consult the job loader")
	}
}

func TestPRActionsShowsFailedJobCause(t *testing.T) {
	t.Parallel()
	m := NewPRActions("o/r", PRItem{Number: 9})
	failed := jobs.Job{
		ID: "job-2", Slug: "o/r", PR: 9, State: jobs.StateFailed,
		Err: "clone /x is gone — re-register it", StartedAt: time.Now(),
	}

	_, cmd := m.Update(prActionsJobMsg{job: &failed, tail: []string{"boom"}})
	if cmd != nil {
		t.Error("a terminal job must not keep ticking")
	}
	out := m.View()
	for _, want := range []string{"failed", "job error:", "clone /x is gone"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

// When the watched job completes, the review it produced is pulled in so
// [Open review] appears without bouncing through L1.
func TestPRActionsLoadsReviewWhenJobFinishes(t *testing.T) {
	t.Parallel()
	m := NewPRActions("o/r", PRItem{Number: 5}) // no ReviewedPath
	done := jobs.Job{
		ID: "job-3", Slug: "o/r", PR: 5, State: jobs.StateDone,
		OutDir: "/reviews/pr-5", StartedAt: time.Now(),
	}

	_, cmd := m.Update(prActionsJobMsg{job: &done})
	if cmd == nil {
		t.Fatal("a finished job with an out dir should load its review")
	}
	msg := cmd()
	loaded, ok := msg.(prActionsLoadedMsg)
	if !ok {
		t.Fatalf("expected prActionsLoadedMsg, got %T", msg)
	}
	// The load fails (the dir does not exist) but the message shape is
	// what matters here: the screen wired the job's OutDir to the review
	// loader.
	if loaded.err == nil {
		t.Error("loading a nonexistent out dir should error")
	}
}

// The PR list owns an OS resource (the job-book watcher); popping the
// screen must release it via the io.Closer hook in Root.
func TestRootClosesPoppedScreens(t *testing.T) {
	t.Parallel()
	closed := false
	root := newTestScreen("L0")
	r := NewRoot(root)
	drive(r, PushMsg{Screen: &closableScreen{testScreen: newTestScreen("L1"), onClose: func() { closed = true }}})

	drive(r, PopMsg{})
	if !closed {
		t.Error("popped screen's Close was not called")
	}
}

type closableScreen struct {
	*testScreen
	onClose func()
}

func (c *closableScreen) Close() error {
	c.onClose()
	return nil
}

func TestJobsWatcherFiresOnJobRecordOnly(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	w, err := newJobsWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer func() { _ = w.Close() }()

	dir, err := jobs.Dir()
	if err != nil {
		t.Fatal(err)
	}

	// A log append must NOT wake the screens (it would hammer gh).
	if err := os.WriteFile(filepath.Join(dir, "job-1.log"), []byte("relay\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.changes:
		t.Fatal("log write should not fire the watcher")
	case <-time.After(2 * jobsDebounce):
	}

	// A job record write must fire (debounced).
	if err := os.WriteFile(filepath.Join(dir, "job-1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.changes:
	case <-time.After(3 * time.Second):
		t.Fatal("job record write never fired the watcher")
	}
}

// newTrueCmd returns an exec.Cmd for a trivially exiting process.
func newTrueCmd() *exec.Cmd { return exec.Command("true") }

// --- WS-E: L2 run actions --------------------------------------------------

func reviewWithSession(tool string) *model.Review {
	r := testReview(5)
	r.GeneratedBy.Tool = tool
	r.GeneratedBy.SessionID = "sess-9"
	return r
}

// The action panel is state-dependent: a loaded review adds [open], a
// recorded session adds [resume] named after its engine, and a running
// job hides the run actions.
func TestPRActionsPanelContents(t *testing.T) {
	t.Parallel()
	m := NewPRActions("o/r", PRItem{Number: 5, ReviewedPath: "/p"})

	m.Update(prActionsLoadedMsg{review: reviewWithSession("codex")})
	ids := func() []string {
		var out []string
		for _, a := range m.actions() {
			out = append(out, a.id)
		}
		return out
	}
	if got := strings.Join(ids(), ","); got != "open,bg,fg,resume" {
		t.Errorf("actions = %s, want open,bg,fg,resume", got)
	}
	if !strings.Contains(m.View(), "Resume codex session") {
		t.Errorf("resume label should name the engine:\n%s", m.View())
	}

	// A running job hides both run actions.
	running := jobs.Job{State: jobs.StateRunning, PID: os.Getpid(), StartedAt: time.Now()}
	m.Update(prActionsJobMsg{job: &running})
	if got := strings.Join(ids(), ","); got != "open,resume" {
		t.Errorf("actions while running = %s, want open,resume", got)
	}
}

// [background] hands the resolved clone to the shared job starter and
// surfaces the started job as a notice.
func TestPRActionsBGStartsJob(t *testing.T) {
	clone := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[[repo]]\nslug = \"o/r\"\npath = \""+clone+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfgPath)

	var gotDir string
	m := NewPRActions("o/r", PRItem{Number: 9})
	m.loadJob = func() (*jobs.Job, []string) { return nil, nil }
	m.startJob = func(workDir string) (jobs.Job, error) {
		gotDir = workDir
		return jobs.Job{ID: "job-x", State: jobs.StateRunning}, nil
	}
	m.Update(m.reloadJob()()) // no review, no job: cursor 0 = bg

	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("bg action should produce a command")
	}
	msg := cmd()
	started, ok := msg.(jobStartedMsg)
	if !ok || started.err != nil {
		t.Fatalf("expected successful jobStartedMsg, got %#v", msg)
	}
	if gotDir != clone {
		t.Errorf("job workdir = %q, want registered clone %q", gotDir, clone)
	}

	m.Update(msg)
	if !strings.Contains(m.View(), "Started background job job-x") {
		t.Errorf("notice missing:\n%s", m.View())
	}
}

// [background] with no resolvable clone shows the registration hint
// instead of starting anything.
func TestPRActionsBGWithoutCloneErrors(t *testing.T) {
	t.Setenv("REVU_CONFIG", filepath.Join(t.TempDir(), "none.toml"))

	m := NewPRActions("o/r", PRItem{Number: 9})
	m.loadJob = func() (*jobs.Job, []string) { return nil, nil }
	m.startJob = func(string) (jobs.Job, error) {
		t.Error("startJob must not run without a clone")
		return jobs.Job{}, nil
	}
	m.Update(m.reloadJob()())

	_, cmd := m.Update(keyMsg("enter"))
	msg := cmd()
	started, ok := msg.(jobStartedMsg)
	if !ok || started.err == nil {
		t.Fatalf("expected failing jobStartedMsg, got %#v", msg)
	}
	m.Update(msg)
	if !strings.Contains(m.View(), "revu repo add") {
		t.Errorf("error should carry the registration hint:\n%s", m.View())
	}
}

// [foreground] and [resume] hand the terminal to the revu binary running
// inside the registered clone.
func TestPRActionsExecActions(t *testing.T) {
	clone := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[[repo]]\nslug = \"o/r\"\npath = \""+clone+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", cfgPath)

	var gotArgv []string
	var gotDir string
	m := NewPRActions("o/r", PRItem{Number: 9, ReviewedPath: "/p"})
	m.loadJob = func() (*jobs.Job, []string) { return nil, nil }
	m.runExec = func(argv []string, dir string) tea.Cmd {
		gotArgv, gotDir = argv, dir
		return nil
	}
	review := reviewWithSession("claude")
	review.BaseDir = "/reviews/pr-9"
	m.Update(prActionsLoadedMsg{review: review})
	m.Update(m.reloadJob()())

	// actions: open(0) bg(1) fg(2) resume(3)
	m.cursor = 2
	m.Update(keyMsg("enter"))
	if len(gotArgv) < 3 || gotArgv[1] != "review" || gotArgv[2] != "9" {
		t.Errorf("fg argv = %v, want [.. review 9]", gotArgv)
	}
	if gotDir != clone {
		t.Errorf("fg dir = %q, want %q", gotDir, clone)
	}

	m.cursor = 3
	m.Update(keyMsg("enter"))
	if len(gotArgv) < 3 || gotArgv[1] != "resume" || gotArgv[2] != "/reviews/pr-9" {
		t.Errorf("resume argv = %v, want [.. resume /reviews/pr-9]", gotArgv)
	}
}

// Returning from a terminal-handover action re-reads everything: the
// agent may have created or edited the review while we were gone.
func TestPRActionsExecDoneReloads(t *testing.T) {
	t.Parallel()
	loads := 0
	m := NewPRActions("o/r", PRItem{Number: 9})
	m.load = func() (*model.Review, error) {
		loads++
		return nil, nil
	}
	m.loadJob = func() (*jobs.Job, []string) { return nil, nil }

	_, cmd := m.Update(execDoneMsg{})
	if cmd == nil {
		t.Fatal("execDoneMsg should reload")
	}
	if msg := cmd(); msg != nil {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					c()
				}
			}
		}
	}
	if loads != 1 {
		t.Errorf("review loads = %d, want 1", loads)
	}
}
