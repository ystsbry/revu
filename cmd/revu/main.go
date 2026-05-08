package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/config"
	"github.com/ystsbry/revu/internal/model"
)

var (
	version = "0.1.0-dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "revu",
		Short:         "Review viewer & GitHub submission agent for Claude Code generated reviews",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return installSeverityRegistry()
		},
	}
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newOpenCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newSubmitCmd())
	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newSeveritiesCmd())
	cmd.AddCommand(newReviewCmd())
	cmd.AddCommand(newResumeCmd())
	cmd.AddCommand(newPRCmd())
	cmd.AddCommand(newNowCmd())
	cmd.AddCommand(newPruneCmd())
	return cmd
}

// installSeverityRegistry loads config (if present) and installs the
// severity registry so subsequent store/filter validation accepts user-
// configured names. A missing config file is fine; a malformed one or
// invalid [[review.severity]] entries abort the command.
func installSeverityRegistry() error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	reg, err := config.BuildSeverityRegistry(cfg.Review.Severities)
	if err != nil {
		return err
	}
	model.SetActiveSeverityRegistry(reg)
	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print revu version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "revu %s (commit %s, built %s)\n", version, commit, date)
			return nil
		},
	}
}
