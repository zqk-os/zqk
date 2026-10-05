package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestGetObjectView_MilestoneCompletionReport_ProjectsFields(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	criteriaFields, err := fieldRegistry.GetFieldsForKind("criteria")
	if err != nil {
		t.Fatalf("failed to get criteria fields: %v", err)
	}
	milestoneFields, err := fieldRegistry.GetFieldsForKind("milestone")
	if err != nil {
		t.Fatalf("failed to get milestone fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	crit1ID := "CRIT-011000"
	crit2ID := "CRIT-011001"

	crit1 := createTestObject("criteria", crit1ID, criteriaFields, 0)
	crit1[objects.FieldKeyStatus] = objectStatusComplete
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, crit1, casLeaveStatus(objects.GetString(crit1, objects.FieldKeyStatus)))

	crit2 := createTestObject("criteria", crit2ID, criteriaFields, 1)
	crit2[objects.FieldKeyStatus] = objectStatusValidated
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, crit2, casLeaveStatus(objects.GetString(crit2, objects.FieldKeyStatus)))

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"criteria"}); err != nil {
		t.Fatalf("ensure criteria visible: %v", err)
	}

	milestoneID := "MIL-011000"
	milestone := createTestObject("milestone", milestoneID, milestoneFields, 0)
	milestone[objects.FieldKeyCriteriaRefs] = []string{crit1ID, crit2ID}
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, milestone, casLeaveStatus(objects.GetString(milestone, objects.FieldKeyStatus)))
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"milestone"}); err != nil {
		t.Fatalf("ensure milestone visible: %v", err)
	}

	cmd := testEnv.CreateCLICommand(
		"object",
		"get",
		milestoneID,
		"--format",
		"json",
		"--view",
		"milestone-completion-report",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get failed: %v output=%s", err, string(out))
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal get output: %v; output=%s", err, string(out))
	}

	if got["milestone_complete_via_criteria_refs"] != true {
		t.Fatalf("expected milestone_complete_via_criteria_refs=true; got=%v", got["milestone_complete_via_criteria_refs"])
	}
	if got["milestone_percent_complete_via_criteria_refs"] != float64(100) {
		t.Fatalf("expected percent=100; got=%v", got["milestone_percent_complete_via_criteria_refs"])
	}

	if _, ok := got[objects.FieldKeyCompletionCriteria]; ok {
		t.Fatalf("expected completion_criteria excluded from milestone-completion-report view")
	}
	if _, ok := got["reference_resolver_overlay_applied"]; ok {
		t.Fatalf("expected resolver overlay metadata excluded from view projection")
	}

	resolved, ok := got["resolved_criteria_refs"].([]any)
	if !ok {
		t.Fatalf("expected resolved_criteria_refs as array; got=%T", got["resolved_criteria_refs"])
	}
	if len(resolved) != 2 {
		t.Fatalf("expected resolved_criteria_refs length=2; got=%d", len(resolved))
	}

	firstCrit, _ := resolved[0].(map[string]any)
	if firstCrit[objects.FieldKeyStatus] != objectStatusComplete && firstCrit[objects.FieldKeyStatus] != objectStatusValidated {
		t.Fatalf("expected resolved criteria status complete/validated; got=%v", firstCrit[objects.FieldKeyStatus])
	}
	if firstCrit[objects.FieldKeyTitle] == emptyValue {
		t.Fatalf("expected resolved criteria title non-empty")
	}
}
