package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DiyazY/di-agent/cmd/mapctl/render"
)

// newHealthCmd returns `mapctl health`, which pings GET /healthz.
func newHealthCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Ping the daemon's /healthz endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := deps.Client().Health(deps.Ctx)
			if err != nil {
				return err
			}
			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), h)
			}
			if h.OK {
				fmt.Fprintln(cmd.OutOrStdout(), "ok")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "not ok")
			return nil
		},
	}
}
