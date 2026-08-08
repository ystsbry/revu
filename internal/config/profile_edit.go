// Machine editing of the active_profile key in the user config.toml.
//
// Same policy as the [[repo]] editor (repos_edit.go): the edit touches only
// the one thing it owns — here the top-level "active_profile = ..." line —
// and preserves everything else byte for byte. TOML requires top-level keys
// to appear before the first table header, so insertion happens at the top
// of the file, after any leading comment block.
package config

import (
	"fmt"
	"strings"
)

// SetActiveUserProfile persists the active profile choice in the user
// config.toml. Empty or "default" removes the key (= back to showing every
// registered repo). The file is created when missing and a profile is set.
func SetActiveUserProfile(name string) error {
	path, err := UserConfigPath()
	if err != nil {
		return err
	}
	lines, err := readLines(path)
	if err != nil {
		return err
	}

	clearing := name == "" || name == DefaultProfileName
	idx := activeProfileLineIndex(lines)

	switch {
	case clearing && idx < 0:
		return nil // nothing to clear
	case clearing:
		lines = append(lines[:idx], lines[idx+1:]...)
	case idx >= 0:
		lines[idx] = formatActiveProfile(name)
	default:
		if lines == nil {
			lines = []string{"# revu configuration. Managed keys are edited in place by 'revu'."}
		}
		lines = insertTopLevel(lines, formatActiveProfile(name))
	}
	return writeValidated(path, lines)
}

func formatActiveProfile(name string) string {
	return fmt.Sprintf("active_profile = %q", name)
}

// activeProfileLineIndex finds the top-level active_profile assignment: the
// search stops at the first table header, because the same key below a
// header would belong to that table, not to the document root.
func activeProfileLineIndex(lines []string) int {
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[") {
			return -1
		}
		if strings.HasPrefix(trimmed, "active_profile") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "active_profile"))
			if strings.HasPrefix(rest, "=") {
				return i
			}
		}
	}
	return -1
}

// insertTopLevel places line in the document's top-level region: right
// before the first table header, or at EOF when there is none. A blank
// separator keeps it visually apart from what follows.
func insertTopLevel(lines []string, line string) []string {
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			out := make([]string, 0, len(lines)+2)
			out = append(out, lines[:i]...)
			out = append(out, line, "")
			return append(out, lines[i:]...)
		}
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines, line)
}
