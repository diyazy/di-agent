package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// newPropositionCmd returns `mapctl proposition add <id> <from> <to> <+|-> <strength>`.
// Direction must be exactly "+" or "-"; the daemon validates and returns 400
// otherwise.
func newPropositionCmd(deps *Deps) *cobra.Command {
	parent := &cobra.Command{
		Use:   "proposition",
		Short: "Manage propositions (add)",
	}
	parent.AddCommand(&cobra.Command{
		Use:   "add <id> <from> <to> <+|-> <strength>",
		Short: "Add a validated proposition (POST /ontology/proposition)",
		Args:  cobra.ExactArgs(5),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, from, to, dir := args[0], args[1], args[2], args[3]
			if dir != "+" && dir != "-" {
				return fmt.Errorf("direction must be \"+\" or \"-\", got %q", dir)
			}
			s, err := strconv.ParseFloat(args[4], 64)
			if err != nil {
				return fmt.Errorf("strength must be a float in [0,1]: %w", err)
			}
			if err := deps.Client().AddProposition(deps.Ctx, id, from, to, dir, s); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added proposition %s: %s %s %s (strength=%.3f)\n",
				id, from, dir, to, s)
			return nil
		},
	})
	return parent
}
