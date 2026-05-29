package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newDeprecateCmd returns `mapctl deprecate <propID> <reason>`. The reason is
// the entire trailing positional argument (use shell quoting for multi-word).
func newDeprecateCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "deprecate <prop-id> <reason>",
		Short: "Mark a proposition as deprecated (POST /ontology/deprecate)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, reason := args[0], args[1]
			if err := deps.Client().Deprecate(deps.Ctx, id, reason); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deprecated %s (%s)\n", id, reason)
			return nil
		},
	}
}
