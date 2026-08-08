package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/repo"
	"github.com/ystsbry/revu/internal/store"
)

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage the registered-repository list ([[repo]] in user config)",
		Long: `Manage the registered repositories revu can resolve without being run
inside the clone (used by the dashboard and by --repo flags).

Registrations live as [[repo]] blocks in the user config
(os.UserConfigDir()/revu/config.toml). revu edits only those blocks and
leaves the rest of the file untouched.`,
	}
	cmd.AddCommand(newRepoScanCmd())
	cmd.AddCommand(newRepoAddCmd())
	cmd.AddCommand(newRepoListCmd())
	cmd.AddCommand(newRepoRemoveCmd())
	return cmd
}

func newRepoScanCmd() *cobra.Command {
	var maxDepth int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "scan <root>",
		Short: "Walk a directory tree (e.g. a ghq root) and register every clone found",
		Long: `Walk <root> looking for git clones (directories containing .git), resolve
each origin remote to an "owner/repo" slug, and register new ones in the
user config. Already-registered slugs are skipped (their paths are left as
they are); git dirs without a resolvable origin are reported and ignored.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			res, err := repo.Scan(args[0], maxDepth)
			if err != nil {
				return err
			}

			registered, err := config.UserRepos()
			if err != nil {
				return err
			}
			known := make(map[string]struct{}, len(registered))
			for _, r := range registered {
				known[r.Slug] = struct{}{}
			}

			var toAdd []config.RepoDef
			for _, d := range res.Found {
				if _, ok := known[d.Slug]; ok {
					fmt.Fprintf(out, "skip (registered): %s\n", d.Slug)
					continue
				}
				toAdd = append(toAdd, d)
			}
			for _, s := range res.Skipped {
				fmt.Fprintf(out, "skip (no origin):  %s (%s)\n", s.Path, s.Reason)
			}
			for _, d := range toAdd {
				fmt.Fprintf(out, "add:               %s -> %s\n", d.Slug, d.Path)
			}
			if len(toAdd) == 0 {
				fmt.Fprintln(out, "Nothing new to register.")
				return nil
			}
			if dryRun {
				fmt.Fprintf(out, "Dry run: %d repo(s) would be registered.\n", len(toAdd))
				return nil
			}
			if _, err := config.UpsertUserRepos(toAdd); err != nil {
				return err
			}
			path, _ := config.UserConfigPath()
			fmt.Fprintf(out, "Registered %d repo(s) in %s\n", len(toAdd), path)
			return nil
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", repo.DefaultScanDepth,
		"how many directory levels below <root> to search")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"show what would be registered without writing the config")
	return cmd
}

func newRepoAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <path>",
		Short: "Register one clone by its local path",
		Long: `Resolve <path>'s origin remote to an "owner/repo" slug and register it in
the user config. Re-adding a known slug updates its path.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			slug, err := repo.DetectSlug(abs)
			if err != nil {
				return err
			}
			res, err := config.UpsertUserRepos([]config.RepoDef{{Slug: slug, Path: abs}})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case len(res.Added) == 1:
				fmt.Fprintf(out, "Registered %s -> %s\n", slug, abs)
			case len(res.Updated) == 1:
				fmt.Fprintf(out, "Updated %s -> %s\n", slug, abs)
			default:
				fmt.Fprintf(out, "Already registered: %s -> %s\n", slug, abs)
			}
			return nil
		},
	}
}

func newRepoListCmd() *cobra.Command {
	var all bool
	var profileFlag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered repositories (filtered by the active profile)",
		Long: `List registered repositories from all merged config layers.

The active profile (see 'revu profile') narrows the output to its repos;
--all ignores it and --profile <name> applies a different profile for
this invocation only.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := config.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(cfg.Repos) == 0 {
				fmt.Fprintln(out, "No repositories registered. Run `revu repo scan <root>` or `revu repo add <path>`.")
				return nil
			}

			switch {
			case all:
				cfg.ActiveProfile = ""
			case profileFlag != "":
				if _, ok := cfg.FindProfile(profileFlag); !ok && profileFlag != config.DefaultProfileName {
					return fmt.Errorf("profile %q is not declared (see `revu profile list`)", profileFlag)
				}
				cfg.ActiveProfile = profileFlag
			}

			repos, missing, unknown := cfg.ActiveRepos()
			if unknown != "" {
				fmt.Fprintf(out, "warning: active profile %q is not declared; showing all repos\n", unknown)
			} else if cfg.ActiveProfile != "" && cfg.ActiveProfile != config.DefaultProfileName {
				fmt.Fprintf(out, "Profile: %s (%d/%d repos)\n", cfg.ActiveProfile, len(repos), len(cfg.Repos))
			}

			for _, r := range repos {
				marker := ""
				if _, err := os.Stat(r.Path); err != nil {
					marker = "  (missing)"
				}
				line := fmt.Sprintf("%s  %s%s", r.Slug, r.Path, marker)
				if r.Search != "" {
					line += fmt.Sprintf("  [search: %s]", r.Search)
				}
				fmt.Fprintln(out, line)
			}
			for _, slug := range missing {
				fmt.Fprintf(out, "%s  (in profile but not registered)\n", slug)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "ignore the active profile and list every registered repo")
	cmd.Flags().StringVar(&profileFlag, "profile", "", "list a specific profile's repos for this invocation")
	return cmd
}

func newRepoRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <slug>",
		Short: "Remove a registered repository from the user config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := config.RemoveUserRepo(args[0])
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("%s is not registered in the user config", args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", args[0])
			return nil
		},
	}
}

// resolveReviewDirArg turns the shared ([dir] arg, --repo slug) pair into a
// concrete review directory. Precedence: an explicit [dir] wins; --repo
// resolves the slug's latest reviewed PR under ~/.revu without consulting
// cwd; with neither, cwd's git remote decides (store.ResolveReviewDir).
func resolveReviewDirArg(arg, repoSlug string) (string, error) {
	if arg != "" && repoSlug != "" {
		return "", fmt.Errorf("[dir] and --repo are mutually exclusive")
	}
	if repoSlug != "" {
		repoDir, err := store.RepoDir(repoSlug)
		if err != nil {
			return "", err
		}
		return store.LatestPRDir(repoDir)
	}
	return store.ResolveReviewDir(arg)
}

// addRepoFlag registers the shared --repo flag on cmd and returns the
// destination pointer.
func addRepoFlag(cmd *cobra.Command) *string {
	var slug string
	cmd.Flags().StringVar(&slug, "repo", "",
		`resolve the review by "owner/repo" slug instead of cwd (latest reviewed PR)`)
	return &slug
}
