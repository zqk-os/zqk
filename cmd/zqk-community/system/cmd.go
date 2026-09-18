// Package system is the dest-owned first-run system surface.
// Studio NewSystemCmd still compiles the leftover verbs; this SKU does not ship them.
// TRACK: BLI-1789702449225534000-eca4a6bd
package system

import (
	"github.com/spf13/cobra"

	studiosystem "github.com/zqk-os/zqk/cmd/zqk/system"
)

// firstRunSystemCommands is the open-core product surface.
// Codegen, mesh, snapshot, and studio dogfood stay off this binary.
var firstRunSystemCommands = map[string]struct{}{
	"agent-onboard":              {},
	"check":                      {},
	"cleanup-duplicates":         {},
	"config-get":                 {},
	"dashboard":                  {},
	"disk-usage":                 {},
	"init":                       {},
	"quickstart":                 {},
	"recover-cas":                {},
	"repair-cas-corruption":      {},
	"seed-default-agent-seating": {},
	"shutdown":                   {},
	"start":                      {},
	"start-here":                 {},
	"status":                     {},
	"sync-cas-index":             {},
	"sync-git-hooks":             {},
	"truth-sentinel":             {},
	"validate":                   {},
	"whoami":                     {},
}

// NewSystemCmd returns the first-run system group for this SKU.
func NewSystemCmd() *cobra.Command {
	cmd := studiosystem.NewSystemCmd()
	cmd.Short = "First-run kernel operations (init, check, status, dashboard)"
	cmd.Long = "Project initialization, health, and the first-run kernel pulse. Spec origination, codegen, and mesh verbs are not on this SKU."
	var drop []*cobra.Command
	for _, sub := range cmd.Commands() {
		if _, ok := firstRunSystemCommands[sub.Name()]; !ok {
			drop = append(drop, sub)
		}
	}
	for _, sub := range drop {
		cmd.RemoveCommand(sub)
	}
	return cmd
}
