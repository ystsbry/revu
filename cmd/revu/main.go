package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	}
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newOpenCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newSubmitCmd())
	return cmd
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
