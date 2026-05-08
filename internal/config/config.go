// Package config loads revu's optional TOML configuration.
//
// Each "config layer" is a directory holding a config.toml plus an optional
// templates/ subdirectory. Layers are consulted lowest priority first and
// merged onto Defaults(); values from higher layers override the same keys
// below.
//
//  1. os.UserConfigDir()/revu/                     (global user config)
//  2. <repo-root>/.revu/                           (project-shared, committed)
//  3. <repo-root>/.revu-local/                     (per-clone, gitignored)
//
// $REVU_CONFIG, when set, replaces the entire chain with that single file
// (used by tests and CI for isolation; templates/ discovery is skipped).
//
// Every layer is optional: missing files are silently skipped and Defaults()
// fills the gaps. All fields are documented inline.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// Source describes one config file that contributes to the effective
// configuration, in the order Load consulted it.
type Source struct {
	// Path is the absolute filesystem location revu inspected.
	Path string
	// Loaded is true when the file existed and parsed successfully.
	Loaded bool
}

// UserConfigDir returns the global user config directory
// (os.UserConfigDir()/revu). Used by `revu config --init` and the template
// resolver.
func UserConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(dir, "revu"), nil
}

// UserConfigPath returns ~/.config/revu/config.toml, the file revu writes
// when running `revu config --init`.
func UserConfigPath() (string, error) {
	dir, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// LayerDirs returns the ordered list of config-layer directories, lowest
// priority first. Each layer is a directory that may contain config.toml
// and/or templates/. When $REVU_CONFIG is set, this returns nil — the env
// override short-circuits layered discovery.
//
// Repo-root detection runs `git rev-parse --show-toplevel` in cwd. If that
// fails (not inside a git repo), the per-repo entries are omitted.
func LayerDirs() ([]string, error) {
	if os.Getenv("REVU_CONFIG") != "" {
		return nil, nil
	}
	user, err := UserConfigDir()
	if err != nil {
		return nil, err
	}
	out := []string{user}
	if root := repoRoot(); root != "" {
		out = append(out,
			filepath.Join(root, ".revu"),
			filepath.Join(root, ".revu-local"),
		)
	}
	return out, nil
}

// Sources returns the ordered list of TOML paths Load will consult, lowest
// priority first. When $REVU_CONFIG is set, only that path is returned.
// Otherwise sources are <layer-dir>/config.toml for every layer returned by
// LayerDirs.
func Sources() ([]string, error) {
	if v := os.Getenv("REVU_CONFIG"); v != "" {
		return []string{v}, nil
	}
	dirs, err := LayerDirs()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, filepath.Join(d, "config.toml"))
	}
	return out, nil
}

// RepoRoot returns the absolute path of the current git repo's top level,
// or "" when not in a git repo (or git is unavailable). Errors are silent
// because revu is usable outside a repo too. Exported for the template
// resolver, which lives in another package but needs the same root.
func RepoRoot() string { return repoRoot() }

// repoRoot is the unexported implementation; kept separate so the package
// docs stay focused on the exported API.
func repoRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// Load consults the sources returned by Sources() in order, merging each
// layer that exists onto Defaults(). Returns the effective config along
// with the per-source disposition, so `revu config` can show users which
// files contributed.
//
// A source whose file is missing is silently skipped. A source that fails
// to parse or validates poorly aborts Load with an error pointing at the
// offending path.
func Load() (cfg Config, sources []Source, err error) {
	paths, err := Sources()
	if err != nil {
		return Defaults(), nil, err
	}
	cfg = Defaults()
	sources = make([]Source, 0, len(paths))
	for _, p := range paths {
		st, statErr := os.Stat(p)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				sources = append(sources, Source{Path: p, Loaded: false})
				continue
			}
			return Defaults(), sources, statErr
		}
		if st.IsDir() {
			return Defaults(), sources, fmt.Errorf("%s is a directory; expected a TOML file", p)
		}

		var fileCfg Config
		if _, decodeErr := toml.DecodeFile(p, &fileCfg); decodeErr != nil {
			return Defaults(), sources, fmt.Errorf("parse %s: %w", p, decodeErr)
		}
		merged, mergeErr := merge(cfg, fileCfg)
		if mergeErr != nil {
			return Defaults(), sources, fmt.Errorf("%s: %w", p, mergeErr)
		}
		cfg = merged
		sources = append(sources, Source{Path: p, Loaded: true})
	}
	return cfg, sources, nil
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
