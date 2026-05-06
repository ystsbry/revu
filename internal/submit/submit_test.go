package submit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/model"
)

func sampleReview() *model.Review {
	return &model.Review{
		SchemaVersion: 1,
		PR:            model.PRMeta{Repo: "o/r", Number: 7, HeadSHA: "abc1234"},
		ReviewEvent:   model.EventComment,
		SummaryFile:   "summary.md",
		SummaryBody:   "the summary",
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusAccepted, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "a.go", Line: 1, Side: model.SideRight, Body: "fix"},
			{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "b.go", Line: 2, Side: model.SideRight, Body: "?"},
		},
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 3, 14, 30, 0, 0, time.UTC)
}

func TestSubmitDryRun(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	var out bytes.Buffer
	saved := false

	err := Run(context.Background(), Options{
		Review: r,
		Saver:  func(*model.Review) error { saved = true; return nil },
		Out:    &out,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if saved {
		t.Errorf("saver should not be called on dry-run")
	}
	if !strings.Contains(out.String(), "dry-run: not submitted") {
		t.Errorf("dry-run marker missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Inline comments: 1 accepted (0 rejected, 1 pending") {
		t.Errorf("counts missing:\n%s", out.String())
	}
}

func TestSubmitHappyPath(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	var out bytes.Buffer
	in := strings.NewReader("submit\n")

	var saved *model.Review
	saver := func(rr *model.Review) error { saved = rr; return nil }

	headCalls := 0
	postCalls := 0
	client := &github.FakeClient{
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) {
			headCalls++
			return "abc1234", nil
		},
		PostReviewFunc: func(ctx context.Context, slug string, number int, p github.Payload) (int64, error) {
			postCalls++
			if slug != "o/r" || number != 7 {
				t.Errorf("PostReview called with %s#%d", slug, number)
			}
			if p.CommitID != "abc1234" {
				t.Errorf("payload commit_id = %q", p.CommitID)
			}
			return 999, nil
		},
	}

	err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Saver:  saver,
		Now:    fixedNow,
		Out:    &out,
		In:     in,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if headCalls != 1 || postCalls != 1 {
		t.Errorf("call counts: head=%d post=%d", headCalls, postCalls)
	}
	if saved == nil {
		t.Fatal("saver not invoked")
	}
	if saved.SubmittedAt == nil || !saved.SubmittedAt.Equal(fixedNow()) {
		t.Errorf("SubmittedAt = %v", saved.SubmittedAt)
	}
	if saved.ReviewID == nil || *saved.ReviewID != 999 {
		t.Errorf("ReviewID = %v", saved.ReviewID)
	}
	if !strings.Contains(out.String(), "review_id=999") {
		t.Errorf("success message missing:\n%s", out.String())
	}
}

func TestSubmitYesSkipsPrompt(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	var out bytes.Buffer

	postCalled := false
	client := &github.FakeClient{
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) { return "abc1234", nil },
		PostReviewFunc: func(ctx context.Context, slug string, number int, p github.Payload) (int64, error) {
			postCalled = true
			return 42, nil
		},
	}

	err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Saver:  func(*model.Review) error { return nil },
		Now:    fixedNow,
		Out:    &out,
		// In is intentionally nil — Yes should make it unnecessary.
		Yes: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !postCalled {
		t.Errorf("PostReview must be called when Yes=true")
	}
	if strings.Contains(out.String(), "Type 'submit'") {
		t.Errorf("confirmation prompt must not appear when Yes=true:\n%s", out.String())
	}
}

func TestSubmitCancelledByPrompt(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	in := strings.NewReader("no thanks\n")
	var out bytes.Buffer

	postCalled := false
	client := &github.FakeClient{
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) { return "abc1234", nil },
		PostReviewFunc: func(ctx context.Context, slug string, number int, p github.Payload) (int64, error) {
			postCalled = true
			return 0, nil
		},
	}

	err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Saver:  func(*model.Review) error { return nil },
		Now:    fixedNow,
		Out:    &out,
		In:     in,
	})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if postCalled {
		t.Errorf("PostReview must not be called on cancel")
	}
}

