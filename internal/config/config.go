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
	Repos  []RepoDef    `toml:"repo"`

	// Profiles are named subsets of the registered repositories; the one
	// named by ActiveProfile decides what repo-selection surfaces (repo
	// list, dashboard) show. No profiles / no active profile = everything.
	Profiles []ProfileDef `toml:"profile"`

	// ActiveProfile selects the profile in effect. Empty or "default"
	// means the full registry. Persisted by `revu profile use`.
	ActiveProfile string `toml:"active_profile"`
}

// ProfileDef is one entry in the [[profile]] TOML array.
//
// Example:
//
//	[[profile]]
//	name = "work"
//	repos = ["acme/api", "acme/web"]
//
// The name "default" is reserved: it always means "every registered repo"
// and cannot be declared explicitly.
type ProfileDef struct {
	// Name identifies the profile (used by `revu profile use <name>`).
	Name string `toml:"name"`

	// Repos lists the registered slugs this profile shows, in display
	// order. Slugs that are not (or no longer) registered are reported by
	// `revu repo list` / `revu profile list` rather than silently dropped.
	Repos []string `toml:"repos"`
}

// DefaultProfileName is the reserved profile meaning "all registered
// repos" — the state with no profile selected.
const DefaultProfileName = "default"

func validateProfileDef(p ProfileDef) error {
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	if p.Name == DefaultProfileName {
		return fmt.Errorf("profile name %q is reserved (it means all registered repos)", DefaultProfileName)
	}
	for _, s := range p.Repos {
		parts := strings.Split(s, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("profile %q: invalid repo slug %q (want \"owner/repo\")", p.Name, s)
		}
	}
	return nil
}

// FindProfile returns the profile named name, if declared.
func (c Config) FindProfile(name string) (ProfileDef, bool) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return ProfileDef{}, false
}

// ActiveRepos returns the registered repos the active profile selects, in
// the profile's order, plus any slugs the profile references that are not
// registered. With no active profile (or "default") every registered repo
// is returned in registry order. An active profile that does not exist
// resolves like default but is reported via unknownProfile, so callers can
// warn instead of silently showing everything.
func (c Config) ActiveRepos() (repos []RepoDef, missing []string, unknownProfile string) {
	name := c.ActiveProfile
	if name == "" || name == DefaultProfileName {
		return c.Repos, nil, ""
	}
	p, ok := c.FindProfile(name)
	if !ok {
		return c.Repos, nil, name
	}
	for _, slug := range p.Repos {
		if def, ok := c.FindRepo(slug); ok {
			repos = append(repos, def)
		} else {
			missing = append(missing, slug)
		}
	}
	return repos, missing, ""
}

// RepoDef is one entry in the [[repo]] TOML array: a registered repository
// the dashboard and cwd-independent commands can resolve without being run
// inside the clone.
//
// Example:
//
//	[[repo]]
//	slug = "ystsbry/revu"
//	path = "/home/me/ghq/github.com/ystsbry/revu"
//
// Registration normally lives in the global user layer
// (os.UserConfigDir()/revu/config.toml), written by `revu repo scan/add`;
// project layers may still add entries, which override the user layer per
// slug.
type RepoDef struct {
	// Slug is the GitHub "owner/repo" identifier. Required, unique.
	Slug string `toml:"slug"`

	// Path is the local clone location. Relative paths are resolved
	// against the config.toml that declared them; after Load() they are
	// absolute. Required.
	Path string `toml:"path"`

	// Search optionally narrows the PR list for this repository (consumed
	// by the dashboard's PR screens; free-form GitHub search terms).
	Search string `toml:"search,omitempty"`
}

// validateRepoDef rejects entries that cannot be resolved later. Path
// existence is deliberately not checked here: a registered clone may be
// temporarily absent (different machine, not yet cloned) and `revu repo
// list` surfaces that instead.
func validateRepoDef(d RepoDef) error {
	parts := strings.Split(d.Slug, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid repo slug %q (want \"owner/repo\")", d.Slug)
	}
	if d.Path == "" {
		return fmt.Errorf("repo %q: path is required", d.Slug)
	}
	return nil
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
	// and exposes to the revu:pr skill. When empty, revu falls back to
	// the built-in 4 levels (critical / major / minor / nit). When the
	// user provides one or more entries, the entire built-in list is
	// replaced (no per-name merging).
	Severities []SeverityDef `toml:"severity"`

	// ClaudeModel / CodexModel set the default model per engine for
	// review generation (`revu review`, background jobs, the dashboard's
	// run actions). The `--model` flag overrides them per invocation;
	// empty leaves each agent CLI's own default in charge. Split per
	// engine because the two vendors' model names do not overlap.
	ClaudeModel string `toml:"claude_model"`
	CodexModel  string `toml:"codex_model"`

	// Guidelines is the list of paths to additional review-guidance files
	// (markdown, plain text) that the revu:pr skill loads alongside its
	// built-in viewpoints. Paths in the TOML may be relative to the
	// containing config.toml; after Load() they are absolute and
	// concatenated across layers (user → .revu → .revu-local). Missing
	// files are tolerated at load time and surfaced via
	// `revu guidelines list`.
	Guidelines []string `toml:"guidelines"`
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
		merged, mergeErr := merge(cfg, fileCfg, filepath.Dir(p))
		if mergeErr != nil {
			return Defaults(), sources, fmt.Errorf("%s: %w", p, mergeErr)
		}
		cfg = merged
		sources = append(sources, Source{Path: p, Loaded: true})
	}
	return cfg, sources, nil
}

