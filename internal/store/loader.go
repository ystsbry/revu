package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ystsbry/revu/internal/model"
)

// LoadError carries a path and field hint along with the underlying cause,
// so the validate command can point users at the offending location.
type LoadError struct {
	Path  string
	Field string
	Cause error
}

func (e *LoadError) Error() string {
	switch {
	case e.Field != "" && e.Path != "":
		return fmt.Sprintf("%s: %s: %v", e.Path, e.Field, e.Cause)
	case e.Path != "":
		return fmt.Sprintf("%s: %v", e.Path, e.Cause)
	default:
		return e.Cause.Error()
	}
}

func (e *LoadError) Unwrap() error { return e.Cause }

// Load reads the review.yml at <dir>/review.yml, then resolves and reads
// the summary file and each comment body file. The returned Review is fully
// populated, including derived fields (BaseDir, SummaryBody, Comment.Body).
func Load(dir string) (*model.Review, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, &LoadError{Path: dir, Cause: err}
	}

	yamlPath := filepath.Join(abs, "review.yml")
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, &LoadError{Path: yamlPath, Cause: err}
	}

	var r model.Review
	if err := yaml.Unmarshal(raw, &r); err != nil {
		return nil, &LoadError{Path: yamlPath, Cause: err}
	}

	if r.SchemaVersion != model.SchemaVersion {
		return nil, &LoadError{
			Path:  yamlPath,
			Field: "schema_version",
			Cause: fmt.Errorf("unsupported schema_version %d (expected %d)", r.SchemaVersion, model.SchemaVersion),
		}
	}

	if err := validatePR(&r.PR); err != nil {
		return nil, &LoadError{Path: yamlPath, Field: "pr", Cause: err}
	}

	if r.SummaryFile == "" {
		return nil, &LoadError{Path: yamlPath, Field: "summary_file", Cause: errors.New("required")}
	}
	r.BaseDir = abs

	summaryPath := filepath.Join(abs, r.SummaryFile)
	summaryBody, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, &LoadError{Path: summaryPath, Field: "summary_file", Cause: err}
	}
	r.SummaryBody = string(summaryBody)

	seenIDs := make(map[string]struct{}, len(r.Comments))
	for i := range r.Comments {
		c := &r.Comments[i]
		if err := validateComment(c); err != nil {
			return nil, &LoadError{Path: yamlPath, Field: fmt.Sprintf("comments[%d]", i), Cause: err}
		}
		if _, dup := seenIDs[c.ID]; dup {
			return nil, &LoadError{Path: yamlPath, Field: fmt.Sprintf("comments[%d].id", i), Cause: fmt.Errorf("duplicate id %q", c.ID)}
		}
		seenIDs[c.ID] = struct{}{}

		bodyPath := filepath.Join(abs, c.BodyFile)
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return nil, &LoadError{Path: bodyPath, Field: fmt.Sprintf("comments[%d].body_file", i), Cause: err}
		}
		c.Body = string(body)
	}

	return &r, nil
}

func validatePR(p *model.PRMeta) error {
	if p.Repo == "" {
		return errors.New("pr.repo is required")
	}
	if p.Number <= 0 {
		return fmt.Errorf("pr.number must be positive, got %d", p.Number)
	}
	return nil
}

func validateComment(c *model.Comment) error {
	if c.ID == "" {
		return errors.New("id is required")
	}
	if !c.Status.Valid() {
		return fmt.Errorf("invalid status %q", c.Status)
	}
	if !c.Severity.Valid() {
		return fmt.Errorf("invalid severity %q", c.Severity)
	}
	if !c.Category.Valid() {
		return fmt.Errorf("invalid category %q", c.Category)
	}
	if !c.Side.Valid() {
		return fmt.Errorf("invalid side %q", c.Side)
	}
	if c.Path == "" {
		return errors.New("path is required")
	}
	if c.Line <= 0 {
		return fmt.Errorf("line must be positive, got %d", c.Line)
	}
	if c.BodyFile == "" {
		return errors.New("body_file is required")
	}
	if c.StartLine != nil && *c.StartLine > c.Line {
		return fmt.Errorf("start_line %d must be <= line %d", *c.StartLine, c.Line)
	}
	return nil
}
