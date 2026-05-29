package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newConstructCmd returns `mapctl construct add <id> <name> <description>`.
// `construct` itself prints help; only the `add` subcommand mutates state.
func newConstructCmd(deps *Deps) *cobra.Command {
	parent := &cobra.Command{
		Use:   "construct",
		Short: "Manage constructs (add)",
	}
	parent.AddCommand(&cobra.Command{
		Use:   "add <id> <name> <description>",
		Short: "Add a construct (POST /ontology/construct)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, name, desc := args[0], args[1], args[2]
			if err := deps.Client().AddConstruct(deps.Ctx, id, name, desc); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added construct %s (%s)\n", id, name)
			return nil
		},
	})
	return parent
}
