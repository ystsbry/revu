package views

import (
	"errors"
	"fmt"

	"github.com/ystsbry/revu/internal/git"
)

// PreImageSource fetches the base-commit version of files in the repo. Used
// when a comment refers to a LEFT-side line so the line numbers align with
// the pre-image of the diff (rather than the working tree, which carries
// the post-image).
//
// Implementations must cache file contents by path; the detail view may
// query the same file repeatedly as the user navigates between comments.
type PreImageSource interface {
	Content(path string) ([]byte, error)
}

// gitPreImage is the default PreImageSource backed by `git show`. The base
// SHA is resolved lazily on first call via merge-base(headSHA, baseRef);
// this avoids spawning git for reviews that contain only RIGHT-side
// comments.
type gitPreImage struct {
	repoRoot   string
	headSHA    string
	baseRef    string // typically a branch name like "main"
	baseSHA    string // resolved on first use
	resolveErr error
	resolved   bool
	cache      map[string][]byte
}

// NewGitPreImage constructs a PreImageSource backed by `git show`. baseRef
// is the base branch from the review (e.g. "main"); the actual base commit
// is computed as merge-base(headSHA, baseRef) on demand.
func NewGitPreImage(repoRoot, headSHA, baseRef string) PreImageSource {
	return &gitPreImage{
		repoRoot: repoRoot,
		headSHA:  headSHA,
		baseRef:  baseRef,
		cache:    make(map[string][]byte),
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
	if cached, ok := g.cache[path]; ok {
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
	g.cache[path] = raw
	return raw, nil
}
