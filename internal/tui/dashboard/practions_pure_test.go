package dashboard

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ystsbry/revu/internal/jobs"
	"github.com/ystsbry/revu/internal/model"
)

func writeLog(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "job.log")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTailLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		n    int
		want []string
	}{
		{
			name: "fewer lines than asked for",
			body: "one\ntwo\n",
			n:    5,
			want: []string{"one", "two"},
		},
		{
			name: "keeps only the last n",
			body: "1\n2\n3\n4\n5\n",
			n:    2,
			want: []string{"4", "5"},
		},
		{
			name: "a missing trailing newline still counts",
			body: "1\n2\n3",
			n:    2,
			want: []string{"2", "3"},
		},
		{
			name: "trailing blank lines are trimmed",
			body: "1\n2\n\n\n",
			n:    5,
			want: []string{"1", "2"},
		},
		{
			name: "an empty file yields nothing",
			body: "",
			n:    5,
			want: nil,
		},
		{
			name: "a newline-only file yields nothing",
			body: "\n",
			n:    5,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tailLines(writeLog(t, tt.body), tt.n)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("tailLines = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTailLinesOnMissingFile(t *testing.T) {
	t.Parallel()
	if got := tailLines(filepath.Join(t.TempDir(), "nope.log"), 5); got != nil {
		t.Fatalf("a missing log should read as nil, got %q", got)
	}
}

// Only the trailing 32KB is read, so a chatty agent log stays cheap to
// poll. The partial first line inside that window is dropped.
func TestTailLinesReadsOnlyTheTrailingWindow(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for i := 0; i < 4000; i++ {
		b.WriteString(strings.Repeat("x", 20) + strconv.Itoa(i) + "\n")
	}
	path := writeLog(t, b.String())

	got := tailLines(path, 3)
	if len(got) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(got), got)
	}
	if !strings.HasSuffix(got[2], "3999") {
		t.Errorf("last line = %q, want the final log line", got[2])
	}
	for _, l := range got {
		if !strings.HasPrefix(l, strings.Repeat("x", 20)) {
			t.Errorf("line %q looks cut mid-way; the partial line should be dropped", l)
		}
	}
}

func TestLoadJobInfo(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())

	if j, tail := loadJobInfo("o/r", 1); j != nil || tail != nil {
		t.Fatalf("no job should read as (nil, nil), got (%v, %q)", j, tail)
	}

	seeded := seedTestJob(t, "o/r", 1, jobs.StateRunning, func(j *jobs.Job) {
		j.LogPath = writeLog(t, "starting\nworking\ndone\n")
	})

	j, tail := loadJobInfo("o/r", 1)
	if j == nil {
		t.Fatal("the seeded job should be found")
	}
	if j.ID != seeded.ID {
		t.Errorf("job ID = %q, want %q", j.ID, seeded.ID)
	}
	if len(tail) == 0 || tail[len(tail)-1] != "done" {
		t.Errorf("log tail = %q, want it to end with the last log line", tail)
	}
}

// A job whose log file was never created must still return the job — the
// panel shows its state even when there is nothing to tail.
func TestLoadJobInfoWithoutALog(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	seedTestJob(t, "o/r", 2, jobs.StateRunning, func(j *jobs.Job) {
		j.LogPath = filepath.Join(t.TempDir(), "missing.log")
	})

	j, tail := loadJobInfo("o/r", 2)
	if j == nil {
		t.Fatal("the job should be found even without a log")
	}
	if tail != nil {
		t.Errorf("log tail = %q, want nil", tail)
	}
}

func TestReloadSubmissionMeta(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	dir := seedReviewedPR(t, os.Getenv("REVU_HOME"), "o/r", 7, "abc1234")

	// A review that has since been submitted on disk.
	body := "" +
		"schema_version: 1\n" +
		"pr:\n  repo: o/r\n  number: 7\n  head_sha: abc1234567890\n" +
		"review_event: COMMENT\n" +
		"summary_file: summary.md\n" +
		"submitted_at: 2026-08-11T00:00:00Z\n" +
		"review_id: 4242\n" +
		"comments: []\n"
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &model.Review{BaseDir: dir}
	if err := reloadSubmissionMeta(r); err != nil {
		t.Fatal(err)
	}
	if r.SubmittedAt == nil {
		t.Fatal("SubmittedAt should have been copied from disk")
	}
	if got := r.SubmittedAt.UTC().Format(time.RFC3339); got != "2026-08-11T00:00:00Z" {
		t.Errorf("SubmittedAt = %s, want the timestamp on disk", got)
	}
	if r.ReviewID == nil || *r.ReviewID != 4242 {
		t.Errorf("ReviewID = %v, want 4242", r.ReviewID)
	}
}

func TestReloadSubmissionMetaOnUnreadableReview(t *testing.T) {
	t.Parallel()
	r := &model.Review{BaseDir: filepath.Join(t.TempDir(), "not-a-review")}
	if err := reloadSubmissionMeta(r); err == nil {
		t.Fatal("a missing review.yml should surface as an error")
	}
}

func TestFirstLineOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single line is untouched", in: "boom", want: "boom"},
		{name: "multi-line is elided", in: "boom\nstack\nframe", want: "boom ..."},
		{name: "empty stays empty", in: "", want: ""},
		{name: "leading newline elides to an empty head", in: "\nrest", want: " ..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstLineOf(tt.in); got != tt.want {
				t.Fatalf("firstLineOf(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The [background] action needs a clone on disk; an unregistered slug has
// to say so rather than starting a job in the wrong directory.
func TestCloneDirFor(t *testing.T) {
	clone := t.TempDir()
	writeConfig(t, "[[repo]]\nslug = \"o/known\"\npath = \""+clone+"\"\n")

	got, err := cloneDirFor("o/known")
	if err != nil {
		t.Fatal(err)
	}
	if got != clone {
		t.Errorf("cloneDirFor(o/known) = %q, want %q", got, clone)
	}

	if _, err := cloneDirFor("o/unknown"); err == nil {
		t.Error("an unregistered slug should be an error")
	} else if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error = %q, want it to say the repo is not registered", err)
	}

	// Registered but the clone was deleted since.
	writeConfig(t, "[[repo]]\nslug = \"o/gone\"\npath = \""+filepath.Join(t.TempDir(), "removed")+"\"\n")
	if _, err := cloneDirFor("o/gone"); err == nil {
		t.Error("a missing clone directory should be an error")
	}
}

// NewPRActions wires loadJob to the real job-book lookup; without an
// override the panel must find a live job on its own.
func TestNewPRActionsLoadsTheLiveJobByDefault(t *testing.T) {
	t.Setenv("REVU_HOME", t.TempDir())
	seedTestJob(t, "o/r", 3, jobs.StateRunning, func(j *jobs.Job) {
		j.LogPath = writeLog(t, "tick\n")
	})

	m := NewPRActions("o/r", PRItem{Number: 3})
	msg := m.reloadJob()()
	got, ok := msg.(prActionsJobMsg)
	if !ok {
		t.Fatalf("reloadJob produced %T, want prActionsJobMsg", msg)
	}
	if got.job == nil || got.job.State != jobs.StateRunning {
		t.Fatalf("job = %+v, want the running job", got.job)
	}
	if len(got.tail) == 0 || got.tail[0] != "tick" {
		t.Errorf("tail = %q, want the log contents", got.tail)
	}
}
