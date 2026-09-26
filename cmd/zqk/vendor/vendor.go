package vendor

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewVendorCmd creates the root vendor command group
func NewVendorCmd() *cobra.Command {
	vendorCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewVendorVendorCommandBuilder(), &cobra.Command{})

	vendorCmd.AddCommand(NewCursorCmd())
	return vendorCmd
}
