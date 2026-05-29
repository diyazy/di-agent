package render

import (
	"encoding/json"
	"io"
)

// JSON writes v to w as indented JSON followed by a trailing newline. Used by
// every subcommand when --json is set, and by `mapctl dot` for inspection.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
