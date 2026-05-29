package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// newWatchCmd returns `mapctl watch graph|edges`. It re-renders the selected
// view every --interval, clears the screen between frames with ANSI codes
// (suppressed when --no-color is set), and exits cleanly on Ctrl-C.
func newWatchCmd(deps *Deps) *cobra.Command {
	var interval time.Duration
	parent := &cobra.Command{
		Use:   "watch",
		Short: "Continuously re-render a view (graph|edges)",
	}
	parent.PersistentFlags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval")

	parent.AddCommand(&cobra.Command{
		Use:   "graph",
		Short: "Continuously re-render the full graph snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWatch(cmd, deps, interval, func(w io.Writer) error {
				snap, err := deps.Client().Graph(deps.Ctx)
				if err != nil {
					return err
				}
				renderGraphTables(w, snap)
				return nil
			})
		},
	})

	parent.AddCommand(&cobra.Command{
		Use:   "edges",
		Short: "Continuously re-render the edges table",
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			return runWatch(cmd, deps, interval, func(w io.Writer) error {
				edges, err := deps.Client().Edges(deps.Ctx, from, to)
				if err != nil {
					return err
				}
				renderEdgesTable(w, edges)
				return nil
			})
		},
	})

	// Filter flags on the edges variant only.
	for _, c := range parent.Commands() {
		if c.Use == "edges" {
			c.Flags().String("from", "", "source construct ID")
			c.Flags().String("to", "", "target construct ID")
		}
	}

	return parent
}

// runWatch drives the redraw loop. It honors --no-color by skipping the
// clear-screen escape sequence, which is the only reason the loop diverges
// from a plain ticker. Ctrl-C exits with nil so it scripts cleanly.
func runWatch(cmd *cobra.Command, deps *Deps, interval time.Duration, render func(io.Writer) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// First frame immediately, then on every tick.
	for {
		if !deps.NoColor {
			// ANSI: clear screen + move cursor to top-left.
			fmt.Fprint(cmd.OutOrStdout(), "\033[2J\033[H")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[mapctl watch — %s — every %s — Ctrl-C to exit]\n\n",
			time.Now().Format("15:04:05"), interval)
		if err := render(cmd.OutOrStdout()); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "watch: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
