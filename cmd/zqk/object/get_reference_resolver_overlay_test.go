package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestGetReferenceResolverOverlay_RequirementResolvesGoalAndCriteriaRefs(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	goalFields, err := fieldRegistry.GetFieldsForKind("goal")
	if err != nil {
		t.Fatalf("failed to get goal fields: %v", err)
	}
	criteriaFields, err := fieldRegistry.GetFieldsForKind("criteria")
	if err != nil {
		t.Fatalf("failed to get criteria fields: %v", err)
	}
	requirementFields, err := fieldRegistry.GetFieldsForKind("requirement")
	if err != nil {
		t.Fatalf("failed to get requirement fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	goalID := "GOAL-010000"
	criteriaID := "CRIT-010000"
	requirementID := "REQ-010000"

	goalObj := createTestObject("goal", goalID, goalFields, 0)
	goalObj[objects.FieldKeyStatus] = objectStatusActive
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, goalObj, casLeaveStatus(objects.GetString(goalObj, objects.FieldKeyStatus)))

	criteriaObj := createTestObject("criteria", criteriaID, criteriaFields, 0)
	criteriaObj[objects.FieldKeyStatus] = objectStatusComplete
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, criteriaObj, casLeaveStatus(objects.GetString(criteriaObj, objects.FieldKeyStatus)))

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"goal", "criteria"}); err != nil {
		t.Fatalf("ensure referenced objects visible: %v", err)
	}

	requirementObj := createTestObject("requirement", requirementID, requirementFields, 0)
	requirementObj[objects.FieldKeyGoalRefs] = []string{goalID}
	requirementObj[objects.FieldKeyCriteriaRefs] = []string{criteriaID}
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, requirementObj, casLeaveStatus(objects.GetString(requirementObj, objects.FieldKeyStatus)))
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"requirement"}); err != nil {
		t.Fatalf("ensure requirement visible: %v", err)
	}

	cmd := testEnv.CreateCLICommand("object", "get", requirementID, "--format", "json", "--link-hydration", "lazy")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get failed: %v output=%s", err, string(out))
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal get output: %v; output=%s", err, string(out))
	}

	if got["reference_resolver_overlay_applied"] != true {
		t.Fatalf("expected reference_resolver_overlay_applied=true; got=%v", got["reference_resolver_overlay_applied"])
	}

	resolvedGoals, ok := got["resolved_goal_refs"].([]any)
	if !ok {
		t.Fatalf("expected resolved_goal_refs as array; got=%T", got["resolved_goal_refs"])
	}
	if len(resolvedGoals) != 1 {
		t.Fatalf("expected resolved_goal_refs length=1; got=%d", len(resolvedGoals))
	}
	firstGoal, _ := resolvedGoals[0].(map[string]any)
	if firstGoal[objects.FieldKeyID] != goalID {
		t.Fatalf("expected resolved goal id=%s; got=%v", goalID, firstGoal[objects.FieldKeyID])
	}
	if firstGoal[objects.FieldKeyStatus] != objectStatusActive {
		t.Fatalf("expected resolved goal status=%s; got=%v", objectStatusActive, firstGoal[objects.FieldKeyStatus])
	}
	if firstGoal[objects.FieldKeyTitle] == emptyValue {
		t.Fatalf("expected resolved goal title non-empty")
	}

	resolvedCriteria, ok := got["resolved_criteria_refs"].([]any)
	if !ok {
		t.Fatalf("expected resolved_criteria_refs as array; got=%T", got["resolved_criteria_refs"])
	}
	if len(resolvedCriteria) != 1 {
		t.Fatalf("expected resolved_criteria_refs length=1; got=%d", len(resolvedCriteria))
	}
	firstCrit, _ := resolvedCriteria[0].(map[string]any)
	if firstCrit[objects.FieldKeyID] != criteriaID {
		t.Fatalf("expected resolved criteria id=%s; got=%v", criteriaID, firstCrit[objects.FieldKeyID])
	}
	if firstCrit[objects.FieldKeyStatus] != objectStatusComplete {
		t.Fatalf("expected resolved criteria status=%s; got=%v", objectStatusComplete, firstCrit[objects.FieldKeyStatus])
	}
	if firstCrit[objects.FieldKeyTitle] == emptyValue {
		t.Fatalf("expected resolved criteria title non-empty")
	}
}
