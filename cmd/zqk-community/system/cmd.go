// Package system is the community-only first-run system surface.
// Studio NewSystemCmd still compiles the full operator and admin surface.
// TRACK: BLI-1789702449225534000-eca4a6bd
package system

import (
	"github.com/spf13/cobra"

	studiosystem "github.com/zqk-os/zqk/cmd/zqk/system"
)

// firstRunSystemCommand reports whether a Studio system subcommand belongs on
// the community product surface. Codegen, mesh, snapshot, and dogfood stay off.
func firstRunSystemCommand(name string) bool {
	switch name {
	case "agent-onboard",
		"check",
		"cleanup-duplicates",
		"config-get",
		"disk-usage",
		"init",
		"quickstart",
		"recover-cas",
		"repair-cas-corruption",
		"seed-default-agent-seating",
		"shutdown",
		"start",
		"start-here",
		"status",
		"sync-cas-index",
		"sync-git-hooks",
		"truth-sentinel",
		"validate",
		"whoami":
		return true
	default:
		return false
	}
}

// NewSystemCmd returns the first-run system group for the community binary.
func NewSystemCmd() *cobra.Command {
	cmd := studiosystem.NewSystemCmd()
	cmd.Short = "First-run kernel operations (init, check, status, dashboard)"
	cmd.Long = "Project initialization, health, and the first-run kernel pulse. Spec origination, codegen, and mesh verbs are not on this SKU."
	var drop []*cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Name() == "dashboard" {
			drop = append(drop, sub)
			continue
		}
		if !firstRunSystemCommand(sub.Name()) {
			drop = append(drop, sub)
		}
	}
	for _, sub := range drop {
		cmd.RemoveCommand(sub)
	}
	cmd.AddCommand(newDashboardCmd())
	return cmd
}
