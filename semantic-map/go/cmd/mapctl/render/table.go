// Package render holds the two output primitives shared by every mapctl
// subcommand: Table and JSON. Each subcommand builds its own header+row set
// (different columns per command); this package only owns the formatting.
package render

import (
	"io"

	"github.com/olekukonko/tablewriter"
)

// Table writes a bordered, header-aware table to w. Five lines, on purpose:
// the subcommands provide all schema-specific logic; this is just the wire.
func Table(w io.Writer, headers []string, rows [][]string) {
	t := tablewriter.NewWriter(w)
	t.SetHeader(headers)
	t.SetAutoWrapText(false)
	t.AppendBulk(rows)
	t.Render()
}
