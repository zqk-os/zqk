package storage

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPriorityPlanSecurityGate_CompleteRefusesNonTerminalChildren(t *testing.T) {
	testRoot := t.TempDir()
	fos, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fos)
		_ = RunProjectTestTeardown(opts)
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Create priority plan in grooming
	planID := "PRI-TEST-GATE-001"
	plan := map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Test Plan Gate",
		objects.FieldKeyDescription: "Test Priority Plan Gate Description",
		objects.FieldKeyStatus:      objects.ObjectStatusGrooming,
	}
	if err := fos.Create(ctx, secCtx, plan); err != nil {
		t.Fatalf("Create plan: %v", err)
	}

	// 2. Create child backlog item in deferred status
	childID := "BLI-TEST-CHILD-001"
	child := map[string]any{
		objects.FieldKeyID:              childID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Test Deferred Child",
		objects.FieldKeyDescription:     "Test Deferred Child Description",
		objects.FieldKeyStatus:          objects.ObjectStatusDeferred,
		objects.FieldKeyPriorityPlanRef: planID,
	}
	if err := fos.Create(ctx, secCtx, child); err != nil {
		t.Fatalf("Create child: %v", err)
	}

	// 3. Attempt to update plan to complete -> MUST FAIL
	err = fos.Update(ctx, secCtx, planID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	})
	if err == nil {
		t.Fatal("expected update to complete to fail when child is deferred, got nil")
	}
	if !strings.Contains(err.Error(), "Security Gate") && !strings.Contains(err.Error(), "non-terminal") {
		t.Fatalf("expected Security Gate non-terminal error, got: %v", err)
	}

	// 4. Unset priority_plan_ref on child (decouple non-terminal child)
	if err := fos.Update(ctx, secCtx, childID, map[string]any{
		objects.FieldKeyPriorityPlanRef: FieldUnset,
	}); err != nil {
		t.Fatalf("Unset child priority_plan_ref: %v", err)
	}

	// 5. Update plan to complete -> MUST SUCCEED now that non-terminal child is unlinked
	if err := fos.Update(ctx, secCtx, planID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}); err != nil {
		t.Fatalf("expected update to complete to succeed when child is unlinked, got: %v", err)
	}
}

func TestPriorityPlanSecurityGate_InProgressRefusesNonShovelReadyChildren(t *testing.T) {
	testRoot := t.TempDir()
	fos, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fos)
		_ = RunProjectTestTeardown(opts)
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Create priority plan in grooming
	planID := "PRI-TEST-GATE-002"
	plan := map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Test Plan Shovel Ready Gate",
		objects.FieldKeyDescription: "Test Priority Plan Shovel Ready Gate Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}
	if err := fos.Create(ctx, secCtx, plan); err != nil {
		t.Fatalf("Create plan: %v", err)
	}

	// 2. Create child backlog item in roadmap status (not shovel-ready)
	childID := "BLI-TEST-CHILD-002"
	child := map[string]any{
		objects.FieldKeyID:              childID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Test Roadmap Child",
		objects.FieldKeyDescription:     "Test Roadmap Child Description",
		objects.FieldKeyStatus:          objects.ObjectStatusRoadmap,
		objects.FieldKeyPriorityPlanRef: planID,
	}
	if err := fos.Create(ctx, secCtx, child); err != nil {
		t.Fatalf("Create child: %v", err)
	}

	// 3. Attempt to update plan to in_progress -> MUST FAIL
	err = fos.Update(ctx, secCtx, planID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	})
	if err == nil {
		t.Fatal("expected update to in_progress to fail when child is in roadmap, got nil")
	}
	if !strings.Contains(err.Error(), "Security Gate") && !strings.Contains(err.Error(), "non-shovel-ready") {
		t.Fatalf("expected Security Gate non-shovel-ready error, got: %v", err)
	}

	// 4. Update child to planned (shovel-ready)
	if err := fos.Update(ctx, secCtx, childID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	}); err != nil {
		t.Fatalf("Update child to planned: %v", err)
	}

	// 5. Update plan to in_progress -> MUST SUCCEED now that child is planned
	if err := fos.Update(ctx, secCtx, planID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}); err != nil {
		t.Fatalf("expected update to in_progress to succeed when child is planned, got: %v", err)
	}
}
