package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// save creates a job record for slug#pr started at the given time.
func save(t *testing.T, slug string, pr int, started time.Time) Job {
	t.Helper()
	j, err := New(slug, pr, "claude", "bg", "", started)
	if err != nil {
		t.Fatal(err)
	}
	j.StartedAt = started
	if err := Save(j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestLatestForPR(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	base := time.Now().Add(-time.Hour)

	if _, ok := LatestForPR("o/r", 1); ok {
		t.Fatal("an empty job book should report no job")
	}

	older := save(t, "o/r", 1, base)
	newer := save(t, "o/r", 1, base.Add(30*time.Minute))
	// Same PR number in a different repo must not be picked up.
	save(t, "other/repo", 1, base.Add(45*time.Minute))
	// Same repo, different PR.
	save(t, "o/r", 2, base.Add(50*time.Minute))

	got, ok := LatestForPR("o/r", 1)
	if !ok {
		t.Fatal("the seeded job should be found")
	}
	if got.ID != newer.ID {
		t.Fatalf("LatestForPR = %q, want the newer job %q (older was %q)", got.ID, newer.ID, older.ID)
	}

	if got, ok := LatestForPR("o/r", 2); !ok || got.PR != 2 {
		t.Fatalf("LatestForPR(o/r, 2) = (%+v, %v), want PR 2", got, ok)
	}
	if _, ok := LatestForPR("o/r", 99); ok {
		t.Fatal("a PR with no jobs should report none")
	}
}

// An unreadable job book is reported as "no job" rather than an error: the
// badge is decoration, and failing there would take down the PR list.
func TestLatestForPRSurvivesAnUnreadableBook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)

	// A regular file where the jobs directory should be makes List fail.
	if err := os.WriteFile(filepath.Join(home, "jobs"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LatestForPR("o/r", 1); ok {
		t.Fatal("an unreadable job book should report no job")
	}
}

func TestSaveRejectsAJobWithoutAnID(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())

	if err := Save(Job{}); err == nil {
		t.Fatal("a job with no id should not be saved")
	}
}

// Save is atomic (temp + rename) and must leave no .tmp behind.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	save(t, "o/r", 1, time.Now())

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

// Saving the same ID twice overwrites in place — that is how a worker
// records its terminal state.
func TestSaveOverwritesTheSameJob(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	j := save(t, "o/r", 1, time.Now())

	j.State = StateDone
	if err := Save(j); err != nil {
		t.Fatal(err)
	}

	all, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d records, want the job overwritten in place", len(all))
	}
	if all[0].State != StateDone {
		t.Fatalf("state = %q, want done", all[0].State)
	}
}

// Dir hangs off REVU_HOME, which is what lets every test here run against
// a temp directory instead of the real ~/.revu.
func TestDirFollowsRevuHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "jobs"); dir != want {
		t.Fatalf("Dir() = %q, want %q", dir, want)
	}
}

func TestNewAssignsDistinctIDsAndALogPath(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	now := time.Now()

	a, err := New("o/r", 1, "claude", "bg", "", now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("o/r", 1, "claude", "bg", "", now)
	if err != nil {
		t.Fatal(err)
	}

	// Two jobs started in the same second must not collide on disk.
	if a.ID == b.ID {
		t.Fatalf("both jobs got the id %q", a.ID)
	}
	if a.State != StateRunning {
		t.Errorf("state = %q, want running", a.State)
	}
	if filepath.Ext(a.LogPath) != ".log" {
		t.Errorf("LogPath = %q, want a .log file", a.LogPath)
	}
	// The slug is sanitised into the id so it stays a safe filename.
	if filepath.Base(a.LogPath) != a.ID+".log" {
		t.Errorf("LogPath = %q, want it named after the id", a.LogPath)
	}
}

func TestNewSanitisesTheSlugIntoTheID(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())

	j, err := New("owner/repo.name", 7, "codex", "bg", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(j.ID) != j.ID {
		t.Fatalf("id %q contains a path separator", j.ID)
	}
	if want := "pr7"; !strings.Contains(j.ID, want) {
		t.Fatalf("id = %q, want it to carry %q", j.ID, want)
	}
}
