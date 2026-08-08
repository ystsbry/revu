package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/ystsbry/revu/internal/jobs"
)

// jobsChangedMsg is dispatched when the job book changes: a job was
// created or transitioned state. Log appends do not trigger it.
type jobsChangedMsg struct{}

// jobsDebounce coalesces the burst a single transition produces (temp
// file, rename, parent + worker writes) into one reload.
const jobsDebounce = 300 * time.Millisecond

// jobsWatcher streams job-book changes to bubbletea via a channel, in the
// same pattern as the review TUI's file watcher. Nil-safe: a nil watcher
// (fsnotify unavailable) simply never fires, and the screens fall back to
// manual [r] reload.
type jobsWatcher struct {
	w        *fsnotify.Watcher
	changes  chan struct{}
	stopOnce sync.Once
}

// newJobsWatcher watches the job-book directory, creating it if missing
// (fsnotify cannot watch a path that does not exist yet). Returns an
// error when the platform has no watch facility.
func newJobsWatcher() (*jobsWatcher, error) {
	dir, err := jobs.Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fsw.Add(dir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	wr := &jobsWatcher{w: fsw, changes: make(chan struct{}, 1)}
	go wr.run()
	return wr, nil
}

func (w *jobsWatcher) run() {
	var mu sync.Mutex
	var pending *time.Timer

	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			if !isJobBookChange(ev) {
				continue
			}
			mu.Lock()
			if pending != nil {
				pending.Stop()
			}
			pending = time.AfterFunc(jobsDebounce, func() {
				select {
				case w.changes <- struct{}{}:
				default:
				}
			})
			mu.Unlock()
		case _, ok := <-w.w.Errors:
			if !ok {
				return
			}
		}
	}
}

// isJobBookChange accepts only finished job records: the atomic-write temp
// files (dot-prefixed) and the ever-growing .log files are filtered out,
// so a running agent's output stream cannot hammer the GitHub-backed
// screens with reloads.
func isJobBookChange(ev fsnotify.Event) bool {
	name := filepath.Base(ev.Name)
	if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
		return false
	}
	return ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0
}

// Close stops the watcher. Safe on nil and safe to call twice.
func (w *jobsWatcher) Close() error {
	if w == nil {
		return nil
	}
	w.stopOnce.Do(func() { _ = w.w.Close() })
	return nil
}

// wait returns a Cmd that blocks until the next job-book change. The
// screen re-arms it after each message. Nil-safe: a nil watcher blocks
// forever on a nil channel, which bubbletea tolerates as a goroutine that
// never delivers.
func (w *jobsWatcher) wait() tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-w.changes
		if !ok {
			return nil
		}
		return jobsChangedMsg{}
	}
}
