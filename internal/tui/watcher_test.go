package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestIsMarkdownChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ev   fsnotify.Event
		want bool
	}{
		{"md write", fsnotify.Event{Name: "/tmp/c1.md", Op: fsnotify.Write}, true},
		{"md create", fsnotify.Event{Name: "/tmp/c1.md", Op: fsnotify.Create}, true},
		{"md upper", fsnotify.Event{Name: "/tmp/C1.MD", Op: fsnotify.Write}, true},
		{"md remove", fsnotify.Event{Name: "/tmp/c1.md", Op: fsnotify.Remove}, false},
		{"yaml write", fsnotify.Event{Name: "/tmp/review.yml", Op: fsnotify.Write}, false},
		{"go write", fsnotify.Event{Name: "/tmp/x.go", Op: fsnotify.Write}, false},
	}
	for _, tc := range cases {
		if got := isMarkdownChange(tc.ev); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWatcherEmitsDebouncedChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "comments"), 0o755); err != nil {
		t.Fatal(err)
	}
	c1 := filepath.Join(dir, "comments", "c1.md")
	if err := os.WriteFile(c1, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := newWatcher(dir)
	if err != nil {
		t.Skipf("fsnotify unavailable on this platform: %v", err)
	}
	defer w.Stop()

	// Trigger a couple of rapid writes; expect a single coalesced event.
	if err := os.WriteFile(c1, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c1, []byte("v3"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case path := <-w.changes:
		if filepath.Clean(path) != filepath.Clean(c1) {
			t.Errorf("got change for %q, want %q", path, c1)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for change event")
	}
}
