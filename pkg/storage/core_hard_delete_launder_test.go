package storage

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestElevationIsNotLaunderedIntoDeclaredIntent pins the invariant that privilege never becomes
// intent, checking both channels a caller can present elevation through: the secCtx argument and
// the security context carried on ctx.
//
// This existed as a real defect, not a hypothetical. pkgctx.ContextWithElevatedCoreDeleteIfAllowed
// stamped the allow-flag for any elevated actor, and it was invoked on the line immediately above
// denyCoreKernelHardDelete on every production delete path:
//
//	ctx = pkgctx.ContextWithElevatedCoreDeleteIfAllowed(ctx, secCtx)  // graph_crud.go:418
//	if err := denyCoreKernelHardDelete(ctx, secCtx, kind, id); ...    // graph_crud.go:419
//
// Because every daemon is constructed elevated, that exempted precisely the callers the guard
// existed to stop, and it made an earlier narrowing of the guard's internal elevation check inert.
// Unit tests that call the guard directly cannot catch this: they skip the line above it. So this
// test asserts the composed outcome instead of the gate in isolation.
func TestElevationIsNotLaunderedIntoDeclaredIntent(t *testing.T) {
	kind := coreKindForGuardTest(t)

	arms := map[string]*pkgctx.SecurityContext{
		"system_account": pkgctx.NewSystemSecurityContext(),
		"admin_role":     {Roles: []string{"admin"}},
		"delete_all":     {Permissions: []string{pkgctx.PermissionDeleteAll}},
		"delete_core":    {Permissions: []string{pkgctx.PermissionDeleteCore}},
	}
	for name, sec := range arms {
		t.Run(name, func(t *testing.T) {
			// Elevation offered as an argument.
			if err := denyCoreKernelHardDelete(context.Background(), sec, kind, "CRIT-LAUNDER-PROBE"); err == nil {
				t.Errorf("%s (as secCtx arg) erased core kind %s with no declared intent", name, kind)
			}
			// Elevation offered on the context, which is how a reintroduced launderer upstream
			// would most likely reach the guard.
			onCtx := pkgctx.WithSecurityContext(context.Background(), sec)
			if err := denyCoreKernelHardDelete(onCtx, sec, kind, "CRIT-LAUNDER-PROBE"); err == nil {
				t.Errorf("%s (on ctx) erased core kind %s with no declared intent", name, kind)
			}
		})
	}

	// The narrowing must not have closed the legitimate door: declared intent still works.
	sys := pkgctx.NewSystemSecurityContext()
	declared := pkgctx.WithAllowCoreObjectDelete(context.Background())
	if err := denyCoreKernelHardDelete(declared, sys, kind, "CRIT-LAUNDER-PROBE"); err != nil {
		t.Errorf("declared intent must still be honored for elevated callers: %v", err)
	}
}
