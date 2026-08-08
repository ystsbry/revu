package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/config"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Switch between named subsets of the registered repositories",
		Long: `Profiles are named subsets of the [[repo]] registry, declared in config:

  [[profile]]
  name = "work"
  repos = ["acme/api", "acme/web"]

The active profile decides what repo-selection surfaces (revu repo list,
the dashboard) show. "default" is reserved and means every registered
repo. The choice is persisted in the user config as active_profile.`,
	}
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileUseCmd())
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List declared profiles and mark the active one",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := config.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			active := cfg.ActiveProfile
			if active == "" {
				active = config.DefaultProfileName
			}
			if _, ok := cfg.FindProfile(active); !ok && active != config.DefaultProfileName {
				fmt.Fprintf(out, "warning: active profile %q is not declared; showing all repos\n", active)
			}

			mark := func(name string) string {
				if name == active {
					return "* "
				}
				return "  "
			}
			fmt.Fprintf(out, "%s%s (all %d repos)\n", mark(config.DefaultProfileName), config.DefaultProfileName, len(cfg.Repos))
			for _, p := range cfg.Profiles {
				var missing []string
				known := 0
				for _, slug := range p.Repos {
					if _, ok := cfg.FindRepo(slug); ok {
						known++
					} else {
						missing = append(missing, slug)
					}
				}
				line := fmt.Sprintf("%s%s (%d repos)", mark(p.Name), p.Name, known)
				if len(missing) > 0 {
					line += fmt.Sprintf("  [unregistered: %s]", strings.Join(missing, ", "))
				}
				fmt.Fprintln(out, line)
			}
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(out, "\nNo profiles declared. Add [[profile]] blocks to your config.toml:")
				fmt.Fprintln(out, "  [[profile]]")
				fmt.Fprintln(out, "  name = \"work\"")
				fmt.Fprintln(out, "  repos = [\"owner/repo-a\", \"owner/repo-b\"]")
			}
			return nil
		},
	}
}

func newProfileUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: `Activate a profile ("default" goes back to all repos)`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			out := cmd.OutOrStdout()

			if name != config.DefaultProfileName {
				cfg, _, err := config.Load()
				if err != nil {
					return err
				}
				if _, ok := cfg.FindProfile(name); !ok {
					names := make([]string, 0, len(cfg.Profiles))
					for _, p := range cfg.Profiles {
						names = append(names, p.Name)
					}
					if len(names) == 0 {
						return fmt.Errorf("profile %q is not declared (no profiles exist yet; add [[profile]] blocks to config.toml)", name)
					}
					return fmt.Errorf("profile %q is not declared (available: %s, default)", name, strings.Join(names, ", "))
				}
			}

			if err := config.SetActiveUserProfile(name); err != nil {
				return err
			}
			if name == config.DefaultProfileName {
				fmt.Fprintln(out, "Active profile cleared: showing all registered repos.")
			} else {
				fmt.Fprintf(out, "Active profile: %s\n", name)
			}
			return nil
		},
	}
}
