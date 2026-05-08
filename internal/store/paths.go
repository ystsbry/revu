package store

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Home returns the revu home directory.
// Defaults to ~/.revu; the REVU_HOME env var overrides this (used in tests).
func Home() (string, error) {
	if v := os.Getenv("REVU_HOME"); v != "" {
		return v, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(h, ".revu"), nil
}

// RepoDir returns ~/.revu/{owner}/{repo}/ for a slug like "owner/repo".
func RepoDir(slug string) (string, error) {
	owner, repo, err := splitSlug(slug)
	if err != nil {
		return "", err
	}
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, owner, repo), nil
}

// PRDir returns ~/.revu/{owner}/{repo}/pr-{N}/, the parent directory that
// holds one or more SHA-suffixed review dirs.
func PRDir(slug string, pr int) (string, error) {
	if pr <= 0 {
		return "", fmt.Errorf("pr number must be positive, got %d", pr)
	}
	r, err := RepoDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(r, fmt.Sprintf("pr-%d", pr)), nil
}

// shortSHALen is the number of leading hex characters used as the per-SHA
// directory name under pr-{N}/. Keep this in one place so the writer
// (cmd_pr.go) and the discovery logic (ListReviewedPRDirs) agree.
const shortSHALen = 7

// ShortSHA returns the leading shortSHALen hex characters of sha.
// It does not validate that sha is a valid hex string; it only enforces
// that sha is long enough to slice.
func ShortSHA(sha string) (string, error) {
	if len(sha) < shortSHALen {
		return "", fmt.Errorf("sha %q is too short (need at least %d chars)", sha, shortSHALen)
	}
	return sha[:shortSHALen], nil
}

// ReviewDir returns ~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/, the directory
// where review.yml + summary.md + comments/ live for one specific commit.
func ReviewDir(slug string, pr int, sha string) (string, error) {
	parent, err := PRDir(slug, pr)
	if err != nil {
		return "", err
	}
	short, err := ShortSHA(sha)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, short), nil
}

func splitSlug(slug string) (string, string, error) {
	parts := strings.Split(strings.Trim(slug, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo slug %q (want owner/repo)", slug)
	}
	return parts[0], parts[1], nil
}

// remoteURLPattern matches the owner/repo trailing segment of a git remote URL,
// optionally followed by ".git". Anchored to end-of-string.
var remoteURLPattern = regexp.MustCompile(`(?:[/:])([A-Za-z0-9_.\-]+)/([A-Za-z0-9_.\-]+?)(?:\.git)?/?$`)

// ParseRemoteURL extracts an "owner/repo" slug from a git remote URL.
// Supports SSH (git@host:owner/repo.git), HTTPS (https://host/owner/repo.git),
// and ssh:// (ssh://git@host/owner/repo.git) styles.
func ParseRemoteURL(url string) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", errors.New("empty remote url")
	}
	m := remoteURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", fmt.Errorf("could not parse remote url %q", url)
	}
	return m[1] + "/" + m[2], nil
}

// CurrentRepoSlug runs `git config --get remote.origin.url` in cwd
// and parses out the owner/repo slug.
func CurrentRepoSlug() (string, error) {
	out, err := exec.Command("git", "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return "", fmt.Errorf("git remote origin url: %w", err)
	}
	return ParseRemoteURL(string(out))
}

// ReviewedPRDir is one entry returned by ListReviewedPRDirs.
type ReviewedPRDir struct {
	Number   int    // pr number
	ShortSHA string // leading hex of the head_sha that produced this review
	Path     string // absolute path to the pr-N/{sha} directory
}

