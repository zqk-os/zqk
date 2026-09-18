// Referential integrity on the bulk delete path.
//
// Incident 2026-08-24: the retention_tolerance scheduler job erased 270 archived CAS objects
// (253 criteria among them) that other objects still referenced, leaving GhostRefs that
// `zqk system check` then reported. Single-object Delete has refused this since the 2026-08-03
// workstream loss (deleteImpl -> findDependents -> ErrMsgCannotDeleteDeps), but the bulk path
// reached the same erase without that refusal, so the invariant held or not depending on which
// entry point a caller happened to use.
//
// These tests assert the two entry points agree. The refusal is the interesting half: nothing in
// the repo covered it before this file, which is why the divergence survived.
package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// bulkDeleteRefIntegrityFixture builds a referenced critical object and a referrer that is NOT in
// any deletion set, so erasing the referenced object can only produce a dangling reference.
func bulkDeleteRefIntegrityFixture(t *testing.T) (f *FileObjectStorage, referenced, referrer string) {
	t.Helper()
	resetReverseReferenceIndexSyncForTest(t)
	t.Cleanup(func() { resetReverseReferenceIndexSyncForTest(t) })

	root := t.TempDir()
	mustEnsureProcessSpecsLayout(t, root)
	BindReverseReferenceIndexProjectRoot(root)

	var err error
	f, err = NewFileObjectStorage(root)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	// Bounded context, not t.Context(): that one is already cancelled during cleanup, and Shutdown
	// then blocks in WaitGroupManager.Wait until the test timeout kills the whole package.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = f.Shutdown(ctx)
	})

	ctx := WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	referenced = "CRIT-REFINT-001"
	referrer = "BLI-REFINT-001"

	if err := f.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          referenced,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "referenced criterion",
		objects.FieldKeyDescription: "held alive by a backlog item that is not being deleted",
		objects.FieldKeyStatus:      objects.ObjectStatusArchived,
	}); err != nil {
		t.Fatalf("create referenced criteria: %v", err)
	}
	if err := f.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:           referrer,
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyTitle:        "referrer",
		objects.FieldKeyDescription:  "points at the criterion under test",
		objects.FieldKeyCriteriaRefs: []string{referenced},
	}); err != nil {
		t.Fatalf("create referrer backlog item: %v", err)
	}

	// Guard the fixture itself: if the reverse index never learned the edge, a later "refused"
	// result would prove nothing and a "deleted" result would be a false alarm.
	deps := GetGlobalReverseReferenceIndex().GetDependents(referenced)
	if len(deps) == 0 {
		t.Fatalf("fixture invalid: reverse index has no dependents for %s, so neither delete path has anything to refuse", referenced)
	}
	return f, referenced, referrer
}

// TestBulkDeleteOptimized_refusesToOrphanOutOfSetDependents is the incident in one test: erasing a
// referenced object while its referrer survives.
//
// The slow path already computes findDependents for exactly this object. It then keeps only the
// dependents that are themselves in the deletion set (to order the deletes) and drops the rest —
// which are the only ones that can be orphaned. The answer was computed and discarded.
func TestBulkDeleteOptimized_refusesToOrphanOutOfSetDependents(t *testing.T) {
	f, referenced, referrer := bulkDeleteRefIntegrityFixture(t)
	ctx := WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	res, err := f.BulkDeleteOptimized(ctx, secCtx, []string{referenced}, false, 2)
	if err == nil && res != nil && res.FailureCount == 0 && res.SuccessCount > 0 {
		t.Fatalf("BulkDeleteOptimized erased %s while %s still references it: this is the GhostRef the retention sweep produced (success=%d)",
			referenced, referrer, res.SuccessCount)
	}

	// The refusal must be for referential integrity, not an unrelated failure that happens to
	// leave the object in place. Asserting the referrer is named is the strongest cheap check:
	// a generic "not found" or permission error cannot produce it.
	reason := ""
	if err != nil {
		reason = err.Error()
	} else if res != nil {
		for _, e := range res.Errors {
			reason += e.Message
		}
	}
	if !strings.Contains(reason, referrer) {
		t.Fatalf("delete was refused, but the reason does not name the referrer %s, so it may not be the integrity guard; got %q",
			referrer, reason)
	}

	if _, err := f.Read(ctx, secCtx, referenced); err != nil {
		t.Errorf("refused delete must leave %s readable, got %v", referenced, err)
	}
}

// TestSingleAndBulkDeleteAgreeOnDependentRefusal pins the two entry points to the same answer.
//
// Asserting bulk's behavior alone would let the pair drift again in the other direction (single
// delete quietly losing its check). The invariant is agreement, so the test states agreement.
func TestSingleAndBulkDeleteAgreeOnDependentRefusal(t *testing.T) {
	ctx := WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	fSingle, referencedSingle, _ := bulkDeleteRefIntegrityFixture(t)
	singleErr := fSingle.Delete(ctx, secCtx, referencedSingle, false)

	fBulk, referencedBulk, _ := bulkDeleteRefIntegrityFixture(t)
	res, bulkErr := fBulk.BulkDeleteOptimized(ctx, secCtx, []string{referencedBulk}, false, 2)
	bulkRefused := bulkErr != nil || (res != nil && res.SuccessCount == 0)

	if (singleErr != nil) != bulkRefused {
		t.Fatalf("single and bulk delete disagree on the same fixture: single refused=%v (%v), bulk refused=%v",
			singleErr != nil, singleErr, bulkRefused)
	}
}

// TestCascadeStillDeletesDependents keeps the fix from becoming a blanket refusal. Callers that
// explicitly ask to cascade must still succeed, otherwise the guard would break the documented
// break-glass path instead of narrowing it.
func TestCascadeStillDeletesDependents(t *testing.T) {
	f, referenced, _ := bulkDeleteRefIntegrityFixture(t)
	// WithTestHardDelete, not a bare system context: criteria is a core kernel kind, and elevation
	// alone no longer satisfies denyCoreKernelHardDelete. A cascade is still an erase, so this test
	// has to declare the same intent a real caller would supply via --reason-code.
	ctx := WithTestHardDelete(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	res, err := f.BulkDeleteOptimized(ctx, secCtx, []string{referenced}, true, 2)
	if err != nil {
		t.Fatalf("cascade bulk delete must not be refused by the dependent guard: %v", err)
	}
	if res == nil || res.SuccessCount == 0 {
		t.Fatalf("cascade bulk delete reported no successes: %+v", res)
	}
}
