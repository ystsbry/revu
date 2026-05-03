package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editorDoneMsg is dispatched after the external editor exits.
type editorDoneMsg struct {
	path string
	err  error
}

// editorCmd builds an exec.Cmd that opens path in $EDITOR. If $EDITOR is
// unset or empty, it falls back to "vi". The env var is split on whitespace
// to support common forms like "code --wait" or "zed --wait".
func editorCmd(path string) *exec.Cmd {
	ed := os.Getenv("EDITOR")
	if strings.TrimSpace(ed) == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed)
	parts = append(parts, path)
	c := exec.Command(parts[0], parts[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// openEditor returns a tea.Cmd that suspends the program, runs the editor,
// then dispatches editorDoneMsg with the result.
func openEditor(path string) tea.Cmd {
	return tea.ExecProcess(editorCmd(path), func(err error) tea.Msg {
		return editorDoneMsg{path: path, err: err}
	})
}

// reloadBody re-reads the file at path and updates the corresponding
// in-memory body field on the Review. It accepts either the summary file
// or any comment body file.
func (a *App) reloadBody(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	clean := filepath.Clean(path)

	summaryAbs := filepath.Clean(filepath.Join(a.review.BaseDir, a.review.SummaryFile))
	if clean == summaryAbs {
		a.review.SummaryBody = string(raw)
		return nil
	}
	for i := range a.review.Comments {
		c := &a.review.Comments[i]
		cAbs := filepath.Clean(filepath.Join(a.review.BaseDir, c.BodyFile))
		if clean == cAbs {
			c.Body = string(raw)
			return nil
		}
	}
	return fmt.Errorf("path not in review: %s", path)
}
