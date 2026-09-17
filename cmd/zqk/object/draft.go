package object

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewDraftCmd creates the object draft-plane command group.
// TRACK: BLI-1785827957031623000-b08b9791
func NewDraftCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDraftCommandBuilder(), &cobra.Command{
		Use:   "draft",
		Short: "Object draft-plane operations (enumerate / sweep / promote)",
	})
	cmd.AddCommand(NewDraftSweepCmd())
	cmd.AddCommand(NewDraftPromoteCmd())
	return cmd
}
