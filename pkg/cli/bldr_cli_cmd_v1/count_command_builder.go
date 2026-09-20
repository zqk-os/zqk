package bldr_cli_cmd_v1

import "github.com/spf13/cobra"

// NewCountCommandBuilder is a compatibility alias for object count.
// TRACK: BLI-1785903708509306000-a6d8dc5b — prefer NewObjectCountCommandBuilder; path-qualified DNA.
func NewCountCommandBuilder() *cobra.Command {
	return NewObjectCountCommandBuilder()
}
