// Package template resolves user-managed template files from layered
// directories that mirror revu's config layers.
//
// Layers consulted, highest priority first:
//
//  1. <repo-root>/.revu-local/templates/{name}
//  2. <repo-root>/.revu/templates/{name}
//  3. $REVU_TEMPLATES/{name}                       (when env set)
//  4. ~/.config/revu/templates/{name}              (when env not set)
//
// Skill-bundled templates are intentionally NOT resolved here — the
// review-pr skill owns its install path and falls back to its own bundled
// copy when revu reports "not found", so revu stays agnostic of skill
// internals.
package template

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ystsbry/revu/internal/config"
)

// ErrNotFound is returned by Resolve when no layer holds the requested
// template. Callers should fall back to their own bundled copy.
var ErrNotFound = errors.New("template not found")

// Source describes one location consulted during resolution.
type Source struct {
	// Path is the absolute filesystem path that was checked.
	Path string
	// Layer is a short label describing the precedence layer
	// ("repo-local", "repo-shared", "env", "user").
	Layer string
	// Loaded is true when Path exists; only the highest-priority loaded
	// source is the "winner".
	Loaded bool
}

// SearchDirs returns the ordered list of template directories that
// Resolve will consult, highest priority first. Independent of $REVU_CONFIG
// (which only affects config.toml discovery): templates can come from the
// user dir even when the env override is in effect.
func SearchDirs() ([]Source, error) {
	out := make([]Source, 0, 4)

	if root := config.RepoRoot(); root != "" {
		out = append(out,
			Source{Path: filepath.Join(root, ".revu-local", "templates"), Layer: "repo-local"},
			Source{Path: filepath.Join(root, ".revu", "templates"), Layer: "repo-shared"},
		)
	}
	if env := os.Getenv("REVU_TEMPLATES"); env != "" {
		out = append(out, Source{Path: env, Layer: "env"})
	}
	user, err := config.UserConfigDir()
	if err != nil {
		return nil, err
	}
	out = append(out, Source{Path: filepath.Join(user, "templates"), Layer: "user"})

	for i := range out {
		if st, err := os.Stat(out[i].Path); err == nil && st.IsDir() {
			out[i].Loaded = true
		}
	}
	return out, nil
}

// Resolve returns the absolute path of the highest-priority template named
// `name` (e.g. "summary.md.tmpl"). When no layer holds the file, returns
// ErrNotFound so callers can fall back.
func Resolve(name string) (string, error) {
	if name == "" {
		return "", errors.New("template name is required")
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("template name %q must not contain path separators", name)
	}

	dirs, err := SearchDirs()
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		if !d.Loaded {
			continue
		}
		candidate := filepath.Join(d.Path, name)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	return "", ErrNotFound
}

// List enumerates every template file (by base name) discovered across all
// search dirs, mapped to the Source that wins resolution for it. Skipped
// entries are directories whose stat fails. Useful for `revu templates
// list` output.
func List() (map[string]Source, error) {
	dirs, err := SearchDirs()
	if err != nil {
		return nil, err
	}
	out := make(map[string]Source)
	for _, d := range dirs {
		if !d.Loaded {
			continue
		}
		entries, err := os.ReadDir(d.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if _, seen := out[name]; seen {
				// Already claimed by a higher-priority layer.
				continue
			}
			out[name] = Source{
				Path:   filepath.Join(d.Path, name),
				Layer:  d.Layer,
				Loaded: true,
			}
		}
	}
	return out, nil
}
