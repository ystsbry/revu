package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
)

// reviewOnDisk writes a review whose summary and comment bodies exist as
// real files, so :reload has something to read back.
func reviewOnDisk(t *testing.T) *model.Review {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("summary.md", "summary v1\n")
	write("a.md", "c1 body v1\n")
	write("b.md", "c2 body v1\n")

	r := sampleReview()
	r.BaseDir = dir
	r.SummaryFile = "summary.md"
	r.SummaryBody = "summary v1\n"
	r.Comments[0].Body = "c1 body v1\n"
	r.Comments[1].Body = "c2 body v1\n"
	return r
}

// runCmd types a ":" command into the app and returns the status line.
func runCmd(t *testing.T, a *App, input string) tea.Cmd {
	t.Helper()
	_, cmd := a.runCommand(input)
	return cmd
}

func TestReloadRereadsBodiesFromDisk(t *testing.T) {
	t.Parallel()
	r := reviewOnDisk(t)
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	// Someone edits the files behind the TUI's back.
	for name, body := range map[string]string{
		"summary.md": "summary v2\n",
		"a.md":       "c1 body v2\n",
	} {
		if err := os.WriteFile(filepath.Join(r.BaseDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, a, "reload")

	if r.SummaryBody != "summary v2\n" {
		t.Errorf("summary = %q, want the version on disk", r.SummaryBody)
	}
	if r.Comments[0].Body != "c1 body v2\n" {
		t.Errorf("c1 body = %q, want the version on disk", r.Comments[0].Body)
	}
	if a.statusErr {
		t.Errorf("reload should not report an error: %q", a.statusMsg)
	}
	if !strings.Contains(a.statusMsg, "reloaded") {
		t.Errorf("status = %q, want it to confirm the reload", a.statusMsg)
	}
}

// Reload must not touch status fields — a pending accept/reject the user
// has not saved yet would otherwise be silently reverted.
func TestReloadLeavesPendingStatusAlone(t *testing.T) {
	t.Parallel()
	r := reviewOnDisk(t)
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})
	r.Comments[0].Status = model.StatusAccepted

	runCmd(t, a, "reload")

	if r.Comments[0].Status != model.StatusAccepted {
		t.Fatalf("status = %s, want the unsaved accept kept", r.Comments[0].Status)
	}
}

func TestReloadErrors(t *testing.T) {
	t.Parallel()

	t.Run("no BaseDir", func(t *testing.T) {
		t.Parallel()
		r := sampleReview()
		r.BaseDir = ""
		a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "reload")
		if !a.statusErr || !strings.Contains(a.statusMsg, "BaseDir") {
			t.Fatalf("status = %q (err=%v), want a BaseDir complaint", a.statusMsg, a.statusErr)
		}
	})

	t.Run("summary file is missing", func(t *testing.T) {
		t.Parallel()
		r := reviewOnDisk(t)
		if err := os.Remove(filepath.Join(r.BaseDir, "summary.md")); err != nil {
			t.Fatal(err)
		}
		a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "reload")
		if !a.statusErr || !strings.Contains(a.statusMsg, "reload summary") {
			t.Fatalf("status = %q (err=%v), want a summary read error", a.statusMsg, a.statusErr)
		}
	})

	t.Run("a comment body is missing", func(t *testing.T) {
		t.Parallel()
		r := reviewOnDisk(t)
		if err := os.Remove(filepath.Join(r.BaseDir, "b.md")); err != nil {
			t.Fatal(err)
		}
		a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "reload")
		if !a.statusErr || !strings.Contains(a.statusMsg, "c2") {
			t.Fatalf("status = %q (err=%v), want the failing comment named", a.statusMsg, a.statusErr)
		}
	})
}

// A comment with no body file is skipped rather than treated as an error.
func TestReloadSkipsCommentsWithoutABodyFile(t *testing.T) {
	t.Parallel()
	r := reviewOnDisk(t)
	r.Comments[1].BodyFile = ""
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	runCmd(t, a, "reload")
	if a.statusErr {
		t.Fatalf("status = %q, want no error", a.statusMsg)
	}
}