// merge applies the over layer onto base. baseDir is the directory of the
// config.toml that produced over; it's used to resolve relative paths
// (e.g. review.guidelines entries) against the layer that introduced them.
func merge(base, over Config, baseDir string) (Config, error) {
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
	if over.Review.ClaudeModel != "" {
		out.Review.ClaudeModel = over.Review.ClaudeModel
	}
	if over.Review.CodexModel != "" {
		out.Review.CodexModel = over.Review.CodexModel
	}
	if len(over.Review.Severities) > 0 {
		// Validate by constructing a registry; replace the whole list.
		if _, err := BuildSeverityRegistry(over.Review.Severities); err != nil {
			return Config{}, fmt.Errorf("review.severity: %w", err)
		}
		out.Review.Severities = append([]SeverityDef(nil), over.Review.Severities...)
	}
	for _, g := range over.Review.Guidelines {
		abs := g
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(baseDir, abs)
		}
		abs = filepath.Clean(abs)
		// Dedupe so the same path appearing in multiple layers (or twice
		// in the same layer by mistake) does not double-load.
		if !containsString(out.Review.Guidelines, abs) {
			out.Review.Guidelines = append(out.Review.Guidelines, abs)
		}
	}
	if len(over.Repos) > 0 {
		// Clone before the upsert below: base and out share the slice's
		// backing array, and merge must not mutate its input.
		out.Repos = append([]RepoDef(nil), out.Repos...)
	}
	for _, r := range over.Repos {
		if err := validateRepoDef(r); err != nil {
			return Config{}, err
		}
		if !filepath.IsAbs(r.Path) {
			r.Path = filepath.Join(baseDir, r.Path)
		}
		r.Path = filepath.Clean(r.Path)
		// Slug-keyed upsert: a later layer (or a later duplicate in the
		// same layer) overrides the earlier entry in place, so ordering
		// stays stable and there is never more than one entry per slug.
		if i := repoIndex(out.Repos, r.Slug); i >= 0 {
			out.Repos[i] = r
		} else {
			out.Repos = append(out.Repos, r)
		}
	}
	if len(over.Profiles) > 0 {
		// Same aliasing rule as Repos: clone before upserting.
		out.Profiles = append([]ProfileDef(nil), out.Profiles...)
	}
	for _, p := range over.Profiles {
		if err := validateProfileDef(p); err != nil {
			return Config{}, err
		}
		if i := profileIndex(out.Profiles, p.Name); i >= 0 {
			out.Profiles[i] = p
		} else {
			out.Profiles = append(out.Profiles, p)
		}
	}
	if over.ActiveProfile != "" {
		out.ActiveProfile = over.ActiveProfile
	}
	return out, nil
}

// profileIndex returns the index of name in defs, or -1.
func profileIndex(defs []ProfileDef, name string) int {
	for i, d := range defs {
		if d.Name == name {
			return i
		}
	}
	return -1
}

// repoIndex returns the index of slug in defs, or -1.
func repoIndex(defs []RepoDef, slug string) int {
	for i, d := range defs {
		if d.Slug == slug {
			return i
		}
	}
	return -1
}

// FindRepo returns the registered entry for slug, if any.
func (c Config) FindRepo(slug string) (RepoDef, bool) {
	if i := repoIndex(c.Repos, slug); i >= 0 {
		return c.Repos[i], true
	}
	return RepoDef{}, false
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
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

# Registered repositories: the dashboard and cwd-independent commands
# resolve "owner/repo" slugs to local clones through these entries.
# Normally maintained by 'revu repo scan <root>' / 'revu repo add <path>'
# rather than by hand; [[repo]] blocks are machine-managed (revu rewrites
# the matching block on update/remove, keeping everything else intact).
#
# [[repo]]
# slug = "ystsbry/revu"
# path = "/home/me/ghq/github.com/ystsbry/revu"
# # search = "label:needs-review"   # optional PR-list filter (dashboard)

# Profiles: named subsets of the registered repositories. The active one
# (set with 'revu profile use <name>', stored as active_profile) decides
# what 'revu repo list' and the dashboard show. "default" is reserved and
# means every registered repo.
#
# active_profile = "work"
#
# [[profile]]
# name = "work"
# repos = ["acme/api", "acme/web"]

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

# Additional review-guidance files the revu:pr skill reads alongside its
# built-in viewpoints. Paths are relative to this config.toml. Layers
# (user / .revu / .revu-local) are concatenated, so a project can append
# its own rules without losing global ones. Missing files are reported by
# 'revu guidelines list' but never abort a review.
# guidelines = [
#   "guidelines/coding-style.md",
#   "guidelines/security-checklist.md",
# ]

# Severity definitions. When omitted, the built-in 4 levels are used
# (critical / major / minor / nit). When you define one entry, the whole
# list is replaced. "level" expresses relative importance (higher = more
# severe). "review_event" is the GitHub review event a comment of this
# severity implies; the revu:pr skill picks the strongest event across
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
