package views

import (
	"errors"
	"fmt"

	"github.com/ystsbry/revu/internal/git"
)

// PreImageSource exposes the diff context revu needs to render LEFT-side
// and cross-side comments accurately: the pre-image of a file (so LEFT
// line numbers align) and the unified diff between base and head (so
// cross-side ranges can be shown as a hunk).
//
// Implementations must cache results by path; the detail view may query
// the same file repeatedly as the user navigates between comments.
type PreImageSource interface {
	Content(path string) ([]byte, error)
	Diff(path string) ([]byte, error)
}

// DefaultDiffContextLines is how many lines of context git diff carries
// per hunk by default. Aligned with code.go's default so LEFT-side single
// lines and cross-side hunks show comparable amounts of surrounding code.
const DefaultDiffContextLines = 5

// gitPreImage is the default PreImageSource backed by `git show`. The base
// SHA is resolved lazily on first call via merge-base(headSHA, baseRef);
// this avoids spawning git for reviews that contain only RIGHT-side
// comments.
type gitPreImage struct {
	repoRoot     string
	headSHA      string
	baseRef      string // typically a branch name like "main"
	baseSHA      string // resolved on first use
	resolveErr   error
	resolved     bool
	contentCache map[string][]byte
	diffCache    map[string][]byte
	diffCtx      int
}

// NewGitPreImage constructs a PreImageSource backed by `git show`. baseRef
// is the base branch from the review (e.g. "main"); the actual base commit
// is computed as merge-base(headSHA, baseRef) on demand.
func NewGitPreImage(repoRoot, headSHA, baseRef string) PreImageSource {
	return &gitPreImage{
		repoRoot:     repoRoot,
		headSHA:      headSHA,
		baseRef:      baseRef,
		contentCache: make(map[string][]byte),
		diffCache:    make(map[string][]byte),
		diffCtx:      DefaultDiffContextLines,
	}
}

func (g *gitPreImage) resolveBase() (string, error) {
	if g.resolved {
		return g.baseSHA, g.resolveErr
	}
	g.resolved = true
	if g.repoRoot == "" {
		g.resolveErr = errors.New("preimage: repo root not configured")
		return "", g.resolveErr
	}
	if g.headSHA == "" {
		g.resolveErr = errors.New("preimage: head SHA not set on review")
		return "", g.resolveErr
	}
	if g.baseRef == "" {
		g.resolveErr = errors.New("preimage: base branch not set on review")
		return "", g.resolveErr
	}
	sha, err := git.MergeBase(g.repoRoot, g.baseRef, g.headSHA)
	if err != nil {
		g.resolveErr = fmt.Errorf("preimage: resolve base: %w", err)
		return "", g.resolveErr
	}
	g.baseSHA = sha
	return sha, nil
}

func (g *gitPreImage) Content(path string) ([]byte, error) {
	if cached, ok := g.contentCache[path]; ok {
		return cached, nil
	}
	base, err := g.resolveBase()
	if err != nil {
		return nil, err
	}
	raw, err := git.Show(g.repoRoot, base, path)
	if err != nil {
		return nil, err
	}
	g.contentCache[path] = raw
	return raw, nil
}

func (g *gitPreImage) Diff(path string) ([]byte, error) {
	if cached, ok := g.diffCache[path]; ok {
		return cached, nil
	}
	base, err := g.resolveBase()
	if err != nil {
		return nil, err
	}
	raw, err := git.Diff(g.repoRoot, base, g.headSHA, path, g.diffCtx)
	if err != nil {
		return nil, err
	}
	g.diffCache[path] = raw
	return raw, nil
}
