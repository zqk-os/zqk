package workflow

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/object"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestWorkflowAddCommand(t *testing.T) {
	t.Parallel()
	env := object.SetupTestEnvironment(t)
	cliBinary := env.CLIBinary
	tmpDir := env.TestRoot

	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Create a goal
	goalID := "GOAL-WORKFLOW-ADD"
	goal := map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        "goal",
		objects.FieldKeyTitle:       "Workflow Add Goal Test",
		objects.FieldKeyDescription: "Verify workflow add command goal requirement",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyNamespaceID: "zqk:kernel",
	}
	if err := storageProvider.Create(pkgctx.NewSystemContext(), secCtx, goal); err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}

	// 2. Create a backlog item in exploring status
	bliID := "ITEM-WORKFLOW-ADD"
	bli := map[string]any{
		objects.FieldKeyID:          bliID,
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyTitle:       "Workflow Add BLI Test",
		objects.FieldKeyDescription: "Verify workflow add command linkage",
		objects.FieldKeyStatus:      objects.ObjectStatusExploring,
		objects.FieldKeyGoalRefs:    []any{goalID},
		objects.FieldKeyNamespaceID: "zqk:kernel",
	}
	if err := storageProvider.Create(pkgctx.NewSystemContext(), secCtx, bli); err != nil {
		t.Fatalf("failed to create backlog item: %v", err)
	}

	// Flush indexes to make sure they are visible
	_ = storage.GetGlobalListingIndexWriteQueue().FlushKind("goal", 2*time.Second)
	_ = storage.GetGlobalListingIndexWriteQueue().FlushKind("backlog_item", 2*time.Second)

	// Shutdown the in-process storage before spawning the CLI command to avoid file locks
	shutCtx, cancelShut := context.WithTimeout(context.Background(), 10*time.Second)
	if err := storageProvider.Shutdown(shutCtx); err != nil {
		t.Fatalf("failed to shutdown in-process storage: %v", err)
	}
	cancelShut()

	t.Run("Workflow add automatically scaffolds priority plan and milestone", func(t *testing.T) {
		cmd := exec.Command(cliBinary, "workflow", "add", bliID, "3", "--override", "--reason-code", "this is a test override reason code for testing bypasses")
		zqkenv.WireExecForIsolatedProject(cmd, tmpDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("workflow add command failed: %v\nOutput: %s", err, string(output))
		}

		// Verify output message shows scaffolding and successful addition
		outStr := string(output)
		if !strings.Contains(outStr, "Scaffolding new plan") {
			t.Errorf("expected output to contain 'Scaffolding new plan', got: %s", outStr)
		}
		if !strings.Contains(outStr, "added to plan") {
			t.Errorf("expected output to contain 'added to plan', got: %s", outStr)
		}

		// Recreate storage to verify updates
		verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
		if err != nil {
			t.Fatalf("failed to recreate storage: %v", err)
		}
		defer func() {
			_ = verifyStorage.Shutdown(context.Background())
		}()

		updatedBli, err := verifyStorage.Read(pkgctx.NewSystemContext(), secCtx, bliID)
		if err != nil {
			t.Fatalf("failed to read backlog item: %v", err)
		}

		// Verify plan ref starts with PLAN-AUTO-
		planRef, _ := updatedBli[objects.FieldKeyPriorityPlanRef].(string)
		if !strings.HasPrefix(planRef, "PLAN-AUTO-") {
			t.Errorf("expected priority_plan_ref starting with PLAN-AUTO-, got %s", planRef)
		}

		// Verify milestone ref was auto-resolved and starts with MIL-AUTO-
		mRefsRaw, ok := updatedBli[objects.FieldKeyMilestoneRefs]
		if !ok || mRefsRaw == nil {
			t.Fatalf("expected milestone_refs list to be present")
		}
		mRefs, _ := mRefsRaw.([]any)
		if len(mRefs) != 1 || !strings.HasPrefix(mRefs[0].(string), "MIL-AUTO-") {
			t.Errorf("expected milestone_refs to contain auto-scaffolded milestone, got %v", mRefs)
		}

		// Verify priority was defaulted to medium
		priority, _ := updatedBli[objects.FieldKeyPriority].(string)
		if priority != "medium" {
			t.Errorf("expected priority = medium, got %s", priority)
		}

		// Verify status toggled to planned
		status, _ := updatedBli[objects.FieldKeyStatus].(string)
		if status != "planned" {
			t.Errorf("expected status = planned, got %s", status)
		}
	})

	t.Run("Workflow add links to existing priority plan", func(t *testing.T) {
		// 1. Re-open/recreate storage to set up a new plan and backlog item
		storageProvider, err := storage.NewFileObjectStorage(tmpDir)
		if err != nil {
			t.Fatalf("failed to recreate storage: %v", err)
		}

		planID := "PLAN-EXISTING-ADD"
		plan := map[string]any{
			objects.FieldKeyID:          planID,
			objects.FieldKeyKind:        "priority_plan",
			objects.FieldKeyTitle:       "Existing Plan Test",
			objects.FieldKeyDescription: "Verify workflow add links to existing plan",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyNamespaceID: "zqk:kernel",
		}
		if err := storageProvider.Create(pkgctx.NewSystemContext(), secCtx, plan); err != nil {
			t.Fatalf("failed to create priority plan: %v", err)
		}

		milestoneID := "MIL-EXISTING-ADD"
		milestone := map[string]any{
			objects.FieldKeyID:          milestoneID,
			objects.FieldKeyKind:        "milestone",
			objects.FieldKeyTitle:       "Existing Milestone Test",
			objects.FieldKeyDescription: "Verify milestone links",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			"priority_plan_refs":        []any{planID},
			objects.FieldKeyNamespaceID: "zqk:kernel",
		}
		if err := storageProvider.Create(pkgctx.NewSystemContext(), secCtx, milestone); err != nil {
			t.Fatalf("failed to create milestone: %v", err)
		}

		bliID2 := "ITEM-WORKFLOW-ADD2"
		bli := map[string]any{
			objects.FieldKeyID:          bliID2,
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyTitle:       "Workflow Add BLI Test 2",
			objects.FieldKeyDescription: "Verify linkage to existing plan",
			objects.FieldKeyStatus:      objects.ObjectStatusExploring,
			objects.FieldKeyGoalRefs:    []any{goalID},
			objects.FieldKeyNamespaceID: "zqk:kernel",
		}
		if err := storageProvider.Create(pkgctx.NewSystemContext(), secCtx, bli); err != nil {
			t.Fatalf("failed to create backlog item: %v", err)
		}

		_ = storage.GetGlobalListingIndexWriteQueue().FlushKind("priority_plan", 2*time.Second)
		_ = storage.GetGlobalListingIndexWriteQueue().FlushKind("milestone", 2*time.Second)
		_ = storage.GetGlobalListingIndexWriteQueue().FlushKind("backlog_item", 2*time.Second)

		shutCtx, cancelShut := context.WithTimeout(context.Background(), 10*time.Second)
		if err := storageProvider.Shutdown(shutCtx); err != nil {
			t.Fatalf("failed to shutdown storage: %v", err)
		}
		cancelShut()

		cmd := exec.Command(cliBinary, "workflow", "add", bliID2, planID, "--override", "--reason-code", "this is a test override reason code for testing bypasses")
		zqkenv.WireExecForIsolatedProject(cmd, tmpDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("workflow add command failed: %v\nOutput: %s", err, string(output))
		}

		verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
		if err != nil {
			t.Fatalf("failed to recreate storage: %v", err)
		}
		defer func() {
			_ = verifyStorage.Shutdown(context.Background())
		}()

		updatedBli, err := verifyStorage.Read(pkgctx.NewSystemContext(), secCtx, bliID2)
		if err != nil {
			t.Fatalf("failed to read backlog item: %v", err)
		}

		planRef, _ := updatedBli[objects.FieldKeyPriorityPlanRef].(string)
		if planRef != planID {
			t.Errorf("expected priority_plan_ref = %s, got %s", planID, planRef)
		}

		mRefsRaw, ok := updatedBli[objects.FieldKeyMilestoneRefs]
		if !ok || mRefsRaw == nil {
			t.Fatalf("expected milestone_refs list to be present")
		}
		mRefs, _ := mRefsRaw.([]any)
		if len(mRefs) != 1 || mRefs[0].(string) != milestoneID {
			t.Errorf("expected milestone_refs to contain %s, got %v", milestoneID, mRefs)
		}
	})
}
