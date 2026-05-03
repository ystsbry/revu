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
		Long: `Without arguments, prints the effective config (defaults merged
with values from the config file, if present) and the resolved file path.

With --init, writes a starter config.toml to the resolved path. Will not
overwrite an existing file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if initFlag {
				path, err := config.Path()
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

			cfg, path, ok, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Path:                       %s\n", path)
			if ok {
				fmt.Fprintln(out, "Status:                     loaded")
			} else {
				fmt.Fprintln(out, "Status:                     not present (defaults shown)")
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
			return nil
		},
	}
	cmd.Flags().BoolVar(&initFlag, "init", false, "write a starter config to the resolved path (will not overwrite)")
	return cmd
}
