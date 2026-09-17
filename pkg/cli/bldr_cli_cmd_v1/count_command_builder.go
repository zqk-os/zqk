package bldr_cli_cmd_v1

import "github.com/spf13/cobra"

// NewCountCommandBuilder is a compatibility alias for object count.
// TRACK: BLI-REDACTED — prefer NewObjectCountCommandBuilder; path-qualified DNA.
func NewCountCommandBuilder() *cobra.Command {
	return NewObjectCountCommandBuilder()
}
