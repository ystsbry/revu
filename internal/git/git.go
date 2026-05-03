// Package git is a thin wrapper over the system git CLI for the read-only
// operations revu needs (currently: fetching pre-image file contents and
// resolving the merge-base for a review).
//
// We shell out to git rather than embedding go-git because revu already
// requires gh for posting reviews and assumes a working git CLI; pulling
// in a 200kLOC dependency just to read a blob is overkill.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Show returns the contents of `path` at the given `ref` from the repository
// at `repoRoot`. Equivalent to: `git -C repoRoot show ref:path`.
func Show(repoRoot, ref, path string) ([]byte, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("git.Show: repoRoot is empty")
	}
	if ref == "" {
		return nil, fmt.Errorf("git.Show: ref is empty")
	}
	if path == "" {
		return nil, fmt.Errorf("git.Show: path is empty")
	}
	cmd := exec.Command("git", "-C", repoRoot, "show", ref+":"+path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w: %s",
			ref, path, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Diff returns the unified diff for `path` between baseRef and headRef in
// the repository at repoRoot, with `ctx` lines of surrounding context per
// hunk. Equivalent to:
//
//	git -C repoRoot diff --no-color -U<ctx> baseRef..headRef -- path
//
// The returned bytes contain the standard "diff --git", "---", "+++",
// "@@ ... @@" headers followed by hunk lines.
func Diff(repoRoot, baseRef, headRef, path string, ctx int) ([]byte, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("git.Diff: repoRoot is empty")
	}
	if baseRef == "" || headRef == "" {
		return nil, fmt.Errorf("git.Diff: refs must be non-empty")
	}
	if path == "" {
		return nil, fmt.Errorf("git.Diff: path is empty")
	}
	if ctx < 0 {
		ctx = 0
	}
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--no-color",
		fmt.Sprintf("-U%d", ctx),
		baseRef+".."+headRef, "--", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git diff %s..%s -- %s: %w: %s",
			baseRef, headRef, path, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// MergeBase returns the merge-base commit SHA between two refs.
// Equivalent to: `git -C repoRoot merge-base a b`.
func MergeBase(repoRoot, a, b string) (string, error) {
	if repoRoot == "" {
		return "", fmt.Errorf("git.MergeBase: repoRoot is empty")
	}
	if a == "" || b == "" {
		return "", fmt.Errorf("git.MergeBase: refs must be non-empty")
	}
	cmd := exec.Command("git", "-C", repoRoot, "merge-base", a, b)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git merge-base %s %s: %w: %s",
			a, b, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
