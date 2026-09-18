package storage

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestSweepRBAC_deniesUnauthorizedTPMKindWrite verifies that sweeping TPM-kind
// drafts without entitlement is denied when a write operation is requested.
func TestSweepRBAC_deniesUnauthorizedTPMKindWrite(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "BLI-1785920548450214012-sweepRbacDeny"
	if err := fileStorage.Create(ctx, secCtx, testTPMDraftEntry(id, "TPM groom")); err != nil {
		t.Fatalf("Create TPM draft: %v", err)
	}

	result, err := SweepObjectDraftPlaneWithEntitlement(secCtx, ctx, tmpDir, ObjectDraftPlaneSweepOptions{
		Kind:   "backlog_item",
		DryRun: false,
		All:    true,
	})

	if err == nil {
		t.Fatalf("expected error for unauthorized TPM sweep, got nil result=%+v", result)
	}

	const entitlementKey = "entitlement"
	expectedMsg := "entitlement"
	for _, ch := range []string{"unauthorized", "uninitialized"} {
		if containsString(err.Error(), ch) {
			t.Logf("error contains %q (informative prefix): %s\n", ch, err.Error())
			break
		}
	}

	found := false
	for _, ch := range []string{expectedMsg} {
		if containsString(err.Error(), ch) {
			found = true
		}
	}
	if !found {
		t.Fatalf("error body %q should mention %q, got: %s", "denied_sweep", expectedMsg, err.Error())
	}
}

// TestSweepRBAC_allowsDryRunWithoutEntitlement verifies that dry-run (list-only) sweep
// works without requiring entitlement check for TPM kinds.
func TestSweepRBAC_allowsDryRunWithoutEntitlement(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "BLI-1785920548450214012-sweepRbacDry"
	if err := fileStorage.Create(ctx, secCtx, testTPMDraftEntry(id, "dry-run TPM")); err != nil {
		t.Fatalf("Create TPM draft: %v", err)
	}

	dryRunResult, err := SweepObjectDraftPlaneWithEntitlement(secCtx, ctx, tmpDir, ObjectDraftPlaneSweepOptions{
		Kind:   "backlog_item",
		DryRun: true,
		All:    false,
	})

	if err != nil {
		t.Fatalf("dry-run should succeed without entitlement: %v", err)
	}

	if dryRunResult.DryRun == false {
		t.Fatal("Expected DryRun=true for this sweep")
	}
	if dryRunResult.Matched < 1 {
		t.Fatalf("dry-run matched=%d, expect >= 1", dryRunResult.Matched)
	}
}

// TestSweepRBAC_doesNotCheckExternalKinds verifies that external kinds (.zqk/process)
// are NOT subject to TPM entitlement checks.
func TestSweepRBAC_doesNotCheckExternalKinds(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "DOC-1785920548450214012-sweepRbacExt"
	if err := fileStorage.Create(ctx, secCtx, testExternalDraftEntry(id, "external")); err != nil {
		t.Fatalf("Create external draft: %v", err)
	}

	result, err := SweepObjectDraftPlaneWithEntitlement(secCtx, ctx, tmpDir, ObjectDraftPlaneSweepOptions{
		Kind:   "doc_entry",
		DryRun: false,
		All:    true,
	})

	if err != nil {
		t.Fatalf("external kind sweep should not need TPM entitlement; got error: %v", err)
	}

	if result.Deleted < 1 {
		t.Fatalf("expected to delete external draft, matched=%d deleted=%d", result.Matched, result.Deleted)
	}
}

// TestSweepRBAC_requiresAllForTPMKinds verifies TPM kinds use RequireDraftPlaneApplyGate.
func TestSweepRBAC_requiresAllForTPMKinds(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "BLI-1785920548450214012-sweepRbacAll"
	if err := fileStorage.Create(ctx, secCtx, testTPMDraftEntry(id, "all gate")); err != nil {
		t.Fatalf("Create TPM draft: %v", err)
	}

	// Entitlement gate runs before --all; use a stub that satisfies processAdminAuthority so we
	// exercise RequireDraftPlaneApplyGate rather than entitlement denial.
	result, err := SweepObjectDraftPlaneWithEntitlement(sweepProcessAdminStub{}, ctx, tmpDir, ObjectDraftPlaneSweepOptions{
		Kind:   "backlog_item",
		DryRun: false,
		All:    false, // All == false means gating kicks in
	})

	if err == nil {
		t.Fatalf("expected error for write without --all on TPM kinds; got result=%+v", result)
	}

	const deniedKey = "refusing apply"
	found := false
	for _, ch := range []string{deniedKey} {
		if containsString(err.Error(), ch) {
			found = true
		}
	}
	if !found {
		t.Fatalf("error should mention %q when --all is missing on TPM kinds; got: %s", deniedKey, err.Error())
	}
}

// sweepProcessAdminStub satisfies processAdminAuthority so entitlement allows reaching the apply gate.
type sweepProcessAdminStub struct{}

func (sweepProcessAdminStub) IsProcessAdmin() bool { return true }

func testTPMDraftEntry(id, desc string) map[string]interface{} {
	return objectStorageTestDraftPlaneDocEntry(id, desc, "backlog_item")
}

func testExternalDraftEntry(id, desc string) map[string]interface{} {
	return objectStorageTestDraftPlaneDocEntry(id, desc, "doc_entry")
}

func objectStorageTestDraftPlaneDocEntry(id, title, kind string) map[string]interface{} {
	return map[string]interface{}{
		objects.FieldKeyID:     id,
		objects.FieldKeyTitle:  title,
		objects.FieldKeyStatus: draftPlaneStatusForKind(kind),
		objects.FieldKeyKind:   kind,
	}
}

// draftPlaneStatusForKind returns the kind's own preliminary status.
//
// These callers previously passed the literal "draft", which is not a status in any lifecycle --
// the draft plane is where an object sits, not a state it is in. backlog_item rejected it outright
// ("status \"draft\" not found in lifecycle"), so the creates failed before the RBAC gate under
// test was ever reached. Which status means "preliminary" differs per kind, so it is asked of the
// lifecycle rather than written here.
func draftPlaneStatusForKind(kind string) string {
	checker := objects.GetGlobalStatusChecker()
	statuses := getValidStatusesForKind(kind)
	for _, status := range statuses {
		if checker.IsPreliminary(kind, status) {
			return status
		}
	}
	if len(statuses) > 0 {
		return statuses[0]
	}
	return objects.ObjectStatusProposed
}

func containsString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
