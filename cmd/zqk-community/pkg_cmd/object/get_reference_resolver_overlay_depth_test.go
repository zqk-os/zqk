package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestGetReferenceResolverOverlay_DefaultView_Depth2ResolvesNestedRefs(t *testing.T) {
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
	milestoneFields, err := fieldRegistry.GetFieldsForKind("milestone")
	if err != nil {
		t.Fatalf("failed to get milestone fields: %v", err)
	}
	requirementFields, err := fieldRegistry.GetFieldsForKind("requirement")
	if err != nil {
		t.Fatalf("failed to get requirement fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	goalID := "GOAL-020000"
	criteriaID := "CRIT-020000"
	milestoneID := "MIL-020000"
	requirementID := "REQ-020000"

	goalObj := createTestObject("goal", goalID, goalFields, 0)
	goalObj[objects.FieldKeyStatus] = objectStatusActive
	if err := fs.Create(cliCtx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	milestoneObj := createTestObject("milestone", milestoneID, milestoneFields, 0)
	milestoneObj[objects.FieldKeyStatus] = objectStatusComplete
	if err := fs.Create(cliCtx, secCtx, milestoneObj); err != nil {
		t.Fatalf("create milestone: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"milestone"}); err != nil {
		t.Fatalf("ensure milestone visible: %v", err)
	}

	criteriaObj := createTestObject("criteria", criteriaID, criteriaFields, 0)
	criteriaObj[objects.FieldKeyStatus] = objectStatusComplete
	criteriaObj[objects.FieldKeyMilestoneRefs] = []string{milestoneID}
	if err := fs.Create(cliCtx, secCtx, criteriaObj); err != nil {
		t.Fatalf("create criteria: %v", err)
	}

	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"criteria"}); err != nil {
		t.Fatalf("ensure criteria visible: %v", err)
	}

	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"goal"}); err != nil {
		t.Fatalf("ensure goal visible: %v", err)
	}

	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"goal", "milestone", "criteria"}); err != nil {
		t.Fatalf("ensure referenced objects visible: %v", err)
	}

	requirementObj := createTestObject("requirement", requirementID, requirementFields, 0)
	requirementObj[objects.FieldKeyGoalRefs] = []string{goalID}
	requirementObj[objects.FieldKeyCriteriaRefs] = []string{criteriaID}
	if err := fs.Create(cliCtx, secCtx, requirementObj); err != nil {
		t.Fatalf("create requirement: %v", err)
	}
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"requirement"}); err != nil {
		t.Fatalf("ensure requirement visible: %v", err)
	}

	cmd := testEnv.CreateCLICommand("object", "get", requirementID, "--format", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get failed: %v output=%s", err, string(out))
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal get output: %v; output=%s", err, string(out))
	}

	resolvedCriteria, ok := got["resolved_criteria_refs"].([]any)
	if !ok {
		t.Fatalf("expected resolved_criteria_refs as array; got=%T", got["resolved_criteria_refs"])
	}
	if len(resolvedCriteria) != 1 {
		t.Fatalf("expected resolved_criteria_refs length=1; got=%d", len(resolvedCriteria))
	}

	firstCrit, _ := resolvedCriteria[0].(map[string]any)
	resolvedMilestones, ok := firstCrit["resolved_milestone_refs"].([]any)
	if !ok {
		t.Fatalf("expected nested resolved_milestone_refs on criteria embed; got=%T", firstCrit["resolved_milestone_refs"])
	}
	if len(resolvedMilestones) != 1 {
		t.Fatalf("expected resolved_milestone_refs length=1; got=%d", len(resolvedMilestones))
	}
	firstMilestone, _ := resolvedMilestones[0].(map[string]any)
	if firstMilestone[objects.FieldKeyID] != milestoneID {
		t.Fatalf("expected nested milestone id=%s; got=%v", milestoneID, firstMilestone[objects.FieldKeyID])
	}
	if firstMilestone[objects.FieldKeyStatus] != objectStatusComplete {
		t.Fatalf("expected nested milestone status=%s; got=%v", objectStatusComplete, firstMilestone[objects.FieldKeyStatus])
	}
}
