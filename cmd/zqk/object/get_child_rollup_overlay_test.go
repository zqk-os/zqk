package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestGetChildRollupOverlay_MilestoneAndPriorityPlan(t *testing.T) {
	storage.SetCacheOperationHandler(func(*pkgctx.CacheContext) error { return nil })
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	bliFields, err := fieldRegistry.GetFieldsForKind(objects.KindBacklogItem)
	if err != nil {
		t.Fatalf("failed to get backlog_item fields: %v", err)
	}
	milestoneFields, err := fieldRegistry.GetFieldsForKind(objects.KindMilestone)
	if err != nil {
		t.Fatalf("failed to get milestone fields: %v", err)
	}
	planFields, err := fieldRegistry.GetFieldsForKind(objects.KindPriorityPlan)
	if err != nil {
		t.Fatalf("failed to get priority_plan fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	critFields, err := fieldRegistry.GetFieldsForKind(objects.KindCriteria)
	if err != nil {
		t.Fatalf("failed to get criteria fields: %v", err)
	}
	critID := "CRIT-ROLLUP-001"
	crit := createTestObject(objects.KindCriteria, critID, critFields, 0)
	crit[objects.FieldKeyStatus] = objects.ObjectStatusComplete
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, crit, objects.ObjectStatusComplete)

	milestoneID := "MIL-ROLLUP-001"
	mil := createTestObject(objects.KindMilestone, milestoneID, milestoneFields, 0)
	mil[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, mil, objects.ObjectStatusInProgress)

	planID := "PRI-ROLLUP-001"
	plan := createTestObject(objects.KindPriorityPlan, planID, planFields, 0)
	plan[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, plan, objects.ObjectStatusInProgress)

	// Backlog items for milestone
	bli1ID := "BLI-ROLLUP-001"
	bli1 := createTestObject(objects.KindBacklogItem, bli1ID, bliFields, 0)
	bli1[objects.FieldKeyMilestoneRef] = milestoneID
	bli1[objects.FieldKeyCriteriaRefs] = []string{critID}
	bli1[objects.FieldKeyStatus] = objects.ObjectStatusComplete
	bli1[objects.FieldKeyStartedAt] = "2026-09-08T00:00:00Z"
	bli1[objects.FieldKeyCompletedAt] = "2026-09-08T10:00:00Z"
	bli1[objects.FieldKeyEstimatedEffort] = "4h"
	bli1[objects.FieldKeyActualEffort] = "3h"
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, bli1, objects.ObjectStatusComplete)

	bli2ID := "BLI-ROLLUP-002"
	bli2 := createTestObject(objects.KindBacklogItem, bli2ID, bliFields, 1)
	bli2[objects.FieldKeyMilestoneRef] = milestoneID
	bli2[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	bli2[objects.FieldKeyStartedAt] = "2026-09-08T00:00:00Z"
	bli2[objects.FieldKeyUpdatedAt] = "2026-09-08T10:00:00Z"
	bli2[objects.FieldKeyEstimatedEffort] = "6h"
	bli2[objects.FieldKeyActualEffort] = "2h"
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, bli2, objects.ObjectStatusInProgress)

	// Backlog items for priority plan
	bli3ID := "BLI-ROLLUP-003"
	bli3 := createTestObject(objects.KindBacklogItem, bli3ID, bliFields, 2)
	bli3[objects.FieldKeyPriorityPlanRef] = planID
	bli3[objects.FieldKeyCriteriaRefs] = []string{critID}
	bli3[objects.FieldKeyStatus] = objects.ObjectStatusComplete
	bli3[objects.FieldKeyStartedAt] = "2026-09-08T00:00:00Z"
	bli3[objects.FieldKeyCompletedAt] = "2026-09-08T20:00:00Z"
	bli3[objects.FieldKeyEstimatedEffort] = "1d" // 24h
	bli3[objects.FieldKeyActualEffort] = "12h"
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, bli3, objects.ObjectStatusComplete)

	bli4ID := "BLI-ROLLUP-004"
	bli4 := createTestObject(objects.KindBacklogItem, bli4ID, bliFields, 3)
	bli4[objects.FieldKeyPriorityPlanRef] = planID
	bli4[objects.FieldKeyCriteriaRefs] = []string{critID}
	bli4[objects.FieldKeyStatus] = objects.ObjectStatusComplete
	bli4[objects.FieldKeyStartedAt] = "2026-09-08T00:00:00Z"
	bli4[objects.FieldKeyCompletedAt] = "2026-09-08T20:00:00Z"
	bli4[objects.FieldKeyEstimatedEffort] = "1d" // 24h
	bli4[objects.FieldKeyActualEffort] = "18h"
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, bli4, objects.ObjectStatusComplete)

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{objects.KindCriteria, objects.KindMilestone, objects.KindPriorityPlan, objects.KindBacklogItem}); err != nil {
		t.Fatalf("ensure visible: %v", err)
	}

	// 1. Check milestone get
	cmdMil := testEnv.CreateCLICommand("object", "get", milestoneID, "--format", "json")
	outMil, err := cmdMil.CombinedOutput()
	if err != nil {
		t.Fatalf("object get milestone failed: %v, out=%s", err, string(outMil))
	}
	var gotMil map[string]any
	if err := json.Unmarshal(outMil, &gotMil); err != nil {
		t.Fatalf("unmarshal milestone: %v", err)
	}

	rollupMil, ok := gotMil["child_rollup"].(map[string]any)
	if !ok {
		t.Fatalf("expected child_rollup map on milestone; got=%T", gotMil["child_rollup"])
	}
	if rollupMil["total_count"] != float64(2) {
		t.Fatalf("milestone total_count=%v want 2", rollupMil["total_count"])
	}
	if rollupMil["completed_count"] != float64(1) {
		t.Fatalf("milestone completed_count=%v want 1", rollupMil["completed_count"])
	}
	if rollupMil["percent_complete"] != float64(50) {
		t.Fatalf("milestone percent_complete=%v want 50", rollupMil["percent_complete"])
	}
	milEst := rollupMil["estimated_effort"].(map[string]any)
	if milEst["total_hours"] != float64(10) {
		t.Fatalf("milestone estimated total_hours=%v want 10", milEst["total_hours"])
	}
	milAct := rollupMil["actual_effort"].(map[string]any)
	if milAct["total_hours"] != float64(5) {
		t.Fatalf("milestone actual total_hours=%v want 5", milAct["total_hours"])
	}
	if gotMil["milestone_complete"] != false {
		t.Fatalf("expected milestone_complete=false")
	}
	if gotMil["milestone_percent_complete"] != float64(50) {
		t.Fatalf("expected milestone_percent_complete=50")
	}

	// 2. Check priority plan get
	cmdPlan := testEnv.CreateCLICommand("object", "get", planID, "--format", "json")
	outPlan, err := cmdPlan.CombinedOutput()
	if err != nil {
		t.Fatalf("object get plan failed: %v, out=%s", err, string(outPlan))
	}
	var gotPlan map[string]any
	if err := json.Unmarshal(outPlan, &gotPlan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	rollupPlan, ok := gotPlan["child_rollup"].(map[string]any)
	if !ok {
		t.Fatalf("expected child_rollup map on plan; got=%T", gotPlan["child_rollup"])
	}
	if rollupPlan["total_count"] != float64(2) {
		t.Fatalf("plan total_count=%v want 2", rollupPlan["total_count"])
	}
	if rollupPlan["completed_count"] != float64(2) {
		t.Fatalf("plan completed_count=%v want 2", rollupPlan["completed_count"])
	}
	if rollupPlan["percent_complete"] != float64(100) {
		t.Fatalf("plan percent_complete=%v want 100", rollupPlan["percent_complete"])
	}
	planEst := rollupPlan["estimated_effort"].(map[string]any)
	if planEst["total_hours"] != float64(48) {
		t.Fatalf("plan estimated total_hours=%v want 48", planEst["total_hours"])
	}
	planAct := rollupPlan["actual_effort"].(map[string]any)
	if planAct["total_hours"] != float64(30) {
		t.Fatalf("plan actual total_hours=%v want 30", planAct["total_hours"])
	}
	if gotPlan["priority_plan_complete"] != true {
		t.Fatalf("expected priority_plan_complete=true")
	}
	if gotPlan["priority_plan_percent_complete"] != float64(100) {
		t.Fatalf("expected priority_plan_percent_complete=100")
	}
}
