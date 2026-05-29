package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newResetCmd returns `mapctl reset <from> <to>`, which POSTs to /agent/reset
// to restore the prior weight on the edge between from and to.
func newResetCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "reset <from> <to>",
		Short: "Reset an edge's EMA back to its prior (POST /agent/reset)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, to := args[0], args[1]
			if err := deps.Client().ResetEdge(deps.Ctx, from, to); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reset edge %s -> %s\n", from, to)
			return nil
		},
	}
}
