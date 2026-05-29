package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DiyazY/di-agent/cmd/mapctl/client"
	"github.com/DiyazY/di-agent/cmd/mapctl/render"
)

// versionOutput is the JSON shape used when --json is set on `mapctl version`.
type versionOutput struct {
	ClientVersion string                  `json:"client_version"`
	Server        *client.VersionResponse `json:"server,omitempty"`
	ServerError   string                  `json:"server_error,omitempty"`
}

// newVersionCmd returns `mapctl version`. The server may be unreachable; in
// that case we print the client version with "(server unreachable)" and
// still exit 0, so this command can be used as an offline sanity check.
func newVersionCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print mapctl and daemon versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := versionOutput{ClientVersion: ClientVersion}
			server, err := deps.Client().Version(deps.Ctx)
			if err != nil {
				out.ServerError = err.Error()
			} else {
				out.Server = server
			}

			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), out)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "mapctl  %s\n", out.ClientVersion)
			if out.Server != nil {
				fmt.Fprintf(cmd.OutOrStdout(),
					"agent   %s (go=%s, commit=%s, constructs=%d, propositions=%d)\n",
					out.Server.AgentVersion, out.Server.GoVersion, out.Server.BuildCommit,
					out.Server.SemmapConstructs, out.Server.SemmapPropositions)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "agent   (server unreachable: %s)\n", out.ServerError)
			}
			return nil
		},
	}
}
