package vendor

import (
	"github.com/spf13/cobra"
)

// NewVendorCmd creates the root vendor command group
func NewVendorCmd() *cobra.Command {
	vendorCmd := &cobra.Command{
		Use:   "vendor",
		Short: "Vendor-specific integrations and IDE adapters",
		Long:  "Vendor-specific integrations, adapters, and UI automation bridges isolated from the core kernel and scheduler planes.",
	}

	vendorCmd.AddCommand(NewCursorCmd())
	return vendorCmd
}
