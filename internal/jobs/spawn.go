package jobs

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Spawn starts bin with args as a detached process: its own session
// (setsid) so it survives the parent and receives no terminal signals,
// stdout/stderr appended to logPath, stdin from /dev/null. Returns the
// child PID without waiting for it.
//
// A goroutine reaps the child when the parent is long-lived (the
// dashboard); a parent that exits first just leaves the child to be
// re-parented to init, which is the point of the new session.
func Spawn(bin string, args []string, logPath string) (int, error) {
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open job log: %w", err)
	}
	// The parent's copy of the fd can close as soon as Start has handed
	// the child its own; a close error here has nothing to report.
	defer func() { _ = logf.Close() }()

	cmd := exec.Command(bin, args...)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("spawn %s: %w", bin, err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// SpawnWorker launches the hidden review worker for job via the given revu
// binary (normally os.Executable()).
func SpawnWorker(revuBin string, job Job) (int, error) {
	return Spawn(revuBin, []string{"_review-worker", "--job", job.ID}, job.LogPath)
}
