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

// PRDir returns ~/.revu/{owner}/{repo}/pr-{N}/.
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

// LatestPRDir returns the most recently modified pr-N directory under repoDir,
// where N is the largest number when multiple are tied on mtime.
func LatestPRDir(repoDir string) (string, error) {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return "", fmt.Errorf("read repo dir: %w", err)
	}
	type cand struct {
		name string
		num  int
	}
	var cands []cand
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
		cands = append(cands, cand{name: name, num: n})
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("no pr-* directories under %s", repoDir)
	}
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].num > cands[j].num
	})
	return filepath.Join(repoDir, cands[0].name), nil
}

// ResolveReviewDir is the entry point used by `revu open [arg]`.
//
// If arg is non-empty, it is treated as a filesystem path (absolute or relative
// to cwd). Otherwise, the current repository's git origin is read and the
// latest pr-* directory under ~/.revu/{owner}/{repo}/ is returned.
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
