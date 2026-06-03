package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/DiyazY/di-agent/cmd/mapctl/client"
	"github.com/DiyazY/di-agent/cmd/mapctl/render"
)

// newPeersCmd returns the `mapctl peers` parent command and wires its four
// children (list, add, remove, trust). Mirrors the style of cmd_deprecate.go:
// one verb per subcommand, deps-bound RunE, table or JSON output via the
// shared render helpers.
func newPeersCmd(deps *Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "peers",
		Short: "Manage the agent's peer registry (list/add/remove/trust)",
		Long: "Subcommands for the multi-agent coordination layer.\n\n" +
			"GET    /peers           list           list registered peers and trust state\n" +
			"POST   /peers           add <url>      register a new peer\n" +
			"DELETE /peers/{id}      remove <id>    unregister a peer\n" +
			"POST   /peers/{id}/trust trust <id> <v> override a peer's trust score",
	}
	root.AddCommand(
		newPeersListCmd(deps),
		newPeersAddCmd(deps),
		newPeersRemoveCmd(deps),
		newPeersTrustCmd(deps),
	)
	return root
}

func newPeersListCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every registered peer (GET /peers)",
		RunE: func(cmd *cobra.Command, args []string) error {
			peers, err := deps.Client().ListPeers(deps.Ctx)
			if err != nil {
				return err
			}
			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), peers)
			}
			renderPeerTable(cmd, peers)
			return nil
		},
	}
}

func newPeersAddCmd(deps *Deps) *cobra.Command {
	var note string
	c := &cobra.Command{
		Use:   "add <url>",
		Short: "Register a new peer (POST /peers)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deps.Client().AddPeer(deps.Ctx, args[0], note)
			if err != nil {
				return err
			}
			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), d)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "registered peer %s (url=%s trust=%.2f)\n", d.ID, d.URL, d.Trust)
			return nil
		},
	}
	c.Flags().StringVar(&note, "note", "", "optional human-readable label for the peer")
	return c
}

func newPeersRemoveCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Unregister a peer by ID (DELETE /peers/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Client().RemovePeer(deps.Ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed peer %s\n", args[0])
			return nil
		},
	}
}

func newPeersTrustCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "trust <id> <value>",
		Short: "Override a peer's trust score (POST /peers/{id}/trust)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			val, err := strconv.ParseFloat(args[1], 64)
			if err != nil {
				return fmt.Errorf("value must be a float in [0, 1]: %w", err)
			}
			if err := deps.Client().SetPeerTrust(deps.Ctx, args[0], val); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set trust on peer %s to %.3f\n", args[0], val)
			return nil
		},
	}
}

// renderPeerTable prints peers as a six-column table. Empty registry → a
// short "no peers" message so the operator sees a clear signal rather than a
// blank header row.
func renderPeerTable(cmd *cobra.Command, peers []client.PeerDTO) {
	if len(peers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no peers registered")
		return
	}
	rows := make([][]string, 0, len(peers))
	for _, p := range peers {
		seen := "never"
		if !p.LastSeen.IsZero() {
			seen = p.LastSeen.Format(time.RFC3339)
		}
		rows = append(rows, []string{
			p.ID,
			p.URL,
			fmt.Sprintf("%.3f", p.Trust),
			fmt.Sprintf("%d", p.NObserved),
			seen,
			p.Note,
		})
	}
	render.Table(cmd.OutOrStdout(),
		[]string{"ID", "URL", "Trust", "N", "LastSeen", "Note"},
		rows)
}
