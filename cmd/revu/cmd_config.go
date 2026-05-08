package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/config"
)

func newConfigCmd() *cobra.Command {
	var initFlag bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or initialise revu's configuration",
		Long: `Without arguments, prints the effective config (defaults merged with the
discovered config files, in precedence order) along with each source's
load status.

Resolution order, lowest priority first:
  1. ~/.config/revu/config.toml         (global)
  2. <repo-root>/.revu                  (project-shared, committed)
  3. <repo-root>/.revu-local            (per-clone, gitignored)

$REVU_CONFIG, when set, replaces the entire chain with that single file.

With --init, writes a starter config.toml to the global user-config path.
Will not overwrite an existing file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if initFlag {
				path, err := config.UserConfigPath()
				if err != nil {
					return err
				}
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("refusing to overwrite existing %s", path)
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return fmt.Errorf("create config dir: %w", err)
				}
				if err := os.WriteFile(path, []byte(config.SampleTOML), 0o644); err != nil {
					return fmt.Errorf("write config: %w", err)
				}
				fmt.Fprintf(out, "Wrote starter config to %s\n", path)
				return nil
			}

			cfg, sources, err := config.Load()
			if err != nil {
				return err
			}

			fmt.Fprintln(out, "Sources (lowest → highest priority):")
			if len(sources) == 0 {
				fmt.Fprintln(out, "  (none — using defaults)")
			}
			for _, s := range sources {
				status := "not present"
				if s.Loaded {
					status = "loaded"
				}
				fmt.Fprintf(out, "  %-12s %s\n", status, s.Path)
			}
			fmt.Fprintln(out)

			editor := cfg.Editor.Command
			if editor == "" {
				editor = "(falls back to $EDITOR)"
			}
			fmt.Fprintf(out, "editor.command              %s\n", editor)
			fmt.Fprintf(out, "ui.code_context_lines       %d\n", cfg.UI.CodeContextLines)
			fmt.Fprintf(out, "ui.horizontal_threshold     %d\n", cfg.UI.HorizontalThreshold)
			fmt.Fprintf(out, "review.default_event        %s\n", cfg.Review.DefaultEvent)
			fmt.Fprintf(out, "review.severity (%d)\n", len(cfg.Review.Severities))
			for _, s := range cfg.Review.Severities {
				fmt.Fprintf(out, "  - %-12s level=%-4d event=%-15s color=%s\n",
					s.Name, s.Level, s.ReviewEvent, s.Color)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&initFlag, "init", false, "write a starter config to ~/.config/revu/config.toml (will not overwrite)")
	return cmd
}
