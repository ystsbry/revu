package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PeekSubmittedAt reads <reviewDir>/review.yml and reports whether
// submitted_at is set to a non-empty value. It only parses the field of
// interest, so it is much cheaper than store.Load (no comment-body file
// reads, no schema validation). Used by `revu prune` to decide whether a
// review still has unsubmitted work.
//
// Returns (false, nil) when review.yml does not exist; the caller treats a
// missing file as "no submission record". Other I/O or parse errors are
// returned as-is.
func PeekSubmittedAt(reviewDir string) (bool, error) {
	if reviewDir == "" {
		return false, errors.New("reviewDir is required")
	}
	path := filepath.Join(reviewDir, "review.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var peek struct {
		SubmittedAt string `yaml:"submitted_at"`
	}
	if err := yaml.Unmarshal(raw, &peek); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	return peek.SubmittedAt != "", nil
}
