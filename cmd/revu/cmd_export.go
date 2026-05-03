package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/github"
	"github.com/ystsbry/revu/internal/store"
)

func newExportCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "export [dir]",
		Short: "Export the GitHub submission payload",
		Long: `Export the GitHub submission payload (the body sent to
POST /repos/{owner}/{repo}/pulls/{N}/reviews) without contacting the API.

Useful for piping to jq or saving for inspection. Only --format json is
supported in MVP.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "json" {
				return fmt.Errorf("unsupported format %q (only 'json' is supported)", format)
			}
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			dir, err := store.ResolveReviewDir(arg)
			if err != nil {
				return err
			}
			r, err := store.Load(dir)
			if err != nil {
				return err
			}
			payload, _, err := github.BuildPayload(r)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	return cmd
}
