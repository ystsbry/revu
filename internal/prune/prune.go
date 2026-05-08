// Package prune sweeps closed/merged PRs' review directories under
// ~/.revu/{owner}/{repo}/. The user-facing entry point is `revu prune`.
//
// The package is split into Plan (read-only classification of pr-N dirs as
// delete/keep/error) and Execute (actually rm -rf the chosen entries) so
// tests can assert on the plan without touching the filesystem.
package prune

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/store"
)

// StateQuerier abstracts the gh CLI call so tests can substitute fakes
// without depending on the broader github.Client interface.
type StateQuerier interface {
	PRState(ctx context.Context, slug string, number int) (string, error)
}

// Entry is one row of the plan.
type Entry struct {
	Number          int    // PR number
	State           string // OPEN/CLOSED/MERGED, or "" when querying failed
	Path            string // absolute pr-N directory
	HasUnsubmitted  bool   // any {sha}/review.yml under this PR lacks submitted_at
	SHADirCount     int    // number of {sha}/review.yml dirs under pr-N/
	QueryErr        error  // non-nil when PRState failed
}

// Plan classifies pr-N directories under repoDir into delete / keep / error
// buckets. It does not modify the filesystem.
type Plan struct {
	RepoDir string
	Slug    string
	Delete  []Entry // CLOSED or MERGED on GitHub
	Keep    []Entry // OPEN
	Errored []Entry // PRState query failed; left untouched for safety
}

// BuildOptions adjusts how Plan is computed.
type BuildOptions struct {
	// Slug is the "owner/repo" string passed to PRState.
	Slug string
	// RepoDir is ~/.revu/{owner}/{repo}/. Required.
	RepoDir string
}

// Build inspects RepoDir, queries each pr-N's GitHub state via querier, and
// returns a populated Plan. A pr-N whose query fails goes into Errored
// (never deleted).
func Build(ctx context.Context, querier StateQuerier, opts BuildOptions) (*Plan, error) {
	if opts.RepoDir == "" {
		return nil, errors.New("RepoDir is required")
	}
	if opts.Slug == "" {
		return nil, errors.New("Slug is required")
	}
	if querier == nil {
		return nil, errors.New("querier is required")
	}

	if _, err := os.Stat(opts.RepoDir); err != nil {
		if os.IsNotExist(err) {
			return &Plan{RepoDir: opts.RepoDir, Slug: opts.Slug}, nil
		}
		return nil, fmt.Errorf("stat %s: %w", opts.RepoDir, err)
	}

	numbers, err := store.ListPRNumbers(opts.RepoDir)
	if err != nil {
		return nil, err
	}

	plan := &Plan{RepoDir: opts.RepoDir, Slug: opts.Slug}
	for _, n := range numbers {
		entry := classify(ctx, querier, opts.Slug, opts.RepoDir, n)
		switch {
		case entry.QueryErr != nil:
			plan.Errored = append(plan.Errored, entry)
		case entry.State == github.PRStateClosed || entry.State == github.PRStateMerged:
			plan.Delete = append(plan.Delete, entry)
		default:
			plan.Keep = append(plan.Keep, entry)
		}
	}
	return plan, nil
}

func classify(ctx context.Context, querier StateQuerier, slug, repoDir string, n int) Entry {
	prDir := filepath.Join(repoDir, fmt.Sprintf("pr-%d", n))
	entry := Entry{Number: n, Path: prDir}

	state, err := querier.PRState(ctx, slug, n)
	if err != nil {
		entry.QueryErr = err
		return entry
	}
	entry.State = state

	// Inspect SHA subdirs to surface "has unsubmitted reviews" warnings.
	subs, err := os.ReadDir(prDir)
	if err != nil {
		return entry
	}
	for _, s := range subs {
		if !s.IsDir() {
			continue
		}
		shaDir := filepath.Join(prDir, s.Name())
		if _, err := os.Stat(filepath.Join(shaDir, "review.yml")); err != nil {
			continue
		}
		entry.SHADirCount++
		submitted, perr := store.PeekSubmittedAt(shaDir)
		if perr == nil && !submitted {
			entry.HasUnsubmitted = true
		}
	}
	return entry
}

// Execute deletes each pr-N directory listed in plan.Delete via os.RemoveAll.
// A nil plan is a no-op. Errors deleting individual entries are aggregated
// into the returned error so callers see a complete report.
func Execute(plan *Plan) error {
	if plan == nil || len(plan.Delete) == 0 {
		return nil
	}
	var errs []error
	for _, e := range plan.Delete {
		if err := os.RemoveAll(e.Path); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", e.Path, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
