package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// fsChangeMsg is dispatched when a watched .md file changes on disk.
type fsChangeMsg struct{ path string }

// fsWatcherErrMsg is dispatched once when the watcher fails to start
// (e.g. fsnotify unsupported on this platform). The TUI shows a hint
// recommending :reload for manual refresh.
type fsWatcherErrMsg struct{ err error }

// debounceWindow coalesces rapid edit events (editors often touch a file
// twice on save) into a single reload.
const debounceWindow = 200 * time.Millisecond

// watcher wraps fsnotify and feeds bubbletea via a channel. The bubbletea
// program polls via the listenForChange tea.Cmd.
type watcher struct {
	w        *fsnotify.Watcher
	changes  chan string
	stopOnce sync.Once
}

// newWatcher creates a watcher rooted at baseDir, recursively adding any
// directory that contains *.md (or might in the future). Returns nil and
// the underlying error on failure; callers should fall back to manual
// :reload.
func newWatcher(baseDir string) (*watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	wr := &watcher{w: fsw, changes: make(chan string, 8)}

	// Add baseDir and all its subdirectories.
	if err := addDirsRecursively(fsw, baseDir); err != nil {
		_ = fsw.Close()
		return nil, err
	}

	go wr.run()
	return wr, nil
}

func addDirsRecursively(w *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}

func (w *watcher) run() {
	pending := map[string]*time.Timer{}
	var mu sync.Mutex

	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			if !isMarkdownChange(ev) {
				continue
			}
			path := ev.Name
			mu.Lock()
			if t, ok := pending[path]; ok {
				t.Stop()
			}
			pending[path] = time.AfterFunc(debounceWindow, func() {
				mu.Lock()
				delete(pending, path)
				mu.Unlock()
				select {
				case w.changes <- path:
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

func (w *watcher) Stop() {
	w.stopOnce.Do(func() { _ = w.w.Close() })
}

// listenForChange returns a tea.Cmd that blocks until the next debounced
// .md change event arrives, then dispatches fsChangeMsg. The App
// re-subscribes after each event so changes keep flowing.
func (w *watcher) listenForChange() tea.Cmd {
	return func() tea.Msg {
		path, ok := <-w.changes
		if !ok {
			return nil
		}
		return fsChangeMsg{path: path}
	}
}

// isMarkdownChange returns true for write/create events on *.md files.
func isMarkdownChange(ev fsnotify.Event) bool {
	if !strings.HasSuffix(strings.ToLower(ev.Name), ".md") {
		return false
	}
	return ev.Op&(fsnotify.Write|fsnotify.Create) != 0
}
