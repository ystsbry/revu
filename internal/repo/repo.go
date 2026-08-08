// Package repo discovers local git clones and maps them to GitHub
// "owner/repo" slugs, feeding the [[repo]] registry in the user config.
//
// Discovery shells out to git rather than reading .git internals, so
// worktrees and submodule layouts (where .git is a file, not a directory)
// resolve the same way git itself would.
package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/store"
)

// DefaultScanDepth bounds how deep Scan walks below the root. A ghq root
// is host/owner/repo = 3 levels; one extra level covers roots that nest an
// intermediate directory without letting a stray scan of $HOME walk the
// whole disk.
const DefaultScanDepth = 4

// Skipped is a directory Scan recognised as a git repo but could not turn
// into a registry entry.
type Skipped struct {
	Path   string
	Reason string
}

// ScanResult is everything a scan found below the root.
type ScanResult struct {
	Found   []config.RepoDef
	Skipped []Skipped
}

// DetectSlug resolves dir's origin remote to an "owner/repo" slug.
func DetectSlug(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return "", fmt.Errorf("%s: no remote.origin.url (is it a git repo with an origin?)", dir)
	}
	url := strings.TrimSpace(string(out))
	slug, err := store.ParseRemoteURL(url)
	if err != nil {
		return "", fmt.Errorf("%s: %w", dir, err)
	}
	return slug, nil
}

// Scan walks root up to maxDepth levels deep and returns every git clone
// (a directory containing .git) with a resolvable origin slug. It does not
// descend into clones, so nested repos (vendored checkouts, submodule work
// trees) below a found clone are not reported. Non-git directories are
// silently traversed; git dirs without a usable origin are reported in
// Skipped. maxDepth <= 0 means DefaultScanDepth.
func Scan(root string, maxDepth int) (ScanResult, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultScanDepth
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ScanResult{}, err
	}
	if st, err := os.Stat(absRoot); err != nil {
		return ScanResult{}, err
	} else if !st.IsDir() {
		return ScanResult{}, fmt.Errorf("%s is not a directory", absRoot)
	}

	var res ScanResult
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Unreadable directories are skipped, not fatal: a ghq root
			// may contain trees owned by other tools.
			if errors.Is(walkErr, fs.ErrPermission) {
				return fs.SkipDir
			}
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if name := d.Name(); name != "." && strings.HasPrefix(name, ".") && path != absRoot {
			// .git itself, plus dot-dirs like .cache — never repos roots.
			return fs.SkipDir
		}
		depth := pathDepth(absRoot, path)
		if depth > maxDepth {
			return fs.SkipDir
		}
		if _, statErr := os.Lstat(filepath.Join(path, ".git")); statErr != nil {
			return nil // not a clone; keep walking
		}

		slug, detectErr := DetectSlug(path)
		if detectErr != nil {
			res.Skipped = append(res.Skipped, Skipped{Path: path, Reason: detectErr.Error()})
		} else {
			res.Found = append(res.Found, config.RepoDef{Slug: slug, Path: path})
		}
		return fs.SkipDir // never descend into a clone
	})
	if err != nil {
		return ScanResult{}, err
	}

	// Stable sort so entries sharing a slug keep walk order (WalkDir is
	// lexical), making repeated scans deterministic.
	sort.SliceStable(res.Found, func(i, j int) bool { return res.Found[i].Slug < res.Found[j].Slug })
	// Dedupe by slug: two clones of the same repo below one root (e.g. a
	// fork checkout plus the ghq path) keep the first-walked path.
	res.Found = dedupeBySlug(res.Found)
	return res, nil
}

func dedupeBySlug(defs []config.RepoDef) []config.RepoDef {
	seen := make(map[string]struct{}, len(defs))
	out := defs[:0]
	for _, d := range defs {
		if _, dup := seen[d.Slug]; dup {
			continue
		}
		seen[d.Slug] = struct{}{}
		out = append(out, d)
	}
	return out
}

func pathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}
