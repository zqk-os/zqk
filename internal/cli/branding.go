package cli

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// Standalone "zqk" token, but not the ".zqk" data-dir prefix.
	reExecLower = regexp.MustCompile(`(^|[^.\w])(zqk)\b`)
	reProdExact = regexp.MustCompile(`\b(ZQK|NEXOS)\b`)
)

// ApplyBrandingToCommandTree rewrites user-facing help text in the Cobra tree so it matches the
// configured brand (product name) and executable name. This avoids hardcoding command names across
// the codebase and supports white-labeling.
//
// This intentionally does NOT rewrite cmd.Use for non-root commands (those are subcommand tokens).
func ApplyBrandingToCommandTree(root *cobra.Command, executableName, productName string) {
	if root == nil {
		return
	}
	if executableName == emptyValue {
		executableName = "zqk"
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

	// Update user-facing strings.
	cmd.Short = applyBrandingToText(cmd.Short, executableName, productName)
	cmd.Long = applyBrandingToText(cmd.Long, executableName, productName)
	cmd.Example = applyBrandingToText(cmd.Example, executableName, productName)

	for _, child := range cmd.Commands() {
		applyBrandingRecursive(child, executableName, productName)
	}
}

func applyBrandingToText(s, executableName, productName string) string {
	if s == emptyValue {
		return s
	}

	out := reExecLower.ReplaceAllString(s, "${1}"+executableName)

	// Product replacement: preserve full-uppercase look when the original token is uppercase.
	out = reProdExact.ReplaceAllStringFunc(out, func(match string) string {
		if match == strings.ToUpper(match) {
			return strings.ToUpper(productName)
		}
		return productName
	})

	return out
}
