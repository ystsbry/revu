package dashboard

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/jobs"
)

// A nil watcher is the "this platform has no watch facility" case. wait
// must hand back nil rather than panicking, so the screen simply never
// receives change messages.
func TestJobsWatcherWaitOnNilWatcher(t *testing.T) {
	t.Parallel()
	var w *jobsWatcher
	if cmd := w.wait(); cmd != nil {
		t.Fatalf("a nil watcher should produce no command, got %T", cmd())
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing a nil watcher should be a no-op, got %v", err)
	}
}

func TestJobsWatcherWaitDeliversChanges(t *testing.T) {
	t.Parallel()
	w := &jobsWatcher{changes: make(chan struct{}, 1)}

	w.changes <- struct{}{}
	if msg := w.wait()(); msg == nil {
		t.Fatal("a pending change should produce a message")
	} else if _, ok := msg.(jobsChangedMsg); !ok {
		t.Fatalf("message = %T, want jobsChangedMsg", msg)
	}

	// A closed channel means the watcher is gone: the command retires
	// instead of spinning on a dead channel.
	close(w.changes)
	if msg := w.wait()(); msg != nil {
		t.Fatalf("a closed watcher should produce nil, got %T", msg)
	}
}

func TestNewJobsWatcherCreatesTheJobBookDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)

	w, err := newJobsWatcher()
	if err != nil {
		t.Skipf("no watch facility on this platform: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	dir, err := jobs.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.List(); err != nil {
		t.Fatalf("the job book should be listable once watched: %v", err)
	}
	if !strings.HasPrefix(dir, home) {
		t.Fatalf("job dir %q should live under REVU_HOME %q", dir, home)
	}

	// Close is idempotent — the root screen may tear down twice.
	if err := w.Close(); err != nil {
		t.Fatalf("first Close = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}

func TestJobStateLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state jobs.State
		want  string
	}{
		{state: jobs.StateDone, want: "done"},
		{state: jobs.StateFailed, want: "failed"},
		{state: jobs.StateRunning, want: "running"},
		// Anything unexpected reads as running rather than rendering blank.
		{state: jobs.State("weird"), want: "running"},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			t.Parallel()
			if got := jobStateLabel(tt.state); !strings.Contains(got, tt.want) {
				t.Fatalf("jobStateLabel(%q) = %q, want it to mention %q", tt.state, got, tt.want)
			}
		})
	}
}

// The job tab has three empty-ish states before it can show cards; each
// has to say something actionable.
func TestJobListViewStates(t *testing.T) {
	t.Parallel()

	t.Run("loading", func(t *testing.T) {
		t.Parallel()
		h := homeWithJobs(t, 0, 40)
		h.jobsLoading = true
		if out := h.jobListView(); !strings.Contains(out, "loading jobs") {
			t.Fatalf("view should say it is loading:\n%s", out)
		}
	})

	t.Run("error offers a retry", func(t *testing.T) {
		t.Parallel()
		h := homeWithJobs(t, 0, 40)
		h.jobsErr = errors.New("job book unreadable")
		out := h.jobListView()
		if !strings.Contains(out, "job book unreadable") {
			t.Fatalf("view should surface the cause:\n%s", out)
		}
		if !strings.Contains(out, "[r] retry") {
			t.Fatalf("view should offer a retry:\n%s", out)
		}
	})

	t.Run("empty explains how to start one", func(t *testing.T) {
		t.Parallel()
		h := homeWithJobs(t, 0, 40)
		out := h.jobListView()
		if !strings.Contains(out, "no background jobs yet") {
			t.Fatalf("view should say the list is empty:\n%s", out)
		}
		if !strings.Contains(out, "--bg") {
			t.Fatalf("view should point at how to start a job:\n%s", out)
		}
	})
}

// The card list pages, so it must show where in the list the window is.
func TestJobCardsViewShowsPosition(t *testing.T) {
	t.Parallel()
	h := homeWithJobs(t, 10, 3+cardHeight*3)

	out := h.jobListView()
	if !strings.Contains(out, "1-3 / 10") {
		t.Fatalf("view should show the window position:\n%s", out)
	}

	h.jobCursor = 9
	h.ensureJobVisible()
	if out := h.jobListView(); !strings.Contains(out, "8-10 / 10") {
		t.Fatalf("view should follow the cursor to the end:\n%s", out)
	}
}

func TestHitWithoutAZoneManager(t *testing.T) {
	t.Parallel()
	msg := tea.MouseMsg{X: 1, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	if hit(nil, "anything", msg) {
		t.Fatal("a nil zone manager cannot be hit")
	}
}
