package model

import "testing"

func TestReviewCounts(t *testing.T) {
	t.Parallel()
	r := &Review{Comments: []Comment{
		{ID: "c1", Status: StatusPending},
		{ID: "c2", Status: StatusAccepted},
		{ID: "c3", Status: StatusAccepted},
		{ID: "c4", Status: StatusRejected},
	}}
	got := r.Counts()
	if got[StatusPending] != 1 || got[StatusAccepted] != 2 || got[StatusRejected] != 1 || got[StatusEdited] != 0 {
		t.Fatalf("unexpected counts: %#v", got)
	}
}

func TestReviewFindComment(t *testing.T) {
	t.Parallel()
	r := &Review{Comments: []Comment{
		{ID: "c1", Status: StatusPending},
		{ID: "c2", Status: StatusAccepted},
	}}
	if c := r.FindComment("c2"); c == nil || c.Status != StatusAccepted {
		t.Fatalf("FindComment(c2): %#v", c)
	}
	if c := r.FindComment("missing"); c != nil {
		t.Fatalf("FindComment(missing) should be nil, got %#v", c)
	}

	// Mutation through returned pointer must reflect in the slice.
	c := r.FindComment("c1")
	c.Status = StatusAccepted
	if r.Comments[0].Status != StatusAccepted {
		t.Fatalf("pointer mutation did not propagate")
	}
}
