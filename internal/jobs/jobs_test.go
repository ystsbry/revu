package jobs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("REVU_HOME", dir)
	return dir
}

func TestNewSaveLoadRoundTrip(t *testing.T) {
	setHome(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	j, err := New("owner/repo", 42, "claude", "bg", "security", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"20260810-120000", "owner-repo", "pr42"} {
		if !strings.Contains(j.ID, want) {
			t.Errorf("id %q missing %q", j.ID, want)
		}
	}
	if j.State != StateRunning || j.LogPath == "" || !j.StartedAt.Equal(now) {
		t.Fatalf("unexpected new job: %+v", j)
	}

	j.WorkDir = "/clones/repo"
	if err := Save(j); err != nil {
		t.Fatal(err)
	}
	got, err := Load(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "owner/repo" || got.PR != 42 || got.Engine != "claude" ||
		got.Focus != "security" || got.WorkDir != "/clones/repo" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestListSortsNewestFirstAndSkipsBroken(t *testing.T) {
	home := setHome(t)
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	for i, name := range []string{"old", "new"} {
		j, err := New("o/r", i+1, "claude", "bg", "", base.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		j.WorkDir = name
		if err := Save(j); err != nil {
			t.Fatal(err)
		}
	}
	// A torn write must not take the list down.
	if err := os.WriteFile(filepath.Join(home, "jobs", "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	all, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("list = %d jobs, want 2", len(all))
	}
	if all[0].WorkDir != "new" || all[1].WorkDir != "old" {
		t.Errorf("order wrong: %s, %s", all[0].WorkDir, all[1].WorkDir)
	}
}

func TestEffectiveDetectsDeadWorkers(t *testing.T) {
	now := time.Now()

	// Terminal states pass through untouched.
	done := Job{State: StateDone}
	if st, _ := done.Effective(now); st != StateDone {
		t.Errorf("done job = %s", st)
	}

	// Running with a live PID (our own) stays running.
	live := Job{State: StateRunning, PID: os.Getpid(), StartedAt: now}
	if st, _ := live.Effective(now); st != StateRunning {
		t.Errorf("live worker = %s, want running", st)
	}

	// Running with a dead PID reads as failed.
	dead := Job{State: StateRunning, PID: findDeadPID(t), StartedAt: now}
	st, reason := dead.Effective(now)
	if st != StateFailed || !strings.Contains(reason, "died") {
		t.Errorf("dead worker = %s (%q), want failed", st, reason)
	}

	// No PID yet: fine within the grace window, failed after it.
	fresh := Job{State: StateRunning, StartedAt: now.Add(-time.Second)}
	if st, _ := fresh.Effective(now); st != StateRunning {
		t.Errorf("booting worker = %s, want running", st)
	}
	stuck := Job{State: StateRunning, StartedAt: now.Add(-2 * startGrace)}
	if st, _ := stuck.Effective(now); st != StateFailed {
		t.Errorf("never-started worker = %s, want failed", st)
	}
}

// findDeadPID returns a PID that certainly refers to no live process: it
// spawns a child, waits for it to exit, and returns its (now reaped) PID.
func findDeadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestRunningForGuard(t *testing.T) {
	setHome(t)
	now := time.Now()

	j, err := New("o/r", 7, "claude", "bg", "", now)
	if err != nil {
		t.Fatal(err)
	}
	j.PID = os.Getpid() // "alive"
	if err := Save(j); err != nil {
		t.Fatal(err)
	}

	if _, ok := RunningFor("o/r", 7, now); !ok {
		t.Error("live job for o/r#7 should be reported")
	}
	if _, ok := RunningFor("o/r", 8, now); ok {
		t.Error("different PR must not match")
	}

	// A dead worker releases the guard.
	j.PID = findDeadPID(t)
	if err := Save(j); err != nil {
		t.Fatal(err)
	}
	if _, ok := RunningFor("o/r", 7, now); ok {
		t.Error("dead job must not block a new start")
	}
}

func TestSpawnDetachedWritesLog(t *testing.T) {
	home := setHome(t)
	logPath := filepath.Join(home, "spawn-test.log")

	pid, err := Spawn("/bin/sh", []string{"-c", "echo spawned-ok"}, logPath)
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 {
		t.Fatalf("pid = %d", pid)
	}

	// The child is detached; poll briefly for its output.
	deadline := time.Now().Add(3 * time.Second)
	for {
		raw, _ := os.ReadFile(logPath)
		if strings.Contains(string(raw), "spawned-ok") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("log never received child output: %q", raw)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
