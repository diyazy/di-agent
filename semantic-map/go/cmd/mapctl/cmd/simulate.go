package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DiyazY/di-agent/cmd/mapctl/client"
	"github.com/DiyazY/di-agent/cmd/mapctl/render"
)

// newSimulateCmd returns `mapctl simulate`, which POSTs to /simulate.
func newSimulateCmd(deps *Deps) *cobra.Command {
	var (
		task      string
		source    string
		target    string
		size      int64
		latencyMs float64
		energyJ   float64
		energySet bool
	)
	cmd := &cobra.Command{
		Use:   "simulate",
		Short: "Simulate offload outcome for a target node (POST /simulate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			octx := client.OffloadContext{
				TaskType:        task,
				SourceNodeID:    source,
				DataSizeBytes:   size,
				LatencyBudgetMs: latencyMs,
			}
			if energySet {
				e := energyJ
				octx.EnergyBudgetJoules = &e
			}
			out, err := deps.Client().Simulate(deps.Ctx, octx, target)
			if err != nil {
				return err
			}
			if deps.JSON {
				return render.JSON(cmd.OutOrStdout(), out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Target:                %s\n", target)
			fmt.Fprintf(cmd.OutOrStdout(), "ExpectedLatency:       %.3f ms\n", out.ExpectedLatency)
			fmt.Fprintf(cmd.OutOrStdout(), "ExpectedResourceCost:  %.3f\n", out.ExpectedResourceCost)
			fmt.Fprintf(cmd.OutOrStdout(), "Confidence:            %.2f\n", out.Confidence)
			fmt.Fprintf(cmd.OutOrStdout(), "GraphPath:             %s\n", strings.Join(out.GraphPathUsed, " -> "))
			if out.P95Latency != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "P95Latency:            %.3f ms\n", *out.P95Latency)
			}
			if out.P95ResourceCost != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "P95ResourceCost:       %.3f\n", *out.P95ResourceCost)
			}
			if len(out.RiskFlags) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "RiskFlags:        %s\n", strings.Join(out.RiskFlags, ","))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "task type")
	cmd.Flags().StringVar(&source, "source", "", "source node ID")
	cmd.Flags().StringVar(&target, "target", "", "target node ID (required)")
	cmd.Flags().Int64Var(&size, "size", 0, "data size in bytes")
	cmd.Flags().Float64Var(&latencyMs, "latency-ms", 0, "latency budget in milliseconds")
	cmd.Flags().Float64Var(&energyJ, "energy-j", 0, "optional energy budget in joules")
	_ = cmd.MarkFlagRequired("target")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		energySet = cmd.Flags().Changed("energy-j")
	}
	return cmd
}
