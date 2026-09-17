package object

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// expandObjectIDArgs expands positional args and optional --ids into individual
// object IDs (comma-separated lists and whitespace segments). Shared by update,
// delete, get, promote, demote, and park so multi-ID CLI DNA stays aligned.
// TRACK: BLI-CEF-CLI-MULTI-ID-UPDATE
func expandObjectIDArgs(cmd *cobra.Command, args []string) []string {
	ids := clipkg.ExpandCommaSeparatedIDs(args...)
	if cmd == nil {
		return ids
	}
	if cmd.Flags().Lookup("ids") != nil && cmd.Flags().Changed("ids") {
		idsFlag, _ := cmd.Flags().GetString("ids") //nolint:errcheck // optional multi-ID surface
		ids = append(ids, clipkg.ExpandCommaSeparatedIDs(idsFlag)...)
	}
	return ids
}
