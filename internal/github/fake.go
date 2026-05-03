package github

import "context"

// FakeClient is a Client implementation that lets tests stub each method.
// Set the relevant Func field; unset fields return zero values / nil.
type FakeClient struct {
	AuthStatusFunc func(ctx context.Context) error
	PRHeadFunc     func(ctx context.Context, slug string, number int) (string, error)
	PostReviewFunc func(ctx context.Context, slug string, number int, p Payload) (int64, error)
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
