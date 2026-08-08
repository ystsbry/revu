package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultPRSearch is the search applied when a repository has no per-repo
// override ([[repo]].search in config): the PRs waiting on the current gh
// user's review.
const DefaultPRSearch = "review-requested:@me"

// PRListItem is a single entry in a PR list. Mirrors the JSON shape we
// request from `gh pr list`.
type PRListItem struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	UpdatedAt string `json:"updatedAt"`
}

// prListArgs builds the argv (after the binary) for one `gh pr list`
// invocation. Extracted for tests: the --repo/--search wiring is the whole
// point of ListPRs and must not silently drop out.
func prListArgs(slug, search string) []string {
	args := []string{"pr", "list", "--state", "open"}
	if slug != "" {
		args = append(args, "--repo", slug)
	}
	if search != "" {
		args = append(args, "--search", search)
	}
	return append(args,
		"--json", "number,title,url,baseRefName,headRefName,author,updatedAt",
		"--limit", "50",
	)
}

// ListPRs returns open PRs of slug matching search. An empty slug targets
// the cwd's repo (gh's default); an empty search lists every open PR.
// Empty result and no error when nothing matches.
func (c *GhClient) ListPRs(ctx context.Context, slug, search string) ([]PRListItem, error) {
	cmd := exec.CommandContext(ctx, c.bin(), prListArgs(slug, search)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh pr list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var items []PRListItem
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	return items, nil
}

// ListReviewRequestedPRs returns open PRs in the cwd's repo where the
// current gh user is a requested reviewer. Kept as a thin alias so existing
// callers (revu pr list-mine, the review picker) read as before.
func (c *GhClient) ListReviewRequestedPRs(ctx context.Context) ([]PRListItem, error) {
	return c.ListPRs(ctx, "", DefaultPRSearch)
}
