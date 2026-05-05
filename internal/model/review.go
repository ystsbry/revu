package model

import (
	"fmt"
	"strconv"
	"time"
)

const SchemaVersion = 1

type PRMeta struct {
	Repo       string `yaml:"repo"`
	Number     int    `yaml:"number"`
	HeadSHA    string `yaml:"head_sha"`
	BaseBranch string `yaml:"base_branch"`
}

type GeneratedBy struct {
	Tool      string `yaml:"tool"`
	Skill     string `yaml:"skill"`
	Model     string `yaml:"model"`
	SessionID string `yaml:"session_id,omitempty"`
}

type Comment struct {
	ID        string   `yaml:"id"`
	Status    Status   `yaml:"status"`
	Severity  Severity `yaml:"severity"`
	Category  Category `yaml:"category"`
	Path      string   `yaml:"path"`
	Line      int      `yaml:"line"`
	Side      Side     `yaml:"side"`
	StartLine *int     `yaml:"start_line,omitempty"`
	StartSide *Side    `yaml:"start_side,omitempty"`
	BodyFile  string   `yaml:"body_file"`

	// Derived (not persisted in YAML).
	Body string `yaml:"-"`
}

type Review struct {
	SchemaVersion int         `yaml:"schema_version"`
	PR            PRMeta      `yaml:"pr"`
	GeneratedAt   time.Time   `yaml:"generated_at"`
	GeneratedBy   GeneratedBy `yaml:"generated_by"`
	ReviewEvent   ReviewEvent `yaml:"review_event"`
	SummaryFile   string      `yaml:"summary_file"`
	Comments      []Comment   `yaml:"comments"`

	SubmittedAt *time.Time `yaml:"submitted_at,omitempty"`
	ReviewID    *int64     `yaml:"review_id,omitempty"`

	// Derived (not persisted in YAML).
	BaseDir     string `yaml:"-"`
	SummaryBody string `yaml:"-"`
}

// Counts returns the number of comments per status. Useful for the TUI status bar.
func (r *Review) Counts() map[Status]int {
	out := map[Status]int{
		StatusPending:  0,
		StatusAccepted: 0,
		StatusRejected: 0,
		StatusEdited:   0,
	}
	for _, c := range r.Comments {
		out[c.Status]++
	}
	return out
}

// FindComment returns a pointer to the comment with the given id, or nil if not found.
func (r *Review) FindComment(id string) *Comment {
	for i := range r.Comments {
		if r.Comments[i].ID == id {
			return &r.Comments[i]
		}
	}
	return nil
}

// LineLabel returns a compact string describing the diff position(s) this
// comment refers to. RIGHT-side single lines render as a bare number; any
// involvement of LEFT or a cross-side range adds explicit L/R prefixes so
// the position is unambiguous.
//
//   - "5"        single line, RIGHT
//   - "L5"       single line, LEFT
//   - "5-12"     same-side range, RIGHT
//   - "L5-L12"   same-side range, LEFT
//   - "L5-R12"   cross-side range (LEFT 5 → RIGHT 12)
func (c *Comment) LineLabel() string {
	if c.StartLine == nil {
		if c.Side == SideRight {
			return strconv.Itoa(c.Line)
		}
		return sideLineLabel(c.Line, c.Side)
	}
	startSide := c.Side
	if c.StartSide != nil {
		startSide = *c.StartSide
	}
	if startSide == SideRight && c.Side == SideRight {
		return fmt.Sprintf("%d-%d", *c.StartLine, c.Line)
	}
	return fmt.Sprintf("%s-%s",
		sideLineLabel(*c.StartLine, startSide),
		sideLineLabel(c.Line, c.Side),
	)
}

func sideLineLabel(line int, side Side) string {
	prefix := "R"
	if side == SideLeft {
		prefix = "L"
	}
	return fmt.Sprintf("%s%d", prefix, line)
}
