package storage

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestDenyCoreKernelHardDelete_BlocksWorkstream(t *testing.T) {
	t.Parallel()
	err := denyCoreKernelHardDelete(t.Context(), nil, objects.KindWorkstream, "WS-010")
	if err == nil {
		t.Fatal("expected hard delete refused for workstream")
	}
}

func TestDenyCoreKernelHardDelete_AllowContext(t *testing.T) {
	t.Parallel()
	ctx := pkgctx.WithAllowCoreObjectDelete(t.Context())
	if err := denyCoreKernelHardDelete(ctx, nil, objects.KindWorkstream, "WS-010"); err != nil {
		t.Fatalf("allow context should pass: %v", err)
	}
}

// TestDenyCoreKernelHardDelete_ElevationAloneNeverBypasses replaces two tests that asserted the
// exact opposite (delete:* and delete:core "should bypass reason-code"). They outlived the stance
// change and left this package asserting both sides at once: they contradicted
// TestElevatedActorStillNeedsDeclaredIntentForCoreKinds, so whichever behavior shipped, some test
// in this same package was green for the wrong reason.
//
// Every arm the refusal message names is checked separately. The system context satisfies all of
// them at once (see TestSystemContextSatisfiesEveryElevationArm), so testing only that context
// would not show which arm still opens the door.
func TestDenyCoreKernelHardDelete_ElevationAloneNeverBypasses(t *testing.T) {
	t.Parallel()
	arms := map[string]*pkgctx.SecurityContext{
		"delete_all": pkgctx.NewSecurityContext(
			"ACC-founder", []string{"ROL-003"},
			[]string{pkgctx.PermissionDeleteAll, "read:*", "write:*"},
		),
		"delete_core": pkgctx.NewSecurityContext(
			"ACC-ops", nil,
			[]string{pkgctx.PermissionDeleteCore, "delete:workstream"},
		),
		"admin_role":     {Roles: []string{"admin"}},
		"system_account": pkgctx.NewSystemSecurityContext(),
	}
	for name, sec := range arms {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := denyCoreKernelHardDelete(t.Context(), sec, objects.KindWorkstream, "WS-010"); err == nil {
				t.Errorf("%s alone erased a core kind with no declared intent; elevation answers "+
					"\"may you\", never \"did you mean to\"", name)
			}
		})
	}
}

func TestDenyCoreKernelHardDelete_WriteStarNotEnough(t *testing.T) {
	t.Parallel()
	sec := pkgctx.NewSecurityContext("ACC-dev", []string{"ROL-006"}, []string{"read:*", "write:*"})
	if err := denyCoreKernelHardDelete(t.Context(), sec, objects.KindWorkstream, "WS-010"); err == nil {
		t.Fatal("write:* without delete:* must still require reason-code")
	}
}

func TestDenyCoreKernelHardDelete_NonCoreOK(t *testing.T) {
	t.Parallel()
	if err := denyCoreKernelHardDelete(t.Context(), nil, objects.KindAuditEvent, "AUD-1"); err != nil {
		t.Fatalf("non-core should pass: %v", err)
	}
}

func TestFileObjectTransaction_DeleteRefusesCoreWithoutAllow(t *testing.T) {
	t.Parallel()
	// Enqueue path must fail closed before commit (same gate as DeleteByIDAndKind).
	tx := &FileObjectTransaction{
		ops: make([]fileTransactionOp, 0),
	}
	// Bypass Read by using DeleteByIDAndKind which already gates; also assert Delete path
	// would gate once kind is known — cover DeleteByIDAndKind as the tx membrane for critical.
	err := tx.DeleteByIDAndKind(t.Context(), "WS-tx-1", objects.KindWorkstream)
	if err == nil {
		t.Fatal("expected refuse for critical kind on tx DeleteByIDAndKind")
	}
}

func TestIsCoreKernelKind(t *testing.T) {
	t.Parallel()
	if !isCoreKernelKind(objects.KindCriteria) {
		t.Fatal("criteria should be core")
	}
	// Spec-driven: cas_entity defaults kernel_critical=true (fail-closed).
	if !isCoreKernelKind(objects.KindRiskBlocker) {
		t.Fatal("risk_blocker is cas_entity → kernel_critical by default")
	}
	if isCoreKernelKind(objects.KindAuditEvent) {
		t.Fatal("stream audit_event must not be core")
	}
	if isCoreKernelKind(objects.KindSchedulerJob) {
		t.Fatal("scheduler_job opts out with kernel_critical: false")
	}
}
