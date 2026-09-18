package testrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/testrunner"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestShockwaveHarness_EndToEnd_HelloWorld verifies the entire reactive shockwave
// state machine end-to-end:
//
// 1. Setup an objective verification scenario:
//   - Target disk artifact: sample.txt
//   - CRIT-HELLO verifies sample.txt contains "Hello"
//   - CRIT-WORLD verifies sample.txt contains "World"
//   - BLI-HELLO links to CRIT-HELLO and PRI-HW-001
//   - BLI-WORLD links to CRIT-WORLD and PRI-HW-001
//   - REQ-HW-001 links to CRIT-HELLO and CRIT-WORLD, and GOAL-HW-001
//   - TST-HW-001 encapsulates test execution for REQ-HW-001 (1 REQ : 1 TST : N CRIT)
//   - PRI-HW-001 tracks overall priority plan execution with remaining_open_count
//
// 2. Step 0 (RED phase):
//   - Neither word exists.
//   - Run TST-HW-001: both tests fail.
//   - Criteria stay awaiting_verification; TST-HW-001 remains active; remaining_open_count remains 2.
//
// 3. Step 1 (Task 1 execution - "Hello"):
//   - Subagent/worker writes "Hello\n" to sample.txt.
//   - Run TST-HW-001:
//   - CRIT-HELLO turns GREEN -> auto-transitions to complete.
//   - TST-HW-001 decrements remaining_open_count from 2 -> 1.
//   - CRIT-WORLD remains failing (RED) -> awaiting_verification.
//   - TST-HW-001 remains active (gated, does not complete).
//   - REQ-HW-001 remains active.
//   - Complete BLI-HELLO:
//   - CRIT-HELLO is complete, so BLI-HELLO acceptance gate passes.
//   - BLI-HELLO completes -> shockwave CAS-decrements PRI-HW-001 remaining_open_count from 2 -> 1.
//   - PRI-HW-001 remains in_progress.
//
// 4. Step 2 (Task 2 execution - "World"):
//   - Subagent/worker appends "World\n" to sample.txt.
//   - Run TST-HW-001:
//   - CRIT-WORLD turns GREEN -> auto-transitions to complete.
//   - TST-HW-001 decrements remaining_open_count from 1 -> 0.
//   - Terminal Shockwave 1: TST-HW-001 auto-transitions to complete!
//   - Terminal Shockwave 2: REQ-HW-001 auto-transitions to complete!
//   - Terminal Shockwave 3: GOAL-HW-001 auto-transitions to complete!
//   - Complete BLI-WORLD:
//   - CRIT-WORLD is complete, so BLI-WORLD acceptance gate passes.
//   - BLI-WORLD completes -> shockwave CAS-decrements PRI-HW-001 remaining_open_count from 1 -> 0.
//   - Terminal Shockwave 4: PRI-HW-001 auto-transitions to complete!
func TestShockwaveHarness_EndToEnd_HelloWorld(t *testing.T) {
	t.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, "1")
	t.Setenv(zqkenv.TestBypassGitevidence().Key, "1")

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root

	t.Setenv(zqkenv.ProjectRoot().Key, tempDir)

	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Register lifecycle hook handler so CAS updates trigger ApplyDependencyRefEvents and milestone checks
	storage.SetLifecycleHookHandler(func(hCtx context.Context, kind, fromState, toState string, objectData map[string]any) error {
		pRoot := pkgctx.GetLifecycleProjectRoot(hCtx)
		if pRoot == "" {
			pRoot = tempDir
		}
		id, _ := objectData[objects.FieldKeyID].(string)
		if id != "" && pRoot != "" {
			lifecycle.ApplyDependencyRefEvents(hCtx, logging.NewEventLogger(hCtx), sp, pRoot, kind, id, fromState, toState, objectData)
		}
		if kind == objects.KindBacklogItem && toState == objects.ObjectStatusComplete && pRoot != "" {
			milRefs := make([]string, 0)
			milRef, _ := objectData[objects.FieldKeyMilestoneRef].(string)
			if milRef != "" {
				milRefs = append(milRefs, milRef)
			}
			if milRefsRaw, ok := objectData[objects.FieldKeyMilestoneRefs]; ok {
				for _, mID := range lifecycle.StringRefsFromAny(milRefsRaw) {
					if mID != "" && mID != milRef {
						milRefs = append(milRefs, mID)
					}
				}
			}
			for _, mID := range milRefs {
				lifecycle.TryEmitAllBacklogItemsCompleteForMilestone(hCtx, pRoot, mID, func(string) (storage.ObjectStorageProvider, bool) { return sp, true })
				lifecycle.TryEmitRemainingOpenDrained(hCtx, pRoot, mID, func(string) (storage.ObjectStorageProvider, bool) { return sp, true })
			}
		}
		return nil
	})
	t.Cleanup(func() { storage.SetLifecycleHookHandler(nil) })

	// Target verification file
	sampleFile := filepath.Join(tempDir, "sample.txt")

	goalID := "GOAL-E2E-HW-001"
	milID := "MIL-E2E-HW-001"
	priID := "PRI-E2E-HW-001"
	reqID := "REQ-E2E-HW-001"
	tcID := "TST-E2E-HW-001"
	critHelloID := "CRIT-E2E-HELLO-001"
	critWorldID := "CRIT-E2E-WORLD-001"
	bliHelloID := "BLI-E2E-HELLO-001"
	bliWorldID := "BLI-E2E-WORLD-001"

	// 1. Criteria Hello & World (created first so downstream refs can link)
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          critHelloID,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Criteria: sample.txt must contain Hello",
		objects.FieldKeyDescription: "Criteria: sample.txt must contain Hello",
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:    "test",
	}); err != nil {
		t.Fatalf("failed to create critHello: %v", err)
	}

	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          critWorldID,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Criteria: sample.txt must contain World",
		objects.FieldKeyDescription: "Criteria: sample.txt must contain World",
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:    "test",
	}); err != nil {
		t.Fatalf("failed to create critWorld: %v", err)
	}

	// 2. Goal
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "Hello World E2E Goal",
		objects.FieldKeyDescription: "Hello World E2E Goal Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}); err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}

	// 3. Milestone (tied to Goal and linked Backlog Items, not criteria)
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:                 milID,
		objects.FieldKeyKind:               objects.KindMilestone,
		objects.FieldKeyTitle:              "Hello World Milestone",
		objects.FieldKeyDescription:        "Hello World Milestone Description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyGoalRefs:           []string{goalID},
		objects.FieldKeyRemainingOpenCount: 2,
	}); err != nil {
		t.Fatalf("failed to create milestone: %v", err)
	}

	// 4. Priority Plan
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:                 priID,
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyTitle:              "Hello World Priority Plan",
		objects.FieldKeyDescription:        "Hello World Priority Plan Description",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyRelatedObjectRefs:  []string{milID},
		objects.FieldKeyRemainingOpenCount: 2,
	}); err != nil {
		t.Fatalf("failed to create priority plan: %v", err)
	}

	// 5. Backlog Items
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              bliHelloID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Task 1: Generate Hello in sample.txt",
		objects.FieldKeyDescription:     "Task 1 Description",
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: priID,
		objects.FieldKeyMilestoneRef:    milID,
		objects.FieldKeyCriteriaRefs:    []string{critHelloID},
		objects.FieldKeyEstimatedEffort: "1h",
	}); err != nil {
		t.Fatalf("failed to create bliHello: %v", err)
	}

	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              bliWorldID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Task 2: Append World in sample.txt",
		objects.FieldKeyDescription:     "Task 2 Description",
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: priID,
		objects.FieldKeyMilestoneRef:    milID,
		objects.FieldKeyCriteriaRefs:    []string{critWorldID},
		objects.FieldKeyEstimatedEffort: "1h",
	}); err != nil {
		t.Fatalf("failed to create bliWorld: %v", err)
	}

	// 6. Requirement (links to Goal and both Criteria)
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          reqID,
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Requirement: Hello World Output Verification",
		objects.FieldKeyDescription: "Requirement Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:    []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{
			critHelloID,
			critWorldID,
		},
	}); err != nil {
		t.Fatalf("failed to create requirement: %v", err)
	}

	// 7. Test Case (1 REQ : 1 TST : N CRIT)
	// Uses grep command checking sample.txt
	cmdHello := "grep -q 'Hello' '" + sampleFile + "'"
	cmdWorld := "grep -q 'World' '" + sampleFile + "'"

	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyTitle:           "Test Suite: Hello World Validation",
		objects.FieldKeyDescription:     "Test Suite Description",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyBacklogItemRefs: []string{bliHelloID, bliWorldID},
		objects.FieldKeyCriteriaRefs:    []string{critHelloID, critWorldID},
		objects.FieldKeyVerificationSuites: []string{
			critHelloID + ": " + cmdHello,
			critWorldID + ": " + cmdWorld,
		},
	}); err != nil {
		t.Fatalf("failed to create test case: %v", err)
	}

	// Seed remaining_open_count on test_case: should be 2
	if err := lifecycle.SeedRemainingOpenCountFromMembers(ctx, sp, tcID, true); err != nil {
		t.Fatalf("failed to seed test case remaining_open_count: %v", err)
	}

	tcObj, err := sp.Read(ctx, secCtx, tcID)
	if err != nil {
		t.Fatalf("failed to read test case: %v", err)
	}
	if rem, _ := tcObj[objects.FieldKeyRemainingOpenCount].(int); rem != 2 {
		t.Fatalf("expected initial test_case remaining_open_count=2, got %d", rem)
	}

	// =========================================================================
	// PHASE 0: RED State (Objective Verification: file does not exist yet)
	// =========================================================================
	t.Log("=== Phase 0: Executing RED test phase ===")
	res0, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, testrunner.RunOptions{})
	if err != nil {
		t.Fatalf("test runner failed during Phase 0: %v", err)
	}
	if res0.PassedCriteria != 0 || res0.FailedCriteria != 2 {
		t.Fatalf("Phase 0 RED expected 0 pass and 2 fail, got passed=%d, failed=%d", res0.PassedCriteria, res0.FailedCriteria)
	}

	// Verify criteria remain awaiting_verification
	cHello, _ := sp.Read(ctx, secCtx, critHelloID)
	if st, _ := cHello[objects.FieldKeyStatus].(string); st != "awaiting_verification" {
		t.Errorf("expected critHello awaiting_verification, got %s", st)
	}
	cWorld, _ := sp.Read(ctx, secCtx, critWorldID)
	if st, _ := cWorld[objects.FieldKeyStatus].(string); st != "awaiting_verification" {
		t.Errorf("expected critWorld awaiting_verification, got %s", st)
	}

	// Verify test case remains active with count=2
	tcObj, _ = sp.Read(ctx, secCtx, tcID)
	if st, _ := tcObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusActive {
		t.Errorf("expected test case active, got %s", st)
	}
	if rem, _ := tcObj[objects.FieldKeyRemainingOpenCount].(int); rem != 2 {
		t.Errorf("expected test case remaining_open_count=2, got %d", rem)
	}

	// =========================================================================
	// PHASE 1: Task 1 (Agent generates "Hello" in sample.txt)
	// =========================================================================
	t.Log("=== Phase 1: Task 1 - Writing Hello to sample.txt ===")
	if err := os.WriteFile(sampleFile, []byte("Hello\n"), 0o600); err != nil {
		t.Fatalf("failed to write sample.txt: %v", err)
	}

	// Run test runner
	res1, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, testrunner.RunOptions{})
	if err != nil {
		t.Fatalf("test runner failed during Phase 1: %v", err)
	}
	if res1.PassedCriteria != 1 || res1.FailedCriteria != 1 {
		t.Fatalf("Phase 1 expected 1 pass and 1 fail, got passed=%d, failed=%d", res1.PassedCriteria, res1.FailedCriteria)
	}

	// Verify CRIT-HELLO transitioned to complete
	cHello, _ = sp.Read(ctx, secCtx, critHelloID)
	if st, _ := cHello[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Errorf("expected critHello complete, got %s", st)
	}

	// Verify CRIT-WORLD still awaiting_verification
	cWorld, _ = sp.Read(ctx, secCtx, critWorldID)
	if st, _ := cWorld[objects.FieldKeyStatus].(string); st != "awaiting_verification" {
		t.Errorf("expected critWorld awaiting_verification, got %s", st)
	}

	// Verify test case remaining_open_count decremented 2 -> 1
	tcObj, _ = sp.Read(ctx, secCtx, tcID)
	if rem, _ := tcObj[objects.FieldKeyRemainingOpenCount].(int); rem != 1 {
		t.Errorf("expected test case remaining_open_count=1, got %d", rem)
	}
	if st, _ := tcObj[objects.FieldKeyStatus].(string); st == objects.ObjectStatusComplete {
		t.Fatalf("test case must NOT be complete while 1 criterion is still pending")
	}

	// Verify requirement remains active
	reqObj, _ := sp.Read(ctx, secCtx, reqID)
	if st, _ := reqObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusActive {
		t.Errorf("expected requirement active, got %s", st)
	}

	// Verify milestone remains in_progress (gated, 1 criterion still pending)
	milObj, _ := sp.Read(ctx, secCtx, milID)
	if st, _ := milObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusInProgress {
		t.Errorf("expected milestone in_progress, got %s", st)
	}

	// Complete BLI-HELLO (simulating Task 1 completion by worker agent)
	t.Log("=== Completing BLI-HELLO ===")
	bliHelloUpdates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	bliCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "complete BLI-HELLO")
	if err := sp.Update(bliCtx, secCtx, bliHelloID, bliHelloUpdates); err != nil {
		t.Fatalf("failed to update BLI-HELLO to complete: %v", err)
	}

	// Verify BLI-HELLO is complete
	bHelloObj, _ := sp.Read(ctx, secCtx, bliHelloID)
	if st, _ := bHelloObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Errorf("expected bliHello complete, got %s", st)
	}

	// Verify PRI-HW-001 remaining_open_count decremented 2 -> 1, status remains in_progress
	priObj, _ := sp.Read(ctx, secCtx, priID)
	if rem, _ := priObj[objects.FieldKeyRemainingOpenCount].(int); rem != 1 {
		t.Errorf("expected priority plan remaining_open_count=1, got %d", rem)
	}
	if st, _ := priObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusInProgress {
		t.Errorf("expected priority plan in_progress, got %s", st)
	}

	// =========================================================================
	// PHASE 2: Task 2 (Agent appends "World" to sample.txt)
	// =========================================================================
	t.Log("=== Phase 2: Task 2 - Appending World to sample.txt ===")
	//nolint:gosec
	f, err := os.OpenFile(sampleFile, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open sample.txt for appending: %v", err)
	}
	if _, err := f.WriteString("World\n"); err != nil {
		_ = f.Close()
		t.Fatalf("failed to append World to sample.txt: %v", err)
	}
	_ = f.Close()

	// Run test runner
	res2, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, testrunner.RunOptions{})
	if err != nil {
		t.Fatalf("test runner failed during Phase 2: %v", err)
	}
	if res2.PassedCriteria != 2 {
		t.Fatalf("Phase 2 expected all 2 criteria passed, got %d", res2.PassedCriteria)
	}

	// Verify CRIT-WORLD transitioned to complete
	cWorld, _ = sp.Read(ctx, secCtx, critWorldID)
	if st, _ := cWorld[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Errorf("expected critWorld complete, got %s", st)
	}

	// Verify SHOCKWAVE 1: Test Case auto-completed and remaining_open_count=0
	tcObj, _ = sp.Read(ctx, secCtx, tcID)
	if rem, _ := tcObj[objects.FieldKeyRemainingOpenCount].(int); rem != 0 {
		t.Errorf("expected test case remaining_open_count=0, got %d", rem)
	}
	if st, _ := tcObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Fatalf("expected test case complete, got %s", st)
	}

	// Verify SHOCKWAVE 2: Requirement auto-completed
	reqObj, _ = sp.Read(ctx, secCtx, reqID)
	if st, _ := reqObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Fatalf("expected requirement complete via shockwave, got %s", st)
	}

	// Verify SHOCKWAVE 3: Goal auto-completed
	goalObj, _ := sp.Read(ctx, secCtx, goalID)
	if st, _ := goalObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Fatalf("expected goal complete via shockwave, got %s", st)
	}

	// Complete BLI-WORLD (simulating Task 2 completion by worker agent)
	t.Log("=== Completing BLI-WORLD ===")
	bliWorldUpdates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	if err := sp.Update(bliCtx, secCtx, bliWorldID, bliWorldUpdates); err != nil {
		t.Fatalf("failed to update BLI-WORLD to complete: %v", err)
	}

	// Verify BLI-WORLD is complete
	bWorldObj, _ := sp.Read(ctx, secCtx, bliWorldID)
	if st, _ := bWorldObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Errorf("expected bliWorld complete, got %s", st)
	}

	// Verify SHOCKWAVE 4 & 5: Priority Plan remaining_open_count=0 and auto-completed!
	priObj, _ = sp.Read(ctx, secCtx, priID)
	if rem, _ := priObj[objects.FieldKeyRemainingOpenCount].(int); rem != 0 {
		t.Errorf("expected priority plan remaining_open_count=0, got %d", rem)
	}
	if st, _ := priObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Fatalf("expected priority plan complete via shockwave, got %s", st)
	}

	// Verify SHOCKWAVE 6: Milestone auto-completed!
	milObj, _ = sp.Read(ctx, secCtx, milID)
	if st, _ := milObj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusComplete {
		t.Fatalf("expected milestone complete via shockwave, got %s", st)
	}

	t.Log("=== End-to-End Shockwave State Machine Verification SUCCESSFUL ===")
}
