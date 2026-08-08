package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The in-process tests drive cobra with fake engines, which cannot show
// what the *process* writes: claude's own stderr and the stream relay's
// diagnostics bypass cobra's writers entirely. --json promises that stdout
// carries nothing but the result object, so prove it by running the built
// binary against a fake claude CLI and reading the two streams apart.

// buildRevu compiles the CLI into a temp dir and returns its path.
func buildRevu(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI is a POSIX shell script")
	}
	if testing.Short() {
		t.Skip("builds the binary; skipped under -short")
	}
	bin := filepath.Join(t.TempDir(), "revu")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v: %s", err, stderr.String())
	}
	return bin
}

// e2eEnv sets up a git checkout whose origin is owner/repo, a REVU_HOME to
// write reviews into, and a fake claude on PATH. writesReview controls
// whether the fake produces a review at all.
func e2eEnv(t *testing.T, writesReview bool) (workdir string, env []string, revuHome string) {
	t.Helper()
	base := t.TempDir()
	workdir = filepath.Join(base, "repo")
	revuHome = filepath.Join(base, "revu-home")
	binDir := filepath.Join(base, "bin")
	for _, d := range []string{workdir, revuHome, binDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// revu resolves the repository from the cwd's origin remote.
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "https://github.com/owner/repo.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = workdir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	outDir := filepath.Join(revuHome, "owner", "repo", "pr-42", "a1b2c3d")
	body := `#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-e2e"}'
printf 'claude noise on stderr\n' >&2
`
	if writesReview {
		body += "mkdir -p \"" + outDir + "\"\n" +
			"printf 'pr: 42\\ngenerated_by:\\n  tool: claude-code\\n' > \"" + outDir + "/review.yml\"\n"
	}
	body += `printf '%s\n' '{"type":"result","duration_ms":1000,"num_turns":1}'
`
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(body), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	env = append(os.Environ(),
		"REVU_HOME="+revuHome,
		"HOME="+base,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	return workdir, env, revuHome
}

func runRevu(t *testing.T, bin, workdir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = workdir
	cmd.Env = env
	cmd.Stdin = strings.NewReader("")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errOut.String(), code
}

func TestE2ENonInteractiveJSONStdoutIsPure(t *testing.T) {
	bin := buildRevu(t)
	workdir, env, revuHome := e2eEnv(t, true)

	stdout, stderr, code := runRevu(t, bin, workdir, env, "review", "42", "--no-resume", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not exactly one JSON object (%v): %q", err, stdout)
	}
	wantDir := filepath.Join(revuHome, "owner", "repo", "pr-42", "a1b2c3d")
	if got["out_dir"] != wantDir {
		t.Errorf("out_dir = %v, want %v", got["out_dir"], wantDir)
	}
	if got["session_id"] != "sess-e2e" || got["repo"] != "owner/repo" || got["engine"] != "claude" {
		t.Errorf("unexpected result object: %v", got)
	}

	// Everything else — revu's status lines, the relay, and claude's own
	// stderr — has to be on the other stream.
	for _, want := range []string{"Generating review for owner/repo#42", "claude session ready", "claude noise on stderr"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr should contain %q, got %q", want, stderr)
		}
	}

	// The session id must be recorded regardless of resume.
	raw, err := os.ReadFile(filepath.Join(wantDir, "review.yml"))
	if err != nil {
		t.Fatalf("read review.yml: %v", err)
	}
	if !strings.Contains(string(raw), "session_id: sess-e2e") {
		t.Errorf("generated_by.session_id not written:\n%s", raw)
	}
}

func TestE2ENonInteractiveFailsWhenNoReviewWritten(t *testing.T) {
	bin := buildRevu(t)
	workdir, env, _ := e2eEnv(t, false)

	stdout, stderr, code := runRevu(t, bin, workdir, env, "review", "42", "--no-resume", "--json")
	if code == 0 {
		t.Fatalf("expected a non-zero exit when no review was generated\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout must stay empty when generation failed, got %q", stdout)
	}
	if !strings.Contains(stderr, "may have failed silently") {
		t.Errorf("stderr should explain the failure, got %q", stderr)
	}
}

func TestE2EJSONWithoutNoResumeKeepsStdoutClean(t *testing.T) {
	bin := buildRevu(t)
	workdir, env, _ := e2eEnv(t, true)

	stdout, stderr, code := runRevu(t, bin, workdir, env, "review", "42", "--json")
	if code == 0 {
		t.Fatal("expected --json without --no-resume to fail")
	}
	// SilenceUsage on the root keeps the usage dump off stdout, which is
	// what makes "parse stdout unconditionally" safe for consumers.
	if stdout != "" {
		t.Errorf("stdout should be empty, got %q", stdout)
	}
	if !strings.Contains(stderr, "--json requires --no-resume") {
		t.Errorf("stderr should explain the rejected flag combination, got %q", stderr)
	}
}

// --- background jobs (--bg) ------------------------------------------------

// startBGJob runs `revu review 42 --bg` and returns the job id parsed from
// its output.
func startBGJob(t *testing.T, bin, workdir string, env []string) string {
	t.Helper()
	stdout, stderr, code := runRevu(t, bin, workdir, env, "review", "42", "--bg")
	if code != 0 {
		t.Fatalf("--bg exit code = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	m := regexp.MustCompile(`job (\S+) \(pid (\d+)\)`).FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("no job id in --bg output: %q", stdout)
	}
	return m[1]
}

// waitJobTerminal polls the job book until the job leaves running.
func waitJobTerminal(t *testing.T, revuHome, id string) map[string]any {
	t.Helper()
	path := filepath.Join(revuHome, "jobs", id+".json")
	deadline := time.Now().Add(15 * time.Second)
	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			var j map[string]any
			if json.Unmarshal(raw, &j) == nil && j["state"] != "running" {
				return j
			}
		}
		if time.Now().After(deadline) {
			raw, _ := os.ReadFile(path)
			t.Fatalf("job %s never finished: %s", id, raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// The acceptance criteria driven through the real binary: --bg registers a
// job, a detached worker transitions it running→done, and the finished job
// carries session_id + out_dir with review.yml patched.
func TestE2EBackgroundJobRunsToDone(t *testing.T) {
	bin := buildRevu(t)
	workdir, env, revuHome := e2eEnv(t, true)

	id := startBGJob(t, bin, workdir, env)
	job := waitJobTerminal(t, revuHome, id)

	if job["state"] != "done" {
		t.Fatalf("job = %v, want done", job)
	}
	wantDir := filepath.Join(revuHome, "owner", "repo", "pr-42", "a1b2c3d")
	if job["out_dir"] != wantDir || job["session_id"] != "sess-e2e" {
		t.Errorf("job result = %v", job)
	}

	// session_id must land in review.yml exactly like a foreground run.
	raw, err := os.ReadFile(filepath.Join(wantDir, "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "session_id: sess-e2e") {
		t.Errorf("generated_by.session_id not written:\n%s", raw)
	}

	// The job book surfaces through the CLI, and the log holds the relay.
	stdout, _, code := runRevu(t, bin, workdir, env, "jobs", "list")
	if code != 0 || !strings.Contains(stdout, "done") || !strings.Contains(stdout, id) {
		t.Errorf("jobs list = %q (code %d)", stdout, code)
	}
	stdout, _, code = runRevu(t, bin, workdir, env, "jobs", "log", id)
	if code != 0 || !strings.Contains(stdout, "Generating review for owner/repo#42") {
		t.Errorf("jobs log = %q (code %d)", stdout, code)
	}
}

// A worker whose agent produces nothing must record the failure in the
// job book (the dashboard's red badge feeds on this).
func TestE2EBackgroundJobRecordsFailure(t *testing.T) {
	bin := buildRevu(t)
	workdir, env, revuHome := e2eEnv(t, false)

	id := startBGJob(t, bin, workdir, env)
	job := waitJobTerminal(t, revuHome, id)

	if job["state"] != "failed" {
		t.Fatalf("job = %v, want failed", job)
	}
	errText, _ := job["err"].(string)
	if !strings.Contains(errText, "locate review dir") {
		t.Errorf("failure cause not recorded: %q", errText)
	}

	stdout, _, code := runRevu(t, bin, workdir, env, "jobs", "list")
	if code != 0 || !strings.Contains(stdout, "failed") {
		t.Errorf("jobs list should show the failure: %q (code %d)", stdout, code)
	}
}
