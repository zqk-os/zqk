package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

var fixtureSeq atomic.Uint64

// fixtureID returns a per-call graph id so shared Memgraph rows do not collide
// across tests *and* across process runs (monotonic seq alone resets and reclaims PRI-1).
// TRACK: BLI-1785443942668406000-1ec5c811 — Local CI Memgraph pollution on reused fixture IDs.
func fixtureID(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), fixtureSeq.Add(1))
}

// mustCreateCASVisible creates obj then force-promotes to the intended status.
// Create coerces non-origin statuses to lifecycle origin (e.g. priority_plan → planning);
// tests need shovel-ready / in_progress fixtures without walking full template preconditions.
// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
func mustCreateCASVisible(t *testing.T, store storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) {
	t.Helper()
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		t.Fatal("mustCreateCASVisible: id required")
	}
	leave, _ := obj[objects.FieldKeyStatus].(string)
	kind, _ := obj[objects.FieldKeyKind].(string)
	if _, ok := obj[objects.FieldKeyTitle]; !ok {
		obj[objects.FieldKeyTitle] = id
	}
	if kind == objects.KindCriteria {
		if _, ok := obj[objects.FieldKeyCategory]; !ok {
			obj[objects.FieldKeyCategory] = "acceptance"
		}
	}
	// backlog_item cannot leave the grooming plane without an effort estimate, so a fixture that
	// omits it is refused at promote. Backfilled here rather than at each call site for the same
	// reason category is: these tests are asserting plan-membership and criterion behavior, and the
	// estimate is a precondition they have to satisfy rather than something they are exercising.
	if kind == objects.KindBacklogItem && leave != emptyValue {
		if _, ok := obj[objects.FieldKeyEstimatedEffort]; !ok {
			obj[objects.FieldKeyEstimatedEffort] = "1d"
		}
	}
	// complete/archived fail closed when criteria_refs is empty or unsatisfied
	// (pkg/validation/ref_status_constraints.go). Seed a validated CRIT so promote
	// can land; tests here are asserting emit/shockwave, not the complete gate.
	if kind == objects.KindBacklogItem && (leave == statusComplete || leave == objects.ObjectStatusArchived) {
		if !backlogCriteriaRefsPresent(obj) {
			critID := fixtureID(t, "CRIT")
			mustCreateCASVisible(t, store, ctx, secCtx, map[string]any{
				objects.FieldKeyID:       critID,
				objects.FieldKeyKind:     objects.KindCriteria,
				objects.FieldKeyStatus:   objects.ObjectStatusValidated,
				objects.FieldKeyTitle:    critID,
				objects.FieldKeyCategory: "acceptance",
			})
			obj[objects.FieldKeyCriteriaRefs] = []any{critID}
		}
	}

	_ = store.Delete(ctx, secCtx, id, true) //nolint:errcheck // shared Memgraph fixture reset

	err := store.Create(ctx, secCtx, obj)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		t.Fatalf("mustCreateCASVisible Create %s: %v", id, err)
	}

	forceCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "cas_visible_test_helpers promote")
	updates := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == objects.FieldKeyID || k == objects.FieldKeyKind {
			continue
		}
		updates[k] = v
	}
	if leave != "" {
		updates[objects.FieldKeyStatus] = leave
	}
	if err := store.Update(forceCtx, secCtx, id, updates); err != nil {
		t.Fatalf("mustCreateCASVisible promote %s → %s: %v", id, leave, err)
	}
	if leave != "" {
		obj[objects.FieldKeyStatus] = leave
	}
}

func backlogCriteriaRefsPresent(obj map[string]any) bool {
	v, ok := obj[objects.FieldKeyCriteriaRefs]
	if !ok || v == nil {
		return false
	}
	switch refs := v.(type) {
	case []any:
		return len(refs) > 0
	case []string:
		return len(refs) > 0
	default:
		return true
	}
}
