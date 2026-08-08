package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/store"
	"github.com/ystsbry/revu/internal/tui"
	"github.com/ystsbry/revu/internal/tui/picker"
)

func newOpenCmd() *cobra.Command {
	var repoRootFlag string
	cmd := &cobra.Command{
		Use:   "open [dir]",
		Short: "Open a review directory in the TUI",
		Long: `Open a review directory in the TUI.

If [dir] is omitted, the reviewed pr-N directories under
~/.revu/{owner}/{repo}/ (those that contain a review.yml) are listed in a
picker so you can choose which one to open. The repo is derived from the
current git remote, or from --repo when given.

By default, cwd's git remote must match the review's repo; when it does
not, a clone registered via 'revu repo add/scan' is used instead. Pass
--repo-root to point at a clone explicitly (or to bypass verification,
e.g. for opening fixture review dirs that have no matching local clone).`,
		Args: cobra.MaximumNArgs(1),
	}
	repoSlug := addRepoFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			if *repoSlug != "" {
				return fmt.Errorf("[dir] and --repo are mutually exclusive")
			}
			dir, err := store.ResolveReviewDir(args[0])
			if err != nil {
				return err
			}
			return openReviewDir(dir, repoRootFlag)
		}
		dir, err := pickReviewedPRDir(cmd, *repoSlug)
		if err != nil {
			return err
		}
		if dir == "" {
			return nil // user cancelled
		}
		return openReviewDir(dir, repoRootFlag)
	}
	cmd.Flags().StringVar(&repoRootFlag, "repo-root", "",
		"path to the local clone (skips cwd verification when set)")
	return cmd
}

// pickReviewedPRDir lists reviewed pr-* dirs for slug (cwd's repo when
// empty) and runs the picker. Returns "" when the user cancels.
func pickReviewedPRDir(cmd *cobra.Command, slug string) (string, error) {
	if slug == "" {
		var err error
		slug, err = store.CurrentRepoSlug()
		if err != nil {
			return "", fmt.Errorf("auto-resolve review dir (run inside a clone or pass --repo): %w", err)
		}
	}
	repoDir, err := store.RepoDir(slug)
	if err != nil {
		return "", err
	}
	dirs, err := store.ListReviewedPRDirs(repoDir)
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no reviewed PRs found under %s — run `revu review <PR>` first", repoDir)
	}

	items := make([]picker.LocalPRItem, 0, len(dirs))
	for _, d := range dirs {
		t := time.Time{}
		if st, err := os.Stat(filepath.Join(d.Path, "review.yml")); err == nil {
			t = st.ModTime()
		}
		items = append(items, picker.LocalPRItem{
			Number:      d.Number,
			ShortSHA:    d.ShortSHA,
			Path:        d.Path,
			GeneratedAt: t,
		})
	}
	picked, err := picker.PickLocal(items)
	if err != nil {
		return "", err
	}
	if picked == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
		return "", nil
	}
	return picked.Path, nil
}

// openReviewDir loads the review at dir and launches the TUI on it.
// repoRootOverride is the optional --repo-root flag value; empty means
// "verify cwd matches the review's repo".
func openReviewDir(dir, repoRootOverride string) error {
	r, err := store.Load(dir)
	if err != nil {
		return err
	}
	repoRoot, err := resolveRepoRoot(repoRootOverride, r.PR.Repo)
	if err != nil {
		return err
	}
	cfg, _, cfgErr := config.Load()
	if cfgErr != nil {
		return fmt.Errorf("load config: %w", cfgErr)
	}
	return tui.Run(tui.Config{
		Review:   r,
		Saver:    store.SaveStatuses,
		Reloader: reloadSubmissionMeta,
		RepoRoot: repoRoot,
		Settings: tui.Settings{
			EditorCommand:       cfg.Editor.Command,
			CodeContextLines:    cfg.UI.CodeContextLines,
			HorizontalThreshold: cfg.UI.HorizontalThreshold,
		},
	})
}

// reloadSubmissionMeta re-reads review.yml and copies SubmittedAt/ReviewID
// onto the in-memory Review. Used after `:submit` runs in a subprocess so
// the TUI shows the new submission record.
func reloadSubmissionMeta(r *model.Review) error {
	newR, err := store.Load(r.BaseDir)
	if err != nil {
		return err
	}
	r.SubmittedAt = newR.SubmittedAt
	r.ReviewID = newR.ReviewID
	return nil
}

// resolveRepoRoot returns the absolute repo root to hand to the TUI.
// When override is set, it must point to an existing directory; verification
// against expectedSlug is skipped. Otherwise cwd is checked against the slug.
func resolveRepoRoot(override, expectedSlug string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve --repo-root: %w", err)
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("--repo-root %s: %w", abs, err)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("--repo-root %s is not a directory", abs)
		}
		return abs, nil
	}
	if root, err := store.VerifyRepoMatches(expectedSlug); err == nil {
		return root, nil
	}
	// cwd is not the review's repo: fall back to the registered clone, so
	// `revu open` works from anywhere once the repo is `revu repo add`ed.
	if cfg, _, err := config.Load(); err == nil {
		if def, ok := cfg.FindRepo(expectedSlug); ok {
			if st, statErr := os.Stat(def.Path); statErr == nil && st.IsDir() {
				return def.Path, nil
			}
			return "", fmt.Errorf("registered clone for %s not found at %s (fix with `revu repo add <path>` or pass --repo-root)", expectedSlug, def.Path)
		}
	}
	return "", fmt.Errorf("revu open must run inside the matching repo (%s), have it registered (`revu repo add <path>`), or be given --repo-root", expectedSlug)
}
