package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

// seedRequirementCriteriaMilestoneGraph creates goal → criteria(with milestone_refs) → requirement
// used by overlay depth tests.
func seedRequirementCriteriaMilestoneGraph(t *testing.T, testEnv *TestEnvironment) (requirementID, criteriaID, milestoneID string) {
	t.Helper()

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
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	goalID := "GOAL-020000"
	criteriaID = "CRIT-020000"
	milestoneID = "MIL-020000"
	requirementID = "REQ-020000"

	goalObj := createTestObject("goal", goalID, goalFields, 0)
	goalObj[objects.FieldKeyStatus] = objectStatusActive
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, goalObj, casLeaveStatus(objects.GetString(goalObj, objects.FieldKeyStatus)))

	milestoneObj := createTestObject("milestone", milestoneID, milestoneFields, 0)
	milestoneObj[objects.FieldKeyStatus] = objectStatusComplete
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, milestoneObj, casLeaveStatus(objects.GetString(milestoneObj, objects.FieldKeyStatus)))

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"milestone"}); err != nil {
		t.Fatalf("ensure milestone visible: %v", err)
	}

	criteriaObj := createTestObject("criteria", criteriaID, criteriaFields, 0)
	criteriaObj[objects.FieldKeyStatus] = objectStatusComplete
	criteriaObj[objects.FieldKeyMilestoneRefs] = []string{milestoneID}
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, criteriaObj, casLeaveStatus(objects.GetString(criteriaObj, objects.FieldKeyStatus)))

	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"criteria", "goal"}); err != nil {
		t.Fatalf("ensure criteria/goal visible: %v", err)
	}

	requirementObj := createTestObject("requirement", requirementID, requirementFields, 0)
	requirementObj[objects.FieldKeyGoalRefs] = []string{goalID}
	requirementObj[objects.FieldKeyCriteriaRefs] = []string{criteriaID}
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, requirementObj, casLeaveStatus(objects.GetString(requirementObj, objects.FieldKeyStatus)))
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"requirement"}); err != nil {
		t.Fatalf("ensure requirement visible: %v", err)
	}
	return requirementID, criteriaID, milestoneID
}

func parseObjectGetJSON(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal get output: %v; output=%s", err, string(out))
	}
	return got
}

// TestGetReferenceResolverOverlay_DefaultView_Depth2ResolvesNestedRefs keeps the historical
// name for scheduler/bundle identity. Nested depth-2 resolution is opt-in via
// --link-hydration default (ViewDefault without the flag is one-hop / lazy).
func TestGetReferenceResolverOverlay_DefaultView_Depth2ResolvesNestedRefs(t *testing.T) {
	testEnv := SetupTestEnvironment(t)
	requirementID, _, milestoneID := seedRequirementCriteriaMilestoneGraph(t, testEnv)

	cmd := testEnv.CreateCLICommand("object", "get", requirementID, "--format", "json", "--link-hydration", "default")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get --link-hydration default failed: %v output=%s", err, string(out))
	}

	got := parseObjectGetJSON(t, out)
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

// TestGetReferenceResolverOverlay_UnspecifiedHydration_IsRaw locks ViewDefault product
// contract: omit --link-hydration ⇒ no resolved_* embeds (CAS seal / raw get).
// TRACK: BLI-REDACTED
func TestGetReferenceResolverOverlay_UnspecifiedHydration_IsRaw(t *testing.T) {
	testEnv := SetupTestEnvironment(t)
	requirementID, _, _ := seedRequirementCriteriaMilestoneGraph(t, testEnv)

	cmd := testEnv.CreateCLICommand("object", "get", requirementID, "--format", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get (default hydration) failed: %v output=%s", err, string(out))
	}

	got := parseObjectGetJSON(t, out)
	if _, ok := got["resolved_criteria_refs"]; ok {
		t.Fatalf("default get must be raw: resolved_criteria_refs must be absent; keys=%v", mapKeys(got))
	}
	if _, ok := got["reference_resolver_overlay_applied"]; ok {
		t.Fatalf("default get must be raw: overlay flags must be absent; keys=%v", mapKeys(got))
	}
	if _, ok := got[objects.FieldKeyGoalRefs]; !ok {
		t.Fatalf("default get must still include durable goal_refs")
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
