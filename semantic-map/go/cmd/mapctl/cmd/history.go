package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DiyazY/di-agent/cmd/mapctl/client"
	"github.com/DiyazY/di-agent/cmd/mapctl/render"
)

// newHistoryCmd returns `mapctl history [--since]`. The --since flag accepts
// an RFC3339 timestamp or a Go duration; the daemon does the parsing.
func newHistoryCmd(deps *Deps) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show ontology event history, optionally filtered by --since",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := deps.Client().History(deps.Ctx, since)
			if err != nil {
				return err
			}
			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), events)
			}
			renderHistoryTable(cmd.OutOrStdout(), events)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "",
		"filter events newer than this RFC3339 timestamp or Go duration (e.g. 1h, 30m)")
	return cmd
}

// renderHistoryTable formats events with timestamp truncated to seconds and
// the detail map flattened to "k=v,k=v" so it fits in a single column.
func renderHistoryTable(w io.Writer, events []client.OntologyEventDTO) {
	rows := make([][]string, 0, len(events))
	for _, e := range events {
		rows = append(rows, []string{
			e.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
			e.Actor,
			e.Kind,
			e.TargetID,
			flattenDetail(e.Detail),
		})
	}
	render.Table(w, []string{"Time", "Actor", "Kind", "Target", "Detail"}, rows)
}

// flattenDetail converts the detail map to a compact "k=v,k=v" string,
// using JSON encoding so non-scalar values stay readable.
func flattenDetail(d map[string]any) string {
	if len(d) == 0 {
		return ""
	}
	parts := make([]string, 0, len(d))
	for k, v := range d {
		b, _ := json.Marshal(v)
		parts = append(parts, fmt.Sprintf("%s=%s", k, string(b)))
	}
	return strings.Join(parts, ",")
}
