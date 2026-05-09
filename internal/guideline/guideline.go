// Package guideline resolves user-specified review-guideline files
// (additional context the review-pr skill reads alongside its built-in
// viewpoints).
//
// Guideline paths come from each config layer's [review] guidelines = [...]
// array. Layers are concatenated in their config-precedence order
// (user → .revu → .revu-local), with absolute paths after Load. Missing
// files are tolerated: List() reports their status, Paths() filters them
// out so callers can pass the result straight into a Read loop.
package guideline

import (
	"os"

	"github.com/ystsbry/revu/internal/config"
)

// Resolved is one guideline entry with its absolute path and existence
// status as observed at call time.
type Resolved struct {
	Path   string
	Exists bool
}

// List returns every configured guideline (across all loaded config
// layers) tagged with whether the file currently exists. Order matches
// the order in the merged config: user-level entries first, then
// .revu, then .revu-local.
func List() ([]Resolved, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return nil, err
	}
	out := make([]Resolved, 0, len(cfg.Review.Guidelines))
	for _, p := range cfg.Review.Guidelines {
		out = append(out, Resolved{Path: p, Exists: fileExists(p)})
	}
	return out, nil
}

// Paths returns just the absolute paths of guidelines whose files
// currently exist. Suitable for piping into a shell `Read` loop in the
// review-pr skill.
func Paths() ([]string, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(all))
	for _, r := range all {
		if r.Exists {
			out = append(out, r.Path)
		}
	}
	return out, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !st.IsDir()
}
