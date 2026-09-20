package object

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewDraftCmd creates the object draft-plane command group.
func NewDraftCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDraftCommandBuilder(), &cobra.Command{
		Use:   "draft",
		Short: "Object draft-plane operations (enumerate / sweep / promote)",
	})
	cmd.AddCommand(NewDraftSweepCmd())
	cmd.AddCommand(NewDraftPromoteCmd())
	return cmd
}
