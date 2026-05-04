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

	// Severities defines the set of severities revu accepts in review.yml
	// and exposes to the review-pr skill. When empty, revu falls back to
	// the built-in 4 levels (critical / major / minor / nit). When the
	// user provides one or more entries, the entire built-in list is
	// replaced (no per-name merging).
	Severities []SeverityDef `toml:"severity"`
}

// SeverityDef is one entry in the [[review.severity]] TOML array.
//
// Example:
//
//	[[review.severity]]
//	name = "critical"
//	level = 100
//	description = "..."
//	review_event = "REQUEST_CHANGES"
//	color = "red"
type SeverityDef struct {
	// Name is the identifier written into review.yml comments[].severity.
	// Must be non-empty and unique within the list.
	Name string `toml:"name"`

	// Level expresses relative importance. Higher = more severe. Used for
	// sorting and (in the future) range filters like "severity:>=80".
	Level int `toml:"level"`

	// Description is shown to the skill/LLM and to humans browsing config.
	Description string `toml:"description"`

	// ReviewEvent is the GitHub review event a comment of this severity
	// implies. Skill-side aggregation picks the strongest among comments.
	// Empty defaults to "COMMENT".
	ReviewEvent string `toml:"review_event"`

	// Color is an optional hint for TUI styling. Empty = no color.
	Color string `toml:"color"`
}

// Defaults returns the zero-config Config: empty editor.command (falls back
// to $EDITOR), default code/horizontal_threshold values from the views,
// and the built-in severity set surfaced via Review.Severities.
func Defaults() Config {
	return Config{
		UI: UIConfig{
			CodeContextLines:    5,
			HorizontalThreshold: 100,
		},
		Review: ReviewConfig{
			DefaultEvent: string(model.EventComment),
			Severities:   defaultSeverityDefs(),
		},
	}
}

// defaultSeverityDefs mirrors model.DefaultSeverityRegistry() but in
// config-shape so the same values flow through `revu config` output and
// `revu severities --json`.
func defaultSeverityDefs() []SeverityDef {
	infos := model.DefaultSeverityRegistry().All()
	out := make([]SeverityDef, len(infos))
	for i, s := range infos {
		out[i] = SeverityDef{
			Name:        s.Name,
			Level:       s.Level,
			Description: s.Description,
			ReviewEvent: string(s.ReviewEvent),
			Color:       s.Color,
		}
	}
	return out
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
	if len(over.Review.Severities) > 0 {
		// Validate by constructing a registry; replace the whole list.
		if _, err := BuildSeverityRegistry(over.Review.Severities); err != nil {
			return Config{}, fmt.Errorf("review.severity: %w", err)
		}
		out.Review.Severities = append([]SeverityDef(nil), over.Review.Severities...)
	}
	return out, nil
}

// BuildSeverityRegistry validates defs and constructs a model registry.
// Used by both config.merge (for early validation) and the CLI bootstrap
// (to install the runtime registry). When defs is empty, returns the
// built-in default registry.
func BuildSeverityRegistry(defs []SeverityDef) (*model.SeverityRegistry, error) {
	if len(defs) == 0 {
		return model.DefaultSeverityRegistry(), nil
	}
	infos := make([]model.SeverityInfo, len(defs))
	for i, d := range defs {
		infos[i] = model.SeverityInfo{
			Name:        d.Name,
			Level:       d.Level,
			Description: d.Description,
			ReviewEvent: model.ReviewEvent(d.ReviewEvent),
			Color:       d.Color,
		}
	}
	return model.NewSeverityRegistry(infos)
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

# Severity definitions. When omitted, the built-in 4 levels are used
# (critical / major / minor / nit). When you define one entry, the whole
# list is replaced. "level" expresses relative importance (higher = more
# severe). "review_event" is the GitHub review event a comment of this
# severity implies; the review-pr skill picks the strongest event across
# all comments (REQUEST_CHANGES > COMMENT > APPROVE).
#
# [[review.severity]]
# name = "critical"
# level = 100
# description = "本番障害・データ破損・重大セキュリティに直結する"
# review_event = "REQUEST_CHANGES"
# color = "red"
#
# [[review.severity]]
# name = "suggestion"
# level = 40
# description = "改善はするが優先度低、現状でも動く"
# review_event = "COMMENT"
# color = "cyan"
#
# [[review.severity]]
# name = "nit"
# level = 10
# description = "趣味・スタイルの提案、無視されても困らない"
# review_event = "COMMENT"
# color = "gray"
`
