package github

import (
	"context"
	"errors"
	"testing"
)

// FakeClient exists so other packages can inject a github client without a
// network or a gh binary. These tests pin the two properties callers rely
// on: it satisfies Client, and an unset Func field is a harmless zero value
// rather than a nil-pointer panic.

var _ Client = (*FakeClient)(nil)

func TestFakeClientZeroValueIsUsable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var f FakeClient

	if err := f.AuthStatus(ctx); err != nil {
		t.Errorf("AuthStatus = %v, want nil", err)
	}
	if got, err := f.PRHead(ctx, "o/r", 1); got != "" || err != nil {
		t.Errorf("PRHead = (%q, %v), want zero values", got, err)
	}
	if got, err := f.PRState(ctx, "o/r", 1); got != "" || err != nil {
		t.Errorf("PRState = (%q, %v), want zero values", got, err)
	}
	if got, err := f.PRTitle(ctx, "o/r", 1); got != "" || err != nil {
		t.Errorf("PRTitle = (%q, %v), want zero values", got, err)
	}
	if got, err := f.PostReview(ctx, "o/r", 1, Payload{}); got != 0 || err != nil {
		t.Errorf("PostReview = (%d, %v), want zero values", got, err)
	}
	if got, err := f.ListPRs(ctx, "o/r", ""); got != nil || err != nil {
		t.Errorf("ListPRs = (%v, %v), want zero values", got, err)
	}
	if got, err := f.ListReviewRequestedPRs(ctx); got != nil || err != nil {
		t.Errorf("ListReviewRequestedPRs = (%v, %v), want zero values", got, err)
	}
	if got, err := f.PRMeta(ctx, 1); got != (PRMeta{}) || err != nil {
		t.Errorf("PRMeta = (%+v, %v), want zero values", got, err)
	}
	if got, err := f.PRDiff(ctx, 1); got != "" || err != nil {
		t.Errorf("PRDiff = (%q, %v), want zero values", got, err)
	}
}

// Each stub must receive the arguments the caller passed, otherwise tests
// built on the fake would silently assert against the wrong PR.
func TestFakeClientStubsReceiveTheirArguments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boom := errors.New("boom")

	f := FakeClient{
		AuthStatusFunc: func(context.Context) error { return boom },
		PRHeadFunc: func(_ context.Context, slug string, _ int) (string, error) {
			return slug + "#head", nil
		},
		PRStateFunc: func(_ context.Context, _ string, _ int) (string, error) {
			return PRStateMerged, nil
		},
		PRTitleFunc: func(_ context.Context, _ string, _ int) (string, error) {
			return "title", nil
		},
		PostReviewFunc: func(_ context.Context, _ string, n int, _ Payload) (int64, error) {
			return int64(n), nil
		},
		ListPRsFunc: func(_ context.Context, _, search string) ([]PRListItem, error) {
			return []PRListItem{{Number: 1, Title: search}}, nil
		},
		ListReviewRequestedPRsFunc: func(context.Context) ([]PRListItem, error) {
			return []PRListItem{{Number: 2}}, nil
		},
		PRMetaFunc: func(_ context.Context, n int) (PRMeta, error) {
			return PRMeta{Number: n}, nil
		},
		PRDiffFunc: func(_ context.Context, _ int) (string, error) {
			return "diff", nil
		},
	}

	if err := f.AuthStatus(ctx); !errors.Is(err, boom) {
		t.Errorf("AuthStatus = %v, want the stubbed error", err)
	}
	if got, _ := f.PRHead(ctx, "o/r", 1); got != "o/r#head" {
		t.Errorf("PRHead = %q, want the slug to reach the stub", got)
	}
	if got, _ := f.PRState(ctx, "o/r", 1); got != PRStateMerged {
		t.Errorf("PRState = %q", got)
	}
	if got, _ := f.PRTitle(ctx, "o/r", 1); got != "title" {
		t.Errorf("PRTitle = %q", got)
	}
	if got, _ := f.PostReview(ctx, "o/r", 77, Payload{}); got != 77 {
		t.Errorf("PostReview = %d, want the PR number to reach the stub", got)
	}
	if got, _ := f.ListPRs(ctx, "o/r", "is:open"); len(got) != 1 || got[0].Title != "is:open" {
		t.Errorf("ListPRs = %+v, want the search to reach the stub", got)
	}
	if got, _ := f.ListReviewRequestedPRs(ctx); len(got) != 1 || got[0].Number != 2 {
		t.Errorf("ListReviewRequestedPRs = %+v", got)
	}
	if got, _ := f.PRMeta(ctx, 5); got.Number != 5 {
		t.Errorf("PRMeta = %+v, want the number to reach the stub", got)
	}
	if got, _ := f.PRDiff(ctx, 5); got != "diff" {
		t.Errorf("PRDiff = %q", got)
	}
}

// GhClient is the production implementation of the same interface; a
// signature drift between the two would only show up at the call site
// otherwise.
var _ Client = (*GhClient)(nil)
