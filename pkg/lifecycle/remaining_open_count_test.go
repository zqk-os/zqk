package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestCoerceNonNegInt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{3, 3, true},
		{int64(0), 0, true},
		{float64(2), 2, true},
		{float64(2.5), 0, false},
		{-1, 0, false},
		{"3", 0, false},
		{nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := coerceNonNegInt(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("in=%v got=(%d,%v) want=(%d,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestApplyPlanLastChildComplete_NonLastDoesNotComplete(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := fixtureID(t, "PRI-non-last")
	bliID := fixtureID(t, "BLI-non-last")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 planID,
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusInProgress,
		objects.FieldKeyTitle:              "two open",
		objects.FieldKeyRemainingOpenCount: 2,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          statusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "one of two",
	})

	logger := logging.NewEventLogger(ctx)
	ApplyPlanChildMembershipRemoved(ctx, logger, realStorage, projectRoot, planID, bliID)

	plan, err := realStorage.Read(ctx, secCtx, planID)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := plan[objects.FieldKeyStatus].(string); st != statusInProgress {
		t.Fatalf("plan status=%q want in_progress (non-last)", st)
	}
	n, ok := remainingOpenCountFrom(plan)
	if !ok || n != 1 {
		t.Fatalf("remaining_open_count=%d ok=%v want 1", n, ok)
	}
}

func TestApplyPlanLastChildComplete_UnsetFieldDoesNotComplete(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := fixtureID(t, "PRI-unseeded")
	bliID := fixtureID(t, "BLI-unseeded")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     planID,
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusInProgress,
		objects.FieldKeyTitle:  "unseeded",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          statusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "unseeded child",
	})

	logger := logging.NewEventLogger(ctx)
	ApplyPlanChildMembershipRemoved(ctx, logger, realStorage, projectRoot, planID, bliID)

	plan, err := realStorage.Read(ctx, secCtx, planID)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := plan[objects.FieldKeyStatus].(string); st == statusComplete {
		t.Fatal("unseeded remaining_open_count must not complete (fail closed, no List)")
	}
}

func TestApplyPlanLastChildComplete_ZeroCompletesWithoutMemberList(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := fixtureID(t, "PRI-last-child-complete")
	bliID := fixtureID(t, "BLI-last-child-only")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 planID,
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusInProgress,
		objects.FieldKeyTitle:              "last child plan",
		objects.FieldKeyActiveOrder:        1,
		objects.FieldKeyRemainingOpenCount: 1,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          statusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "only child",
	})

	logger := logging.NewEventLogger(ctx)
	ApplyPlanChildMembershipRemoved(ctx, logger, realStorage, projectRoot, planID, bliID)

	plan, err := realStorage.Read(ctx, secCtx, planID)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := plan[objects.FieldKeyStatus].(string); st != statusComplete {
		t.Fatalf("plan status=%q want complete (path=%s)", st, filepath.Join(projectRoot, ".zqk"))
	}
	if _, ok := plan[objects.FieldKeyActiveOrder]; ok {
		t.Fatalf("active_order must unset on last-child complete, got %v", plan[objects.FieldKeyActiveOrder])
	}
	if n, ok := remainingOpenCountFrom(plan); !ok || n != 0 {
		t.Fatalf("remaining_open_count=%d ok=%v want 0", n, ok)
	}
}

func TestSeedRemainingOpenCountFromMembers_CountsNonTerminal(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := fixtureID(t, "PRI-seed-count")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     planID,
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusInProgress,
		objects.FieldKeyTitle:  "seed me",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              fixtureID(t, "BLI-seed-open"),
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          statusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "open",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              fixtureID(t, "BLI-seed-done"),
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          statusComplete,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "done",
	})

	if err := SeedRemainingOpenCountFromMembers(ctx, realStorage, planID, true); err != nil {
		t.Fatal(err)
	}
	plan, err := realStorage.Read(ctx, secCtx, planID)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := remainingOpenCountFrom(plan)
	if !ok || n != 1 {
		t.Fatalf("remaining_open_count=%d ok=%v want 1", n, ok)
	}
}

func TestSeedRemainingOpenCountFromMembers_Milestone(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	milID := fixtureID(t, "MIL-seed-count")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     milID,
		objects.FieldKeyKind:   objects.KindMilestone,
		objects.FieldKeyStatus: statusInProgress,
		objects.FieldKeyTitle:  "milestone seed me",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           fixtureID(t, "BLI-mil-open-1"),
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyMilestoneRef: milID,
		objects.FieldKeyTitle:        "open 1 via scalar ref",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:            fixtureID(t, "BLI-mil-open-2"),
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyStatus:        statusInProgress,
		objects.FieldKeyMilestoneRefs: []any{milID},
		objects.FieldKeyTitle:         "open 2 via list refs",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           fixtureID(t, "BLI-mil-done"),
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       statusComplete,
		objects.FieldKeyMilestoneRef: milID,
		objects.FieldKeyTitle:        "done",
	})

	if err := SeedRemainingOpenCountFromMembers(ctx, realStorage, milID, true); err != nil {
		t.Fatal(err)
	}
	mil, err := realStorage.Read(ctx, secCtx, milID)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := remainingOpenCountFrom(mil)
	if !ok || n != 2 {
		t.Fatalf("milestone remaining_open_count=%d ok=%v want 2", n, ok)
	}
}

func TestApplyPlanChildMembershipRemoved_ShovelReadyDemotesToGrooming(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := fixtureID(t, "PRI-active-shovel-ready")
	bliID := fixtureID(t, "BLI-active-child")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyStatus:      statusActive,
		objects.FieldKeyTitle:       "shovel-ready plan",
		objects.FieldKeyActiveOrder: 3,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          "planned",
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "only child",
	})

	logger := logging.NewEventLogger(ctx)
	ApplyPlanChildMembershipRemoved(ctx, logger, realStorage, projectRoot, planID, bliID)

	plan, err := realStorage.Read(ctx, secCtx, planID)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := plan[objects.FieldKeyStatus].(string); st != statusGrooming {
		t.Fatalf("plan status=%q want grooming (demoted from active on last child removed)", st)
	}
	if _, ok := plan[objects.FieldKeyActiveOrder]; ok {
		t.Fatalf("active_order must be unset on demotion to grooming, got %v", plan[objects.FieldKeyActiveOrder])
	}
}

func TestApplyPlanChildMembershipRemoved_WithParkTransitionsToPaused(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	planID := fixtureID(t, "PRI-park-plan")
	bliID := fixtureID(t, "BLI-park-child")
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 planID,
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusInProgress,
		objects.FieldKeyTitle:              "execution locked plan",
		objects.FieldKeyRemainingOpenCount: 1,
		objects.FieldKeyActiveOrder:        1,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          statusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "child to remove",
	})

	logger := logging.NewEventLogger(ctx)
	ApplyPlanChildMembershipRemoved(ctx, logger, realStorage, projectRoot, planID, bliID, WithPark(true))

	plan, err := realStorage.Read(ctx, secCtx, planID)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := plan[objects.FieldKeyStatus].(string); st != statusPaused {
		t.Fatalf("plan status=%q want paused (halted via WithPark)", st)
	}
	if _, ok := plan[objects.FieldKeyActiveOrder]; ok {
		t.Fatalf("active_order must be unset on transition to paused, got %v", plan[objects.FieldKeyActiveOrder])
	}
}

