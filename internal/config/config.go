// Package config loads revu's optional TOML configuration from
// $REVU_CONFIG (when set) or os.UserConfigDir()/revu/config.toml.
//
// The file is optional: when missing, Defaults() is returned and revu
// behaves as if no config existed. All fields are documented inline.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/ystsbry/revu/internal/model"
)

// Config is the parsed shape of ~/.config/revu/config.toml.
type Config struct {
	Editor EditorConfig `toml:"editor"`
	UI     UIConfig     `toml:"ui"`
	Review ReviewConfig `toml:"review"`
}

// EditorConfig overrides the $EDITOR environment variable when non-empty.
type EditorConfig struct {
	// Command is the editor invocation, e.g. "code --wait" or "zed --wait".
	// Whitespace separates the executable from its flags.
	Command string `toml:"command"`
}

// UIConfig tweaks the TUI rendering.
type UIConfig struct {
	// CodeContextLines is how many lines of source the detail view shows
	// around the target line (in addition to the target itself).
	// Zero or negative leaves the built-in default (5).
	CodeContextLines int `toml:"code_context_lines"`

	// HorizontalThreshold is the minimum terminal width (in columns) at
	// which the detail view uses a side-by-side layout. Below this it
	// stacks vertically. Zero leaves the default (100).
	HorizontalThreshold int `toml:"horizontal_threshold"`
}

// ReviewConfig affects review-level defaults.
type ReviewConfig struct {
	// DefaultEvent is the review_event used for newly generated reviews
	// that omit the field. Currently unused by revu itself (Claude Code
	// skill writes review.yml), but reserved for future generators.
	DefaultEvent string `toml:"default_event"`
}

// Defaults returns the zero-config Config: empty editor.command (falls back
// to $EDITOR), default code/horizontal_threshold values from the views.
func Defaults() Config {
	return Config{
		UI: UIConfig{
			CodeContextLines:    5,
			HorizontalThreshold: 100,
		},
		Review: ReviewConfig{
			DefaultEvent: string(model.EventComment),
		},
	}
}

// Path returns the path that will be loaded by Load. Honors $REVU_CONFIG
// override; otherwise os.UserConfigDir()/revu/config.toml.
func Path() (string, error) {
	if v := os.Getenv("REVU_CONFIG"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(dir, "revu", "config.toml"), nil
}

// Load reads the TOML file at Path(), validates it, and merges it onto
// Defaults(). A missing file is not an error; Defaults() is returned along
// with the resolved path and ok=false.
func Load() (cfg Config, path string, ok bool, err error) {
	path, err = Path()
	if err != nil {
		return Defaults(), "", false, err
	}
	cfg = Defaults()
	if _, statErr := os.Stat(path); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return cfg, path, false, nil
		}
		return Defaults(), path, false, statErr
	}

	var fileCfg Config
	if _, err := toml.DecodeFile(path, &fileCfg); err != nil {
		return Defaults(), path, false, fmt.Errorf("parse %s: %w", path, err)
	}
	merged, err := merge(cfg, fileCfg)
	if err != nil {
		return Defaults(), path, false, err
	}
	return merged, path, true, nil
}

func merge(base, over Config) (Config, error) {
	out := base
	if over.Editor.Command != "" {
		out.Editor.Command = over.Editor.Command
	}
	if over.UI.CodeContextLines > 0 {
		out.UI.CodeContextLines = over.UI.CodeContextLines
	}
	if over.UI.HorizontalThreshold > 0 {
		out.UI.HorizontalThreshold = over.UI.HorizontalThreshold
	}
	if over.Review.DefaultEvent != "" {
		ev := model.ReviewEvent(over.Review.DefaultEvent)
		if !ev.Valid() {
			return Config{}, fmt.Errorf("invalid review.default_event %q", over.Review.DefaultEvent)
		}
		out.Review.DefaultEvent = over.Review.DefaultEvent
	}
	return out, nil
}

// SampleTOML returns a starter config the user can drop at Path().
const SampleTOML = `# revu configuration. All keys are optional; remove what you don't need.

[editor]
# Editor command used by the 'e' key in the TUI. Whitespace separates
# executable + args. Falls back to $EDITOR (then "vi") when empty.
# command = "code --wait"

[ui]
# Lines of source shown above and below the target line in the detail view.
code_context_lines = 5

# Minimum terminal width for the side-by-side detail layout.
horizontal_threshold = 100

[review]
# Default review_event for new reviews (currently informational).
default_event = "COMMENT"
`
