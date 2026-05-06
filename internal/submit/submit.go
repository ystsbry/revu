// Package submit drives the "post a review to GitHub" flow.
//
// It wires together payload construction, gh-CLI calls, a typed-confirmation
// prompt, and updating the review.yml on success. The flow is the same
// whether invoked from the CLI (`revu submit`) or the TUI (`:submit`).
package submit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/model"
)

// ErrCancelled is returned when the user does not type 'submit' at the prompt.
var ErrCancelled = errors.New("submit cancelled by user")

// ErrAlreadySubmitted is returned when the review.yml already records a
// submission. revu refuses to resubmit silently.
var ErrAlreadySubmitted = errors.New("review already submitted")

// Options carries everything Run needs. All fields are required except
// DryRun and Yes (both default to false).
type Options struct {
	Review *model.Review
	Client github.Client
	Saver  func(*model.Review) error
	Now    func() time.Time
	Out    io.Writer
	In     io.Reader
	DryRun bool
	// Yes skips the typed-confirmation prompt. Intended for non-interactive
	// callers such as CI; In is still required because the flow falls back
	// to the prompt when Yes is false.
	Yes bool
}

// Run executes the full submit flow:
//
//  1. Build the payload (validates & filters to accepted comments).
//  2. Refuse if review.yml already has submitted_at.
//  3. Confirm gh authentication (skipped in DryRun).
//  4. Fetch the PR's current head_sha and refuse on mismatch (skipped in DryRun).
//  5. Print a preview of what will be posted.
//  6. Require the user to type 'submit' (skipped in DryRun).
//  7. POST the review via gh CLI.
//  8. Record submitted_at + review_id in review.yml.
func Run(ctx context.Context, opts Options) error {
	if opts.Review == nil {
		return errors.New("nil review")
	}
	if !opts.DryRun {
		if opts.Client == nil {
			return errors.New("no github client configured")
		}
		if opts.Saver == nil {
			return errors.New("no saver configured")
		}
		if opts.Now == nil {
			opts.Now = time.Now
		}
		if !opts.Yes && opts.In == nil {
			return errors.New("no input reader configured")
		}
	}
	if opts.Out == nil {
		return errors.New("no output writer configured")
	}

	payload, counts, err := github.BuildPayload(opts.Review)
	if err != nil {
		return err
	}

	if opts.Review.SubmittedAt != nil && !opts.DryRun {
		return fmt.Errorf("%w: at %s (review_id=%d)",
			ErrAlreadySubmitted,
			opts.Review.SubmittedAt.Format(time.RFC3339),
			derefInt64(opts.Review.ReviewID),
		)
	}

	var currentHead string
	if !opts.DryRun {
		if err := opts.Client.AuthStatus(ctx); err != nil {
			return fmt.Errorf("gh authentication: %w", err)
		}
		currentHead, err = opts.Client.PRHead(ctx, opts.Review.PR.Repo, opts.Review.PR.Number)
		if err != nil {
			return fmt.Errorf("fetch PR head_sha: %w", err)
		}
		if currentHead != opts.Review.PR.HeadSHA {
			return fmt.Errorf("head_sha mismatch: review was generated for %s but PR is now at %s",
				shortSHA(opts.Review.PR.HeadSHA), shortSHA(currentHead))
		}
	}

	printPreview(opts.Out, opts.Review, payload, counts, currentHead)

	if opts.DryRun {
		fmt.Fprintln(opts.Out, "(dry-run: not submitted)")
		return nil
	}

	if !opts.Yes {
		if !confirmTyped(opts.In, opts.Out, "submit") {
			return ErrCancelled
		}
	}

	reviewID, err := opts.Client.PostReview(ctx, opts.Review.PR.Repo, opts.Review.PR.Number, payload)
	if err != nil {
		return fmt.Errorf("post review: %w", err)
	}

	now := opts.Now()
	opts.Review.SubmittedAt = &now
	opts.Review.ReviewID = &reviewID
	if err := opts.Saver(opts.Review); err != nil {
		return fmt.Errorf("review submitted (id=%d) but saving review.yml failed: %w", reviewID, err)
	}

	fmt.Fprintf(opts.Out, "\nSubmitted: review_id=%d\n", reviewID)
	return nil
}

// printPreview writes the human-readable summary used by both --dry-run and
// the pre-confirmation display in the interactive flow.
func printPreview(out io.Writer, r *model.Review, p github.Payload, counts github.FilteredCounts, currentHead string) {
	fmt.Fprintf(out, "About to submit review to %s#%d\n\n", r.PR.Repo, r.PR.Number)
	fmt.Fprintf(out, "  Event:           %s\n", p.Event)
	fmt.Fprintf(out, "  Summary:         %d chars from %s\n", len(p.Body), r.SummaryFile)
	fmt.Fprintf(out, "  Inline comments: %d accepted (%d rejected, %d pending will be skipped)\n",
		counts.Accepted, counts.Rejected, counts.Pending)
	fmt.Fprintln(out)
	for _, c := range r.Comments {
		if c.Status != model.StatusAccepted && c.Status != model.StatusEdited {
			continue
		}
		fmt.Fprintf(out, "  - %s %s:%d (%s / %s)\n", c.ID, c.Path, c.Line, c.Severity, c.Category)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Head SHA at generation: %s\n", shortSHA(r.PR.HeadSHA))
	if currentHead != "" {
		mark := "✓"
		if currentHead != r.PR.HeadSHA {
			mark = "✗"
		}
		fmt.Fprintf(out, "  Current head SHA:       %s  %s\n", shortSHA(currentHead), mark)
	}
	fmt.Fprintln(out)
}

// confirmTyped reads a line from in and returns true iff the user typed
// the expected word exactly (after trimming whitespace).
func confirmTyped(in io.Reader, out io.Writer, expected string) bool {
	fmt.Fprintf(out, "Type '%s' to confirm, or anything else to cancel:\n> ", expected)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	return strings.TrimSpace(scanner.Text()) == expected
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
