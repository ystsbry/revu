package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// submitDoneMsg is dispatched when the spawned `revu submit` subprocess exits.
type submitDoneMsg struct {
	dryRun bool
	err    error
}

// runSubmit suspends bubbletea and re-execs the same revu binary with
// `submit [--dry-run] <BaseDir>`. The subprocess inherits stdin/stdout/stderr
// so the typed-confirmation prompt works in the user's real terminal.
func (a *App) runSubmit(dryRun bool) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		a.setError(fmt.Sprintf("submit: locate self: %v", err))
		return nil
	}
	args := []string{"submit"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, a.review.BaseDir)

	c := exec.Command(self, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return submitDoneMsg{dryRun: dryRun, err: err}
	})
}

// parseSubmitArgs extracts the flag part of a ":submit ..." command body.
// Returns (recognized, dryRun, errMsg).
func parseSubmitArgs(rest string) (recognized bool, dryRun bool, errMsg string) {
	rest = strings.TrimSpace(rest)
	switch rest {
	case "":
		return true, false, ""
	case "--dry-run", "-n":
		return true, true, ""
	default:
		return false, false, fmt.Sprintf("unknown submit args: %s", rest)
	}
}
