package system

import (
	"fmt"

	"github.com/spf13/cobra"
)

// replace --fast with CAS-hash attestation
// (reuse last validated write when content hash unchanged). Until then, reduced
// surfaces are partial, non-authoritative, and must never mutate. Also: autofix
// must not demote lifecycle status to error without an explicit capability gate.

const (
	checkFlagFast           = "fast"
	checkFlagCheckRefs      = "check-refs"
	checkFlagAutoFix        = "auto-fix"
	checkFlagForce          = "force"
	checkFastPartialVerdict = "PARTIAL CHECK (refs skipped): reference integrity not verified — not an authoritative kernel-health verdict"
)

// refuseReducedSurfaceWithMutatingFlags upgrades a reduced surface to a full
// ref-checking run when --auto-fix or --force is set, instead of erroring.
// Mutation still requires the full surface; callers that pass --fast --auto-fix
// no longer flood the error log.
func refuseReducedSurfaceWithMutatingFlags(cmd *cobra.Command) error {
	if cmd == nil {
		return nil
	}
	autoFix, _ := cmd.Flags().GetBool(checkFlagAutoFix)
	force, _ := cmd.Flags().GetBool(checkFlagForce)
	if !autoFix && !force {
		return nil
	}
	if !checkRefsReduced(cmd) {
		return nil
	}
	// Explicitly warn so combining --fast with mutation flags is visible and not silent.
	fmt.Fprintf(cmd.ErrOrStderr(), "warning: --fast or --check-refs=false cannot be combined with mutation flags (--auto-fix/--force). Upgrading to full reference integrity check.\n")

	if cmd.Flags().Lookup(checkFlagFast) != nil {
		_ = cmd.Flags().Set(checkFlagFast, "false")
	}
	if cmd.Flags().Lookup(checkFlagCheckRefs) != nil {
		_ = cmd.Flags().Set(checkFlagCheckRefs, "true")
	}
	return nil
}

// refuseFastWithMutatingFlags is the historical name; prefer refuseReducedSurfaceWithMutatingFlags.
func refuseFastWithMutatingFlags(cmd *cobra.Command) error {
	return refuseReducedSurfaceWithMutatingFlags(cmd)
}

// checkRefsReduced reports that reference integrity will not run (fast and/or check-refs off).
func checkRefsReduced(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	fast, _ := cmd.Flags().GetBool(checkFlagFast)
	if fast {
		return true
	}
	if cmd.Flags().Lookup(checkFlagCheckRefs) == nil {
		return false
	}
	checkRefs, _ := cmd.Flags().GetBool(checkFlagCheckRefs)
	return !checkRefs
}

func checkFastModeEnabled(cmd *cobra.Command) bool {
	return checkRefsReduced(cmd)
}

func checkFastPartialPassLine() string {
	return fmt.Sprintf("%s. No violations found on the reduced surface (refs not checked).\n\n", checkFastPartialVerdict)
}
