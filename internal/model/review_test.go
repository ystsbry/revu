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

func TestCommentLineLabel(t *testing.T) {
	t.Parallel()
	intp := func(n int) *int { return &n }
	sidep := func(s Side) *Side { return &s }

	cases := []struct {
		name string
		c    Comment
		want string
	}{
		{"single right", Comment{Line: 7, Side: SideRight}, "7"},
		{"single left", Comment{Line: 7, Side: SideLeft}, "L7"},
		{"range right", Comment{Line: 12, Side: SideRight, StartLine: intp(5)}, "5-12"},
		{"range left", Comment{Line: 12, Side: SideLeft, StartLine: intp(5)}, "L5-L12"},
		{"cross-side L→R", Comment{Line: 12, Side: SideRight, StartLine: intp(5), StartSide: sidep(SideLeft)}, "L5-R12"},
		{"cross-side R→L", Comment{Line: 12, Side: SideLeft, StartLine: intp(5), StartSide: sidep(SideRight)}, "R5-L12"},
		{"explicit same-side L start", Comment{Line: 12, Side: SideLeft, StartLine: intp(5), StartSide: sidep(SideLeft)}, "L5-L12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.LineLabel(); got != tc.want {
				t.Errorf("LineLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}
