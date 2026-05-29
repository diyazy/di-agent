package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// newStrengthCmd returns `mapctl strength <propID> <value>`, which POSTs to
// /ontology/strength. Prints a one-line confirmation on success.
func newStrengthCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "strength <prop-id> <value>",
		Short: "Set a proposition's prior strength (POST /ontology/strength)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			v, err := strconv.ParseFloat(args[1], 64)
			if err != nil {
				return fmt.Errorf("strength must be a float in [0,1]: %w", err)
			}
			if err := deps.Client().SetStrength(deps.Ctx, id, v); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "strength %s = %.3f\n", id, v)
			return nil
		},
	}
}
