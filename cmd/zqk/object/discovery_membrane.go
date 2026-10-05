package object

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// FlagAllKinds break-glass for persona/RBAC discovery membrane (full kind catalog).
const FlagAllKinds = "all-kinds"

const flagHelpAllKinds = "Show full registered kind catalog (bypass planner/doer discovery membrane)"

// addAllKindsFlag registers --all-kinds when not already present.
func addAllKindsFlag(cmd *cobra.Command) {
	if cmd == nil || cmd.Flags().Lookup(FlagAllKinds) != nil {
		return
	}
	cmd.Flags().Bool(FlagAllKinds, false, flagHelpAllKinds)
}

// allKindsRequested reports whether --all-kinds was set (local or inherited).
func allKindsRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Lookup(FlagAllKinds) != nil {
		v, err := cmd.Flags().GetBool(FlagAllKinds)
		return err == nil && v
	}
	v, err := cmd.InheritedFlags().GetBool(FlagAllKinds)
	return err == nil && v
}

// applyDiscoveryMembrane filters kind enumeration for the current seat.
func applyDiscoveryMembrane(cmd *cobra.Command, proc *cli.Processor, kinds []string) (filtered []string, lane authcred.DiscoveryLane, allKinds bool) {
	allKinds = allKindsRequested(cmd)
	var secCtx *pkgctx.SecurityContext
	var projectRoot string
	if proc != nil {
		secCtx = proc.SecurityContext()
		projectRoot = proc.ProjectRoot()
	}
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}

	filtered, lane = authcred.FilterKindsForDiscovery(secCtx, kinds, allKinds)

	// S01/S02: list-kinds/create UX cannot advertise kinds missing a storage membrane
	dkm := objects.GetGlobalKindMapper()
	kmPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, paths.KindMappingsConfigFile)
	kmCfg, _ := objects.LoadKindMappingsConfig(kmPath)

	var membraneFiltered []string
	for _, k := range filtered {
		dir := dkm.GetDirectoryFromKind(k)
		isOnDemand := false
		if kmCfg != nil {
			isOnDemand = kmCfg.IsOnDemandKind(k)
		}
		if dir != "" || isOnDemand {
			membraneFiltered = append(membraneFiltered, k)
		}
	}

	return membraneFiltered, lane, allKinds
}
