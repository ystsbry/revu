package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PRListItem is a single entry in the picker. Mirrors the JSON shape we
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

// ListReviewRequestedPRs returns open PRs in the cwd's repo where the
// current gh user is a requested reviewer. Empty result and no error when
// there are no such PRs.
func (c *GhClient) ListReviewRequestedPRs(ctx context.Context) ([]PRListItem, error) {
	cmd := exec.CommandContext(ctx, c.bin(),
		"pr", "list",
		"--state", "open",
		"--search", "review-requested:@me",
		"--json", "number,title,url,baseRefName,headRefName,author,updatedAt",
		"--limit", "50",
	)
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
