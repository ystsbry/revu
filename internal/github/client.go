// Package github wraps a small subset of the gh CLI for revu's submission flow.
//
// We shell out to gh rather than using a native HTTP client so revu does not
// have to handle authentication tokens itself; gh already manages those.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

// Client is the abstraction the rest of revu talks to. Tests inject a fake.
type Client interface {
	AuthStatus(ctx context.Context) error
	PRHead(ctx context.Context, slug string, number int) (string, error)
	PostReview(ctx context.Context, slug string, number int, p Payload) (int64, error)
	ListReviewRequestedPRs(ctx context.Context) ([]PRListItem, error)
	PRMeta(ctx context.Context, number int) (PRMeta, error)
	PRDiff(ctx context.Context, number int) (string, error)
}

// PRMeta is the subset of gh pr view JSON the review-pr skill consumes.
// BaseRepo is the "owner/repo" slug of the PR's base repository, derived from
// the PR url because gh pr view does not expose a baseRepository field.
type PRMeta struct {
	Number     int
	HeadSha    string
	BaseBranch string
	Title      string
	Body       string
	BaseRepo   string
}

// GhClient invokes the gh CLI as a subprocess.
type GhClient struct {
	// Bin is the path to the gh executable. Empty means "look up gh on PATH".
	Bin string
}

// New returns a GhClient that uses the gh executable on PATH.
func New() *GhClient { return &GhClient{} }

func (c *GhClient) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "gh"
}

// AuthStatus runs `gh auth status` and returns nil if authenticated.
func (c *GhClient) AuthStatus(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.bin(), "auth", "status")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stderr.String())
		if out == "" {
			return fmt.Errorf("gh auth status: %w", err)
		}
		return fmt.Errorf("gh auth status: %s", out)
	}
	return nil
}

// PRHead returns the head_sha (headRefOid) of the PR.
func (c *GhClient) PRHead(ctx context.Context, slug string, number int) (string, error) {
	cmd := exec.CommandContext(ctx, c.bin(),
		"pr", "view", strconv.Itoa(number),
		"--repo", slug,
		"--json", "headRefOid",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh pr view: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var resp struct {
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("parse gh pr view output: %w", err)
	}
	if resp.HeadRefOid == "" {
		return "", errors.New("gh pr view returned empty headRefOid")
	}
	return resp.HeadRefOid, nil
}

// PRMeta returns the metadata fields the review-pr skill needs (head_sha,
// base_branch, title, body, base repo slug) in one gh call. Defaults to
// cwd's repo, same as `gh pr view <N>`.
func (c *GhClient) PRMeta(ctx context.Context, number int) (PRMeta, error) {
	cmd := exec.CommandContext(ctx, c.bin(),
		"pr", "view", strconv.Itoa(number),
		"--json", "number,headRefOid,baseRefName,title,body,url",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return PRMeta{}, fmt.Errorf("gh pr view: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var raw struct {
		Number     int    `json:"number"`
		HeadSha    string `json:"headRefOid"`
		BaseBranch string `json:"baseRefName"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		URL        string `json:"url"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return PRMeta{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	if raw.HeadSha == "" {
		return PRMeta{}, errors.New("gh pr view returned empty headRefOid")
	}
	slug, err := slugFromPRURL(raw.URL)
	if err != nil {
		return PRMeta{}, err
	}
	return PRMeta{
		Number:     raw.Number,
		HeadSha:    raw.HeadSha,
		BaseBranch: raw.BaseBranch,
		Title:      raw.Title,
		Body:       raw.Body,
		BaseRepo:   slug,
	}, nil
}

// slugFromPRURL extracts the "owner/repo" slug from a GitHub PR URL like
// https://github.com/owner/repo/pull/123. gh pr view does not expose a
// baseRepository field, so we derive the base slug from the PR url, which
// always points at the base repository (correct even for fork PRs).
func slugFromPRURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("gh pr view returned empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse PR url %q: %w", raw, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", fmt.Errorf("unexpected PR url path: %q", u.Path)
	}
	return parts[0] + "/" + parts[1], nil
}

// PRDiff returns the unified diff of the PR. Defaults to cwd's repo.
func (c *GhClient) PRDiff(ctx context.Context, number int) (string, error) {
	cmd := exec.CommandContext(ctx, c.bin(), "pr", "diff", strconv.Itoa(number))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh pr diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// PostReview submits the review to /repos/{slug}/pulls/{number}/reviews via
// `gh api`. Returns the GitHub-side review ID on success.
func (c *GhClient) PostReview(ctx context.Context, slug string, number int, p Payload) (int64, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}
	endpoint := fmt.Sprintf("/repos/%s/pulls/%d/reviews", slug, number)

	cmd := exec.CommandContext(ctx, c.bin(),
		"api", "-X", "POST", endpoint,
		"--input", "-",
	)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// gh writes the short "gh: ... (HTTP N)" line to stderr but puts the
		// full GitHub response body (where errors[].message lives) on stdout.
		body := strings.TrimSpace(stdout.String())
		if body == "" {
			return 0, fmt.Errorf("gh api POST %s: %w: %s", endpoint, err, strings.TrimSpace(stderr.String()))
		}
		return 0, fmt.Errorf("gh api POST %s: %w: %s: body=%s", endpoint, err, strings.TrimSpace(stderr.String()), body)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return 0, fmt.Errorf("parse review response: %w", err)
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("response has no review id; raw: %s", stdout.String())
	}
	return resp.ID, nil
}
