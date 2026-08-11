package dashboard

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/jobs"
	"github.com/ystsbry/revu/internal/store"
)

// RepoItem is one row of the repository sidebar.
type RepoItem struct {
	Slug          string // "owner/repo"
	Path          string // registered clone path ("" when unregistered)
	Search        string // per-repo PR search override ("" = default)
	ReviewedCount int    // reviewed PRs under ~/.revu
	Registered    bool   // false for store-only fallback rows
	PathMissing   bool   // registered but the clone is absent on disk
}

// repoListData is what the repository loader produces in one shot.
type repoListData struct {
	// Profile is the active profile name ("" when showing everything).
	Profile string
	// Warnings surface non-fatal registry/profile inconsistencies
	// (undeclared active profile, profile slugs that are unregistered).
	Warnings []string
	Items    []RepoItem
}

// loadReposFromConfig resolves the registry through the active profile and
// annotates each repo with its local review count. With an empty registry
// it falls back to the repos that have reviews under ~/.revu.
func loadReposFromConfig() (repoListData, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return repoListData{}, err
	}

	var data repoListData
	repos, missing, unknown := cfg.ActiveRepos()
	if unknown != "" {
		data.Warnings = append(data.Warnings,
			fmt.Sprintf("active profile %q is not declared; showing all repos", unknown))
	} else if cfg.ActiveProfile != "" && cfg.ActiveProfile != config.DefaultProfileName {
		data.Profile = cfg.ActiveProfile
	}
	for _, slug := range missing {
		data.Warnings = append(data.Warnings,
			fmt.Sprintf("profile references unregistered repo %s", slug))
	}

	for _, r := range repos {
		it := RepoItem{Slug: r.Slug, Path: r.Path, Search: r.Search, Registered: true}
		if _, statErr := os.Stat(r.Path); statErr != nil {
			it.PathMissing = true
		}
		it.ReviewedCount = reviewedCount(r.Slug)
		data.Items = append(data.Items, it)
	}

	if len(cfg.Repos) == 0 {
		reviewed, err := store.ListReviewedRepos()
		if err != nil {
			return repoListData{}, err
		}
		for _, r := range reviewed {
			data.Items = append(data.Items, RepoItem{
				Slug:          r.Slug,
				ReviewedCount: r.PRCount,
			})
		}
	}
	return data, nil
}

// reviewedCount is how many reviewed PR dirs exist for slug. Best-effort:
// lookup failures render as zero rather than failing the list.
func reviewedCount(slug string) int {
	repoDir, err := store.RepoDir(slug)
	if err != nil {
		return 0
	}
	dirs, err := store.ListReviewedPRDirs(repoDir)
	if err != nil {
		return 0
	}
	return len(dirs)
}

// PRItem is one card of the PR list: an open PR fetched from GitHub,
// annotated with the local review state.
type PRItem struct {
	Number    int
	Title     string
	Author    string
	UpdatedAt string // as reported by gh (RFC3339); "" when unknown

	// ReviewedPath is the latest local review dir for this PR under
	// ~/.revu, or "" when it has not been reviewed locally.
	ReviewedPath string
	// Submitted is true when that local review records a submitted_at.
	Submitted bool
	// JobState marks a background review job on this PR: "running" or
	// "failed" (crash-aware, see jobs.Effective). Empty otherwise —
	// including for done jobs, whose outcome already shows as [reviewed].
	JobState string
}

// loadPRsFromGitHub lists slug's open PRs matching search via gh, then
// overlays the local review state from ~/.revu.
func loadPRsFromGitHub(slug, search string) ([]PRItem, error) {
	if search == "" {
		search = github.DefaultPRSearch
	}
	prs, err := github.New().ListPRs(context.Background(), slug, search)
	if err != nil {
		return nil, err
	}

	// Local review overlay, best-effort: a store hiccup must not hide the
	// PR list itself.
	reviewed := map[int]string{}
	if repoDir, err := store.RepoDir(slug); err == nil {
		if dirs, err := store.ListReviewedPRDirs(repoDir); err == nil {
			for _, d := range dirs {
				reviewed[d.Number] = d.Path
			}
		}
	}

	items := make([]PRItem, 0, len(prs))
	for _, pr := range prs {
		it := PRItem{
			Number:    pr.Number,
			Title:     pr.Title,
			Author:    pr.Author.Login,
			UpdatedAt: pr.UpdatedAt,
		}
		if path, ok := reviewed[pr.Number]; ok {
			it.ReviewedPath = path
			if submitted, err := store.PeekSubmittedAt(path); err == nil {
				it.Submitted = submitted
			}
		}
		it.JobState = jobBadgeState(slug, pr.Number, time.Now())
		items = append(items, it)
	}
	return items, nil
}

// jobBadgeState is the badge for slug#pr's newest background job:
// "running" and "failed" (crash-aware) surface; a done job stays silent
// because its outcome is the review itself, already badged [reviewed].
func jobBadgeState(slug string, pr int, now time.Time) string {
	j, ok := jobs.LatestForPR(slug, pr)
	if !ok {
		return ""
	}
	st, _ := j.Effective(now)
	if st == jobs.StateDone {
		return ""
	}
	return string(st)
}

// badges renders the PR's state badges ([reviewed] [submitted] [running]...).
func (it PRItem) badges() string {
	out := ""
	if it.ReviewedPath != "" {
		out += "  [reviewed]"
	}
	if it.Submitted {
		out += "  [submitted]"
	}
	if it.JobState != "" {
		out += "  [" + it.JobState + "]"
	}
	return out
}

// faintBox renders body text in the dimmed, padded style non-list states
// (loading, error, empty) share.
func faintBox(s string) string {
	return lipgloss.NewStyle().Faint(true).Padding(1, 1).Render(s)
}

// truncate shortens s to at most n runes (min 10), appending an ellipsis.
// A non-positive n means the screen has no size yet; the text is returned
// whole rather than guessed at.
func truncate(s string, n int) string {
	if n <= 0 {
		return s
	}
	if n < 10 {
		n = 10
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// truncateCells shortens s to at most n terminal cells (display width),
// appending an ellipsis. Unlike truncate, this counts fullwidth (CJK)
// runes as two cells, so a shortened string never wraps inside a
// fixed-width card. Non-positive n returns s whole.
func truncateCells(s string, n int) string {
	if n <= 0 {
		return s
	}
	if n < 10 {
		n = 10
	}
	if runewidth.StringWidth(s) <= n {
		return s
	}
	return runewidth.Truncate(s, n, "…")
}

// cursorRow renders one selectable line, bolding and marking the cursor.
func cursorRow(label string, selected bool) string {
	if selected {
		return lipgloss.NewStyle().Bold(true).Render("▸ " + label)
	}
	return "  " + label
}
