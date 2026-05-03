package model

import "time"

const SchemaVersion = 1

type PRMeta struct {
	Repo       string `yaml:"repo"`
	Number     int    `yaml:"number"`
	HeadSHA    string `yaml:"head_sha"`
	BaseBranch string `yaml:"base_branch"`
}

type GeneratedBy struct {
	Tool  string `yaml:"tool"`
	Skill string `yaml:"skill"`
	Model string `yaml:"model"`
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
