package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DiyazY/di-agent/cmd/mapctl/render"
)

// newTuneCmd returns `mapctl tune <intent-text>`, which POSTs to /agent/tune.
// The intent text is a natural-language string describing the operator's
// priority. The daemon maps it to proposition strength adjustments and returns
// a list of applied changes.
func newTuneCmd(deps *Deps) *cobra.Command {
	var operator string
	cmd := &cobra.Command{
		Use:   "tune <intent-text>",
		Short: "Apply natural-language intent to proposition weights",
		Long: `Tune parses the operator intent text, maps it to proposition strength
adjustments, validates them against hard bounds, and applies the changes.
Returns the list of applied adjustments. Unknown intent returns empty output.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := deps.Client().Tune(cmd.Context(), args[0], operator)
			if err != nil {
				return err
			}
			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), resp)
			}
			if len(resp.Applied) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No adjustments applied (intent unrecognized or no matching propositions).")
				return nil
			}
			headers := []string{"Proposition", "Old", "New", "Rationale"}
			rows := make([][]string, len(resp.Applied))
			for i, a := range resp.Applied {
				rows[i] = []string{
					a.PropositionID,
					fmt.Sprintf("%.3f", a.OldStrength),
					fmt.Sprintf("%.3f", a.NewStrength),
					a.Rationale,
				}
			}
			render.Table(cmd.OutOrStdout(), headers, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&operator, "operator", "", "operator identifier logged in the audit trail")
	return cmd
}