func TestSubmitHeadShaMismatch(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	var out bytes.Buffer

	client := &github.FakeClient{
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) {
			return "deadbee", nil // PR has advanced
		},
		PostReviewFunc: func(ctx context.Context, slug string, number int, p github.Payload) (int64, error) {
			t.Fatal("PostReview should not be called on mismatch")
			return 0, nil
		},
	}

	err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Saver:  func(*model.Review) error { return nil },
		Now:    fixedNow,
		Out:    &out,
		In:     strings.NewReader("submit\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "head_sha mismatch") {
		t.Fatalf("err = %v, want head_sha mismatch", err)
	}
}

func TestSubmitAuthFailureAborts(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	var out bytes.Buffer
	client := &github.FakeClient{
		AuthStatusFunc: func(ctx context.Context) error { return errors.New("not logged in") },
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) {
			t.Fatal("PRHead should not be called on auth failure")
			return "", nil
		},
	}
	err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Saver:  func(*model.Review) error { return nil },
		Now:    fixedNow,
		Out:    &out,
		In:     strings.NewReader("submit\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("err = %v, want auth failure", err)
	}
}

func TestSubmitAlreadySubmittedRefuses(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	prev := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	id := int64(123)
	r.SubmittedAt = &prev
	r.ReviewID = &id

	var out bytes.Buffer
	client := &github.FakeClient{
		PostReviewFunc: func(ctx context.Context, slug string, number int, p github.Payload) (int64, error) {
			t.Fatal("PostReview must not be called when already submitted")
			return 0, nil
		},
	}
	err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Saver:  func(*model.Review) error { return nil },
		Now:    fixedNow,
		Out:    &out,
		In:     strings.NewReader("submit\n"),
	})
	if !errors.Is(err, ErrAlreadySubmitted) {
		t.Fatalf("err = %v, want ErrAlreadySubmitted", err)
	}
}

func TestSubmitAcceptPendingPromotesAndPosts(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	// sampleReview has c1=accepted, c2=pending, plus a rejected one in some tests.
	// Add an explicit rejected comment to confirm AcceptPending leaves it alone.
	r.Comments = append(r.Comments, model.Comment{
		ID: "c3", Status: model.StatusRejected, Severity: model.SeverityNit, Category: model.CategoryStyle,
		Path: "c.go", Line: 3, Side: model.SideRight, Body: "no",
	})

	var out bytes.Buffer
	var posted github.Payload
	client := &github.FakeClient{
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) { return "abc1234", nil },
		PostReviewFunc: func(ctx context.Context, slug string, number int, p github.Payload) (int64, error) {
			posted = p
			return 7, nil
		},
	}

	var saved *model.Review
	err := Run(context.Background(), Options{
		Review:        r,
		Client:        client,
		Saver:         func(rr *model.Review) error { saved = rr; return nil },
		Now:           fixedNow,
		Out:           &out,
		Yes:           true,
		AcceptPending: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(posted.Comments) != 2 {
		t.Errorf("posted comments = %d, want 2 (c1 already accepted + c2 promoted from pending)", len(posted.Comments))
	}
	if !strings.Contains(out.String(), "Inline comments: 2 accepted (1 rejected, 0 pending") {
		t.Errorf("counts not promoted in preview:\n%s", out.String())
	}
	if saved == nil {
		t.Fatal("saver not invoked")
	}
	// Persisted statuses should reflect the promotion so the YAML on disk is honest.
	if saved.Comments[1].Status != model.StatusAccepted {
		t.Errorf("c2 status after submit = %q, want accepted", saved.Comments[1].Status)
	}
	if saved.Comments[2].Status != model.StatusRejected {
		t.Errorf("c3 status after submit = %q, want rejected (AcceptPending must not touch rejected)", saved.Comments[2].Status)
	}
}

func TestSubmitDryRunDoesNotCheckHead(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	var out bytes.Buffer
	client := &github.FakeClient{
		PRHeadFunc: func(ctx context.Context, slug string, number int) (string, error) {
			t.Fatal("PRHead must not be called on dry-run")
			return "", nil
		},
	}
	if err := Run(context.Background(), Options{
		Review: r,
		Client: client,
		Out:    &out,
		DryRun: true,
	}); err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
}
