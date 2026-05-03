package store

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CwdRepoRoot returns the absolute path to the top-level directory of the
// git repository containing the current working directory. Errors if cwd
// is not inside a git work tree.
func CwdRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("git rev-parse returned empty toplevel")
	}
	return root, nil
}

// VerifyRepoMatches checks that cwd is inside a git repo whose origin remote
// resolves to the expected "owner/repo" slug. Returns the absolute repo root
// on success. Used to guard `revu open` against being run in the wrong tree.
func VerifyRepoMatches(expectedSlug string) (string, error) {
	root, err := CwdRepoRoot()
	if err != nil {
		return "", err
	}
	gotSlug, err := CurrentRepoSlug()
	if err != nil {
		return "", fmt.Errorf("read git remote: %w", err)
	}
	if gotSlug != expectedSlug {
		return "", fmt.Errorf("repo mismatch: cwd is %s, but review is for %s", gotSlug, expectedSlug)
	}
	return root, nil
}

// FileInRepo joins a repo-relative path against root and returns the absolute
// path if it exists, or an error otherwise. Used by the detail view to locate
// source files referenced by Comment.Path.
func FileInRepo(root, relPath string) (string, error) {
	if root == "" {
		return "", errors.New("empty repo root")
	}
	abs := filepath.Join(root, relPath)
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	return abs, nil
}
