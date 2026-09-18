package cli

import (
	"github.com/lanceman/zqk/pkg/brand"
	"github.com/spf13/cobra"
)

// ApplyBrandingToCommandTree rewrites user-facing help text in the Cobra tree so it matches the
// configured brand (product name) and executable name. Canonical source tokens are `zqk` / `ZQK`.
// TRACK: TDE-1789678536875854000-47240146
func ApplyBrandingToCommandTree(root *cobra.Command, executableName, productName string) {
	if root == nil {
		return
	}
	if executableName == emptyValue {
		executableName = brand.CanonicalExecutableToken
	}
	if productName == emptyValue {
		productName = "ZQK"
	}

	applyBrandingRecursive(root, executableName, productName)
}

func applyBrandingRecursive(cmd *cobra.Command, executableName, productName string) {
	if cmd == nil {
		return
	}

	cmd.Short = brand.ApplyToUserText(cmd.Short, executableName, productName)
	cmd.Long = brand.ApplyToUserText(cmd.Long, executableName, productName)
	cmd.Example = brand.ApplyToUserText(cmd.Example, executableName, productName)

	for _, child := range cmd.Commands() {
		applyBrandingRecursive(child, executableName, productName)
	}
}
