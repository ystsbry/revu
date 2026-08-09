// Package jobs is the background-review job book: one JSON file per job
// under ~/.revu/jobs/, written by the revu process that owns the job and
// read by everything else (CLI, dashboard).
//
// Concurrency model: the job file has a single writer at any moment — the
// parent writes the initial record before spawning the worker, and from
// then on only the worker updates it (including its own PID). Readers
// never write; a crashed worker is detected on the read side by probing
// the recorded PID, so no lock files are needed.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/ystsbry/revu/internal/store"
)

// State is a job's recorded lifecycle stage.
type State string

const (
	StateRunning State = "running"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

// Job is one background review run, as persisted in the job book.
type Job struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`   // "owner/repo"
	PR     int    `json:"pr"`     // PR number
	Engine string `json:"engine"` // "claude" | "codex"
	Mode   string `json:"mode"`   // "bg" today; "fg" reserved for WS-E
	Focus  string `json:"focus,omitempty"`
	// Model is the per-run model override handed to the engine. Empty
	// means the agent CLI's own default.
	Model string `json:"model,omitempty"`

	State State `json:"state"`
	PID   int   `json:"pid,omitempty"`

	// WorkDir is the clone the worker runs the skill in, resolved by the
	// parent at start time (registry or cwd) so the worker never guesses.
	WorkDir string `json:"work_dir"`
	LogPath string `json:"log_path"`

	OutDir    string `json:"out_dir,omitempty"`
	SessionID string `json:"session_id,omitempty"`

	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Err        string     `json:"err,omitempty"`
}

// startGrace is how long a running record may sit without a PID before the
// read side declares the worker lost: the gap between the parent writing
// the record and the worker booting far enough to record its own PID.
const startGrace = time.Minute

// Dir returns the job-book directory (~/.revu/jobs).
func Dir() (string, error) {
	home, err := store.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "jobs"), nil
}

var idSanitizer = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// New builds a fresh Job for slug#pr in the running state, with its ID and
// LogPath assigned. The caller fills WorkDir and saves it before spawning.
func New(slug string, pr int, engine, mode, focus string, now time.Time) (Job, error) {
	dir, err := Dir()
	if err != nil {
		return Job{}, err
	}
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return Job{}, err
	}
	id := fmt.Sprintf("%s-%s-pr%d-%s",
		now.Format("20060102-150405"),
		strings.Trim(idSanitizer.ReplaceAllString(slug, "-"), "-"),
		pr,
		hex.EncodeToString(suffix),
	)
	return Job{
		ID:        id,
		Slug:      slug,
		PR:        pr,
		Engine:    engine,
		Mode:      mode,
		Focus:     focus,
		State:     StateRunning,
		LogPath:   filepath.Join(dir, id+".log"),
		StartedAt: now,
	}, nil
}

// Save writes j to the job book atomically (temp + rename).
func Save(j Job) error {
	if j.ID == "" {
		return errors.New("job has no id")
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(dir, ".job-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, j.ID+".json"))
}

// Load reads one job by id. The record is returned as written; use
// Effective for the crash-aware state.
func Load(id string) (Job, error) {
	dir, err := Dir()
	if err != nil {
		return Job{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return Job{}, err
	}
	var j Job
	if err := json.Unmarshal(raw, &j); err != nil {
		return Job{}, fmt.Errorf("parse job %s: %w", id, err)
	}
	return j, nil
}

// List returns every job in the book, newest first. Files that fail to
// parse are skipped — a torn write must not take the whole list down.
func List() ([]Job, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Job
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		j, err := Load(strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].StartedAt.After(out[k].StartedAt) })
	return out, nil
}

// Effective returns the state a reader should act on, detecting workers
// that died without writing a terminal state: a running record whose PID
// is gone (or that never got a PID within the start grace) reads as
// failed. The job file itself is not rewritten — the book stays
// single-writer.
func (j Job) Effective(now time.Time) (State, string) {
	if j.State != StateRunning {
		return j.State, j.Err
	}
	if j.PID > 0 && !pidAlive(j.PID) {
		return StateFailed, fmt.Sprintf("worker (pid %d) died without recording a result", j.PID)
	}
	if j.PID == 0 && now.Sub(j.StartedAt) > startGrace {
		return StateFailed, "worker never started"
	}
	return StateRunning, j.Err
}

// RunningFor reports the newest job for slug#pr that is still effectively
// running. Used as the duplicate-start guard.
func RunningFor(slug string, pr int, now time.Time) (Job, bool) {
	all, err := List()
	if err != nil {
		return Job{}, false
	}
	for _, j := range all {
		if j.Slug != slug || j.PR != pr {
			continue
		}
		if st, _ := j.Effective(now); st == StateRunning {
			return j, true
		}
	}
	return Job{}, false
}

// LatestForPR returns the newest job for slug#pr, if any. The dashboard
// uses this for badge synthesis.
func LatestForPR(slug string, pr int) (Job, bool) {
	all, err := List()
	if err != nil {
		return Job{}, false
	}
	for _, j := range all {
		if j.Slug == slug && j.PR == pr {
			return j, true
		}
	}
	return Job{}, false
}

// pidAlive probes pid with signal 0. EPERM still means "exists".
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
