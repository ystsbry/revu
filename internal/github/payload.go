package github

import (
	"errors"
	"fmt"

	"github.com/ystsbry/revu/internal/model"
)

// Payload mirrors POST /repos/{owner}/{repo}/pulls/{N}/reviews.
// JSON tags match the GitHub API schema exactly.
type Payload struct {
	CommitID string           `json:"commit_id"`
	Body     string           `json:"body"`
	Event    string           `json:"event"`
	Comments []PayloadComment `json:"comments"`
}

// PayloadComment is one inline comment element of the review.
type PayloadComment struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	Body      string `json:"body"`
	StartLine *int   `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

// BuildPayload converts an in-memory Review into a GitHub-API-shaped Payload,
// including only comments whose status is "accepted". Returns the payload
// and the IDs of comments that were filtered out (for prompt display).
func BuildPayload(r *model.Review) (Payload, FilteredCounts, error) {
	if r == nil {
		return Payload{}, FilteredCounts{}, errors.New("nil review")
	}
	if r.PR.HeadSHA == "" {
		return Payload{}, FilteredCounts{}, errors.New("review.PR.HeadSHA is empty; cannot submit")
	}
	if r.SummaryBody == "" {
		return Payload{}, FilteredCounts{}, errors.New("summary body is empty; nothing to send as review body")
	}
	if !r.ReviewEvent.Valid() {
		return Payload{}, FilteredCounts{}, fmt.Errorf("invalid review_event %q", r.ReviewEvent)
	}

	var counts FilteredCounts
	out := Payload{
		CommitID: r.PR.HeadSHA,
		Body:     r.SummaryBody,
		Event:    string(r.ReviewEvent),
	}

	for i := range r.Comments {
		c := &r.Comments[i]
		switch c.Status {
		case model.StatusAccepted, model.StatusEdited:
			pc := PayloadComment{
				Path: c.Path,
				Line: c.Line,
				Side: string(c.Side),
				Body: c.Body,
			}
			if c.StartLine != nil {
				v := *c.StartLine
				pc.StartLine = &v
				if c.StartSide != nil {
					pc.StartSide = string(*c.StartSide)
				} else {
					pc.StartSide = string(c.Side)
				}
			}
			out.Comments = append(out.Comments, pc)
			counts.Accepted++
		case model.StatusRejected:
			counts.Rejected++
		case model.StatusPending:
			counts.Pending++
		}
	}

	return out, counts, nil
}

// FilteredCounts summarises which comments were and were not included in the
// payload. The submission confirmation uses these numbers to warn the user.
type FilteredCounts struct {
	Accepted int // included
	Rejected int // excluded by status
	Pending  int // excluded by status
}
