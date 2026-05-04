package github

import "context"

// FakeClient is a Client implementation that lets tests stub each method.
// Set the relevant Func field; unset fields return zero values / nil.
type FakeClient struct {
	AuthStatusFunc             func(ctx context.Context) error
	PRHeadFunc                 func(ctx context.Context, slug string, number int) (string, error)
	PostReviewFunc             func(ctx context.Context, slug string, number int, p Payload) (int64, error)
	ListReviewRequestedPRsFunc func(ctx context.Context) ([]PRListItem, error)
	PRMetaFunc                 func(ctx context.Context, number int) (PRMeta, error)
	PRDiffFunc                 func(ctx context.Context, number int) (string, error)
}

func (f *FakeClient) AuthStatus(ctx context.Context) error {
	if f.AuthStatusFunc != nil {
		return f.AuthStatusFunc(ctx)
	}
	return nil
}

func (f *FakeClient) PRHead(ctx context.Context, slug string, number int) (string, error) {
	if f.PRHeadFunc != nil {
		return f.PRHeadFunc(ctx, slug, number)
	}
	return "", nil
}

func (f *FakeClient) PostReview(ctx context.Context, slug string, number int, p Payload) (int64, error) {
	if f.PostReviewFunc != nil {
		return f.PostReviewFunc(ctx, slug, number, p)
	}
	return 0, nil
}

func (f *FakeClient) ListReviewRequestedPRs(ctx context.Context) ([]PRListItem, error) {
	if f.ListReviewRequestedPRsFunc != nil {
		return f.ListReviewRequestedPRsFunc(ctx)
	}
	return nil, nil
}

func (f *FakeClient) PRMeta(ctx context.Context, number int) (PRMeta, error) {
	if f.PRMetaFunc != nil {
		return f.PRMetaFunc(ctx, number)
	}
	return PRMeta{}, nil
}

func (f *FakeClient) PRDiff(ctx context.Context, number int) (string, error) {
	if f.PRDiffFunc != nil {
		return f.PRDiffFunc(ctx, number)
	}
	return "", nil
}