// ListReviewedPRDirs returns one entry per pr-N directory under repoDir that
// has at least one {sha}/review.yml beneath it. When a pr-N has multiple SHA
// subdirs, the one whose review.yml has the most recent ModTime wins. Results
// are sorted by N descending. Directories without any reviewed SHA subdir
// (e.g. an empty pr-N from an in-flight run, or a legacy pr-N/review.yml from
// before the SHA-suffix layout) are skipped.
func ListReviewedPRDirs(repoDir string) ([]ReviewedPRDir, error) {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return nil, fmt.Errorf("read repo dir: %w", err)
	}
	var out []ReviewedPRDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "pr-") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(name, "pr-"))
		if err != nil || n <= 0 {
			continue
		}
		picked, ok := latestReviewedSHADir(filepath.Join(repoDir, name))
		if !ok {
			continue
		}
		picked.Number = n
		out = append(out, picked)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Number > out[j].Number
	})
	return out, nil
}

// latestReviewedSHADir scans prDir for SHA subdirectories that contain a
// review.yml and returns the one with the most recent ModTime. The returned
// ReviewedPRDir has ShortSHA and Path populated; Number is left to the caller.
func latestReviewedSHADir(prDir string) (ReviewedPRDir, bool) {
	subs, err := os.ReadDir(prDir)
	if err != nil {
		return ReviewedPRDir{}, false
	}
	var (
		best     ReviewedPRDir
		bestMod  int64
		bestSet  bool
	)
	for _, s := range subs {
		if !s.IsDir() {
			continue
		}
		shaDir := filepath.Join(prDir, s.Name())
		st, err := os.Stat(filepath.Join(shaDir, "review.yml"))
		if err != nil {
			continue
		}
		mod := st.ModTime().UnixNano()
		if !bestSet || mod > bestMod {
			best = ReviewedPRDir{ShortSHA: s.Name(), Path: shaDir}
			bestMod = mod
			bestSet = true
		}
	}
	return best, bestSet
}

// LatestPRDir returns the SHA-suffixed review directory under repoDir whose
// PR number is the largest. When the chosen pr-N has multiple SHA subdirs,
// the one with the most recent review.yml ModTime is returned.
func LatestPRDir(repoDir string) (string, error) {
	dirs, err := ListReviewedPRDirs(repoDir)
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no reviewed pr-* directories under %s (no review.yml found)", repoDir)
	}
	return dirs[0].Path, nil
}

// ListPRNumbers returns every PR number whose pr-N directory exists under
// repoDir, regardless of whether it contains a review.yml. Used by `revu
// prune` to enumerate candidates for deletion. Returned in ascending order.
func ListPRNumbers(repoDir string) ([]int, error) {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return nil, fmt.Errorf("read repo dir: %w", err)
	}
	var out []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "pr-") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(name, "pr-"))
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

// LatestReviewDirForPR returns the most recently written {sha}/review.yml dir
// under ~/.revu/{owner}/{repo}/pr-{pr}/. Useful right after a review run when
// the caller knows the PR number but not the head_sha used to write the
// review (e.g. the skill subprocess picked it up via `revu pr prepare`).
func LatestReviewDirForPR(slug string, pr int) (string, error) {
	parent, err := PRDir(slug, pr)
	if err != nil {
		return "", err
	}
	picked, ok := latestReviewedSHADir(parent)
	if !ok {
		return "", fmt.Errorf("no {sha}/review.yml under %s", parent)
	}
	return picked.Path, nil
}

// ResolveReviewDir is the entry point used by `revu open [arg]`.
//
// If arg is non-empty, it is treated as a filesystem path (absolute or relative
// to cwd). Otherwise, the current repository's git origin is read and the
// latest reviewed pr-* directory (one that contains review.yml) under
// ~/.revu/{owner}/{repo}/ is returned.
func ResolveReviewDir(arg string) (string, error) {
	if arg != "" {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("review dir %s: %w", abs, err)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("review path %s is not a directory", abs)
		}
		return abs, nil
	}

	slug, err := CurrentRepoSlug()
	if err != nil {
		return "", fmt.Errorf("auto-resolve review dir: %w", err)
	}
	repoDir, err := RepoDir(slug)
	if err != nil {
		return "", err
	}
	return LatestPRDir(repoDir)
}
