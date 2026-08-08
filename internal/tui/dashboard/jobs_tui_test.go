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
