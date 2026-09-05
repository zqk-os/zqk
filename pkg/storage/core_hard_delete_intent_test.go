package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// coreKindForGuardTest is a kind the guard must protect. Resolved rather than hardcoded so this
// test follows the critical-kind policy instead of pinning a copy of it.
func coreKindForGuardTest(t *testing.T) string {
	t.Helper()
	for _, kind := range []string{objects.KindCriteria, objects.KindBacklogItem, objects.KindPriorityPlan} {
		if isCoreKernelKind(kind) {
			return kind
		}
	}
	t.Skip("no core kernel kind resolvable in this environment")
	return ""
}

// TestSystemContextSatisfiesEveryElevationArm is the finding that makes the elevation arms
// indefensible, and it is why deleting the AccountID check alone would have been inert.
//
// NewSystemSecurityContext sets AccountID=SystemAccountID, Roles=[admin], and
// Permissions=[...,"delete:*"] simultaneously. MayHardDeleteCoreWithoutReason returns true on any
// one of those three, so removing one leaves the answer unchanged. Any fix framed as "drop the
// system-account bypass" is cosmetic; the stance itself has to change.
func TestSystemContextSatisfiesEveryElevationArm(t *testing.T) {
	sys := pkgctx.NewSystemSecurityContext()

	onlyAccount := &pkgctx.SecurityContext{AccountID: sys.AccountID}
	onlyRole := &pkgctx.SecurityContext{Roles: []string{"admin"}}
	onlyPerm := &pkgctx.SecurityContext{Permissions: []string{pkgctx.PermissionDeleteAll}}

	for name, sc := range map[string]*pkgctx.SecurityContext{
		"account_id_alone": onlyAccount,
		"admin_role_alone": onlyRole,
		"delete_all_alone": onlyPerm,
	} {
		if !pkgctx.MayHardDeleteCoreWithoutReason(sc) {
			t.Errorf("%s: expected this arm alone to grant elevation; if this changed, the "+
				"redundancy this test documents is gone and the comment above is stale", name)
		}
	}
	if !pkgctx.MayHardDeleteCoreWithoutReason(sys) {
		t.Fatal("system context should be elevated by construction")
	}
}

// TestElevatedActorStillNeedsDeclaredIntentForCoreKinds pins the stance change.
//
// The guard previously let any elevated actor erase a kernel-critical object with no declared
// reason. Because every daemon is constructed elevated, that made the guard unable to stop the
// caller it most needed to stop: the retention sweep of 2026-08-24 hard-deleted 270 archived
// kernel objects through exactly this arm, leaving GhostRefs. Elevation answers "may you"; it
// cannot answer "did you mean to", and only the second question protects the kernel from an
// automated sweep that is privileged by design.
func TestElevatedActorStillNeedsDeclaredIntentForCoreKinds(t *testing.T) {
	kind := coreKindForGuardTest(t)
	sys := pkgctx.NewSystemSecurityContext()

	err := denyCoreKernelHardDelete(context.Background(), sys, kind, "CRIT-GUARD-PROBE")
	if err == nil {
		t.Fatalf("elevated actor erased core kind %s with no declared intent; this is the arm the "+
			"retention sweep took", kind)
	}
	if !strings.Contains(err.Error(), "reason-code") {
		t.Errorf("refusal should tell the caller how to declare intent; got: %v", err)
	}

	// Declared intent still works, so this narrows the guard rather than blocking the path.
	if err := denyCoreKernelHardDelete(
		pkgctx.WithAllowCoreObjectDelete(context.Background()), sys, kind, "CRIT-GUARD-PROBE",
	); err != nil {
		t.Errorf("declared intent must still be honored for elevated actors: %v", err)
	}

	// Non-core kinds are untouched by this guard at any privilege level.
	if err := denyCoreKernelHardDelete(context.Background(), sys, "audit_event", "AUD-1"); err != nil {
		t.Errorf("non-core kind should not be gated here: %v", err)
	}
}
