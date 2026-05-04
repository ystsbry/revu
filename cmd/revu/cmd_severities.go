package main

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ystsbry/revu/internal/model"
)

// newSeveritiesCmd exposes the active severity registry. The review-pr
// skill consumes `--json` to discover the user-configured severity set
// (name, level, description, review_event, color) and adapts its
// generated review_event accordingly.
func newSeveritiesCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "severities",
		Short: "Print the active severity registry",
		Long: `Print the severities revu accepts in review.yml.

Without --json, prints a human-readable table. With --json, prints a
machine-readable list intended for the review-pr skill (or other tools)
to discover the user's configured severities.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg := model.ActiveSeverityRegistry()
			defs := reg.SortedByLevel()

			if asJSON {
				return writeSeveritiesJSON(cmd, defs)
			}
			return writeSeveritiesTable(cmd, defs)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON for machine consumption (review-pr skill)")
	return cmd
}

type severityJSON struct {
	Name        string `json:"name"`
	Level       int    `json:"level"`
	Description string `json:"description,omitempty"`
	ReviewEvent string `json:"review_event"`
	Color       string `json:"color,omitempty"`
}

func writeSeveritiesJSON(cmd *cobra.Command, defs []model.SeverityInfo) error {
	out := make([]severityJSON, len(defs))
	for i, d := range defs {
		out[i] = severityJSON{
			Name:        d.Name,
			Level:       d.Level,
			Description: d.Description,
			ReviewEvent: string(d.ReviewEvent),
			Color:       d.Color,
		}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeSeveritiesTable(cmd *cobra.Command, defs []model.SeverityInfo) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tLEVEL\tEVENT\tCOLOR\tDESCRIPTION")
	for _, d := range defs {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			d.Name, d.Level, d.ReviewEvent, d.Color, d.Description)
	}
	return w.Flush()
}
