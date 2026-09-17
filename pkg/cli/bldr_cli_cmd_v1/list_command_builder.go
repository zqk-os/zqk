package bldr_cli_cmd_v1

import "github.com/spf13/cobra"

// NewListCommandBuilder is a compatibility alias for object list.
// TRACK: BLI-REDACTED — prefer NewObjectListCommandBuilder; path-qualified DNA.
func NewListCommandBuilder() *cobra.Command {
	return NewObjectListCommandBuilder()
}