func TestFilterCommand(t *testing.T) {
	t.Parallel()

	t.Run("applies a filter and counts the matches", func(t *testing.T) {
		t.Parallel()
		a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "filter severity:major")
		if a.statusErr {
			t.Fatalf("status = %q, want a successful filter", a.statusMsg)
		}
		if !strings.Contains(a.statusMsg, "1 visible") {
			t.Fatalf("status = %q, want the visible count", a.statusMsg)
		}
	})

	t.Run("reports when nothing matches", func(t *testing.T) {
		t.Parallel()
		a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "filter severity:critical")
		if a.statusErr {
			t.Fatalf("status = %q, want a successful filter", a.statusMsg)
		}
		if !strings.Contains(a.statusMsg, "no matches") {
			t.Fatalf("status = %q, want it to say nothing matched", a.statusMsg)
		}
	})

	t.Run("clears with an explicit clear", func(t *testing.T) {
		t.Parallel()
		a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "filter severity:major")
		runCmd(t, a, "filter clear")
		if !strings.Contains(a.statusMsg, "filter cleared") {
			t.Fatalf("status = %q, want the cleared notice", a.statusMsg)
		}
		if a.list.VisibleCount() != len(sampleReview().Comments) {
			t.Fatalf("visible = %d, want every comment back", a.list.VisibleCount())
		}
	})

	t.Run("a bare :filter also clears", func(t *testing.T) {
		t.Parallel()
		a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "filter severity:major")
		runCmd(t, a, "filter")
		if !strings.Contains(a.statusMsg, "filter cleared") {
			t.Fatalf("status = %q, want the cleared notice", a.statusMsg)
		}
	})

	t.Run("a malformed expression is reported", func(t *testing.T) {
		t.Parallel()
		a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})

		runCmd(t, a, "filter severity:")
		if !a.statusErr || !strings.Contains(a.statusMsg, "filter:") {
			t.Fatalf("status = %q (err=%v), want a parse error", a.statusMsg, a.statusErr)
		}
	})
}

// :submit refuses to run while there are unsaved changes — submitting a
// review that differs from what is on disk would post the wrong thing.
func TestSubmitRefusesWhileDirty(t *testing.T) {
	t.Parallel()
	r := reviewOnDisk(t)
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})
	a.dirty = true

	if cmd := runCmd(t, a, "submit"); cmd != nil {
		t.Fatalf("submit while dirty produced %T, want nothing", cmd())
	}
	if !a.statusErr || !strings.Contains(a.statusMsg, ":save") {
		t.Fatalf("status = %q (err=%v), want it to ask for a save first", a.statusMsg, a.statusErr)
	}
}

func TestSubmitRejectsUnknownArgs(t *testing.T) {
	t.Parallel()
	a := NewApp(Config{Review: reviewOnDisk(t), Saver: func(*model.Review) error { return nil }})

	if cmd := runCmd(t, a, "submit --wat"); cmd != nil {
		t.Fatalf("bad args produced %T, want nothing", cmd())
	}
	if !a.statusErr || !strings.Contains(a.statusMsg, "--wat") {
		t.Fatalf("status = %q (err=%v), want the bad flag echoed", a.statusMsg, a.statusErr)
	}
}

// An empty command line is a no-op: pressing ":" then enter must not
// report an error.
func TestEmptyCommandIsANoOp(t *testing.T) {
	t.Parallel()
	a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})

	if cmd := runCmd(t, a, ""); cmd != nil {
		t.Fatalf("an empty command produced %T, want nothing", cmd())
	}
	if a.statusMsg != "" {
		t.Fatalf("status = %q, want it left alone", a.statusMsg)
	}
}

func TestStateReportsTheActiveView(t *testing.T) {
	t.Parallel()
	a := NewApp(Config{Review: sampleReview(), Saver: func(*model.Review) error { return nil }})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	if got := a.State(); got != viewList {
		t.Fatalf("State() = %v, want the list view", got)
	}

	_, cmd := a.Update(runeKey('s'))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			a.Update(msg)
		}
	}
	if got := a.State(); got != viewSummary {
		t.Fatalf("State() after 's' = %v, want the summary view", got)
	}
	if !a.IsSummary() {
		t.Error("IsSummary should agree with State")
	}
}

func TestListenForChangeDeliversPaths(t *testing.T) {
	t.Parallel()
	w := &watcher{changes: make(chan string, 1)}

	w.changes <- "/tmp/x/a.md"
	msg := w.listenForChange()()
	got, ok := msg.(fsChangeMsg)
	if !ok {
		t.Fatalf("message = %T, want fsChangeMsg", msg)
	}
	if got.path != "/tmp/x/a.md" {
		t.Errorf("path = %q, want the changed file", got.path)
	}

	// Once the watcher is stopped the command retires instead of looping
	// on a dead channel.
	close(w.changes)
	if msg := w.listenForChange()(); msg != nil {
		t.Fatalf("a closed watcher should produce nil, got %T", msg)
	}
}
