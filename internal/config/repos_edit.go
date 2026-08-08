// Machine editing of [[repo]] entries in the user config.toml.
//
// Policy (pinned by tests): edits are text-level and touch only [[repo]]
// blocks. Everything else in the file — comments, other tables, formatting
// — is preserved byte for byte. A [[repo]] block runs from its "[[repo]]"
// line to the line before the next table header (or EOF); replacing or
// removing a block drops any hand-written comments inside that block, which
// is the accepted cost of keeping the rest of the file untouched. After
// every edit the whole file is re-parsed; a result that no longer decodes
// aborts the write, so a bug here cannot corrupt the user's config.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// repoBlock is one [[repo]] block located in the file: the half-open line
// range [start, end) and the entry it declares.
type repoBlock struct {
	start, end int
	def        RepoDef
}

// UpsertResult reports what UpsertUserRepos did per entry.
type UpsertResult struct {
	Added   []RepoDef // new slug, block appended
	Updated []RepoDef // existing slug, block rewritten (path/search changed)
	Skipped []RepoDef // existing slug, identical entry — file untouched
}

// UpsertUserRepos registers defs in the user config.toml (created if
// missing), keyed by slug: unknown slugs are appended as new [[repo]]
// blocks, known slugs with different path/search have their block
// rewritten in place, identical entries are skipped.
func UpsertUserRepos(defs []RepoDef) (UpsertResult, error) {
	var res UpsertResult
	for _, d := range defs {
		if err := validateRepoDef(d); err != nil {
			return UpsertResult{}, err
		}
	}

	path, err := UserConfigPath()
	if err != nil {
		return UpsertResult{}, err
	}
	lines, err := readLines(path)
	if err != nil {
		return UpsertResult{}, err
	}
	if lines == nil {
		lines = []string{"# revu configuration. Repositories below are managed by 'revu repo'."}
	}

	for _, d := range defs {
		blocks, err := parseRepoBlocks(lines)
		if err != nil {
			return UpsertResult{}, fmt.Errorf("%s: %w", path, err)
		}
		existing := findBlock(blocks, d.Slug)
		switch {
		case existing == nil:
			lines = appendBlock(lines, d)
			res.Added = append(res.Added, d)
		case existing.def == d:
			res.Skipped = append(res.Skipped, d)
		default:
			lines = append(lines[:existing.start],
				append(formatRepoBlock(d), lines[existing.end:]...)...)
			res.Updated = append(res.Updated, d)
		}
	}

	if len(res.Added) == 0 && len(res.Updated) == 0 {
		return res, nil
	}
	if err := writeValidated(path, lines); err != nil {
		return UpsertResult{}, err
	}
	return res, nil
}

// RemoveUserRepo deletes the [[repo]] block for slug from the user
// config.toml. Returns false (and no error) when no such block exists.
func RemoveUserRepo(slug string) (bool, error) {
	path, err := UserConfigPath()
	if err != nil {
		return false, err
	}
	lines, err := readLines(path)
	if err != nil || lines == nil {
		return false, err
	}
	blocks, err := parseRepoBlocks(lines)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	blk := findBlock(blocks, slug)
	if blk == nil {
		return false, nil
	}

	// Also drop blank lines immediately above the block so repeated
	// add/remove cycles don't accumulate gaps.
	start := blk.start
	for start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	lines = append(lines[:start], lines[blk.end:]...)
	if err := writeValidated(path, lines); err != nil {
		return false, err
	}
	return true, nil
}

// UserRepos returns the [[repo]] entries currently declared in the user
// config.toml (no other layers). Missing file yields an empty list.
func UserRepos() ([]RepoDef, error) {
	path, err := UserConfigPath()
	if err != nil {
		return nil, err
	}
	lines, err := readLines(path)
	if err != nil || lines == nil {
		return nil, err
	}
	blocks, err := parseRepoBlocks(lines)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make([]RepoDef, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.def)
	}
	return out, nil
}

// readLines returns the file's lines, or nil when it does not exist.
func readLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n"), nil
}

// parseRepoBlocks locates every [[repo]] block and decodes each one in
// isolation, so a block's slug is read by the TOML parser rather than by
// fragile string matching.
func parseRepoBlocks(lines []string) ([]repoBlock, error) {
	var out []repoBlock
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "[[repo]]" {
			continue
		}
		end := i + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			end++
		}
		var doc struct {
			Repo []RepoDef `toml:"repo"`
		}
		text := strings.Join(lines[i:end], "\n")
		if _, err := toml.Decode(text, &doc); err != nil {
			return nil, fmt.Errorf("parse [[repo]] block at line %d: %w", i+1, err)
		}
		if len(doc.Repo) != 1 {
			return nil, fmt.Errorf("parse [[repo]] block at line %d: expected 1 entry, got %d", i+1, len(doc.Repo))
		}
		out = append(out, repoBlock{start: i, end: end, def: doc.Repo[0]})
		i = end - 1
	}
	return out, nil
}

func findBlock(blocks []repoBlock, slug string) *repoBlock {
	for i := range blocks {
		if blocks[i].def.Slug == slug {
			return &blocks[i]
		}
	}
	return nil
}

// formatRepoBlock renders the canonical machine-written block for d.
func formatRepoBlock(d RepoDef) []string {
	out := []string{
		"[[repo]]",
		fmt.Sprintf("slug = %q", d.Slug),
		fmt.Sprintf("path = %q", d.Path),
	}
	if d.Search != "" {
		out = append(out, fmt.Sprintf("search = %q", d.Search))
	}
	return out
}

// appendBlock adds d as a new block at the end of the file, separated by
// one blank line.
func appendBlock(lines []string, d RepoDef) []string {
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines, formatRepoBlock(d)...)
}

// writeValidated re-parses the edited content as a full Config and only
// then writes it, atomically, so a broken edit can never land on disk.
func writeValidated(path string, lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	var check Config
	if _, err := toml.Decode(content, &check); err != nil {
		return fmt.Errorf("internal error: edited config no longer parses (aborting write): %w", err)
	}
	for _, r := range check.Repos {
		if err := validateRepoDef(r); err != nil {
			return fmt.Errorf("internal error: edited config invalid (aborting write): %w", err)
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
