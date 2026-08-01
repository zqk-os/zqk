package object

import (
	"encoding/json"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestGetMilestoneCriteriaOverlay_AllComplete_UsesCriteriaStatuses(t *testing.T) {
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

	// Create an isolated file-backed storage.
	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	crit1ID := "CRIT-001000"
	crit2ID := "CRIT-001001"

	crit1 := createTestObject("criteria", crit1ID, criteriaFields, 0)
	crit1[objects.FieldKeyStatus] = "complete"
	if err := fs.Create(cliCtx, secCtx, crit1); err != nil {
		t.Fatalf("create crit1: %v", err)
	}

	crit2 := createTestObject("criteria", crit2ID, criteriaFields, 1)
	crit2[objects.FieldKeyStatus] = "validated"
	if err := fs.Create(cliCtx, secCtx, crit2); err != nil {
		t.Fatalf("create crit2: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"criteria"}); err != nil {
		t.Fatalf("ensure criteria visible: %v", err)
	}

	milestoneID := "MIL-001000"
	milestone := createTestObject("milestone", milestoneID, milestoneFields, 0)
	milestone[objects.FieldKeyCriteriaRefs] = []string{crit1ID, crit2ID}
	// Intentionally contradict durable criteria statuses: even if text says incomplete,
	// the overlay should rely on referenced criteria object lifecycle status.
	milestone[objects.FieldKeyCompletionCriteria] = []string{
		"[ ] backlog-item-example status is complete",
		"[ ] backlog-item-example-2 status is complete",
	}
	if err := fs.Create(cliCtx, secCtx, milestone); err != nil {
		t.Fatalf("create milestone: %v", err)
	}

	cmd := testEnv.CreateCLICommand("object", "get", milestoneID, "--format", "json")
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

	resolved, ok := got["resolved_criteria_refs"].([]any)
	if !ok {
		t.Fatalf("expected resolved_criteria_refs as array; got=%T", got["resolved_criteria_refs"])
	}
	if len(resolved) != 2 {
		t.Fatalf("expected resolved_criteria_refs length=2; got=%d", len(resolved))
	}
}

func TestGetMilestoneCriteriaOverlay_IncompleteWhenAnyCriteriaNotComplete(t *testing.T) {
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
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	crit1ID := "CRIT-002000"
	crit2ID := "CRIT-002001"

	crit1 := createTestObject("criteria", crit1ID, criteriaFields, 0)
	crit1[objects.FieldKeyStatus] = "complete"
	if err := fs.Create(cliCtx, secCtx, crit1); err != nil {
		t.Fatalf("create crit1: %v", err)
	}

	crit2 := createTestObject("criteria", crit2ID, criteriaFields, 1)
	crit2[objects.FieldKeyStatus] = "in_progress"
	if err := fs.Create(cliCtx, secCtx, crit2); err != nil {
		t.Fatalf("create crit2: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"criteria"}); err != nil {
		t.Fatalf("ensure criteria visible: %v", err)
	}

	milestoneID := "MIL-002000"
	milestone := createTestObject("milestone", milestoneID, milestoneFields, 0)
	milestone[objects.FieldKeyCriteriaRefs] = []string{crit1ID, crit2ID}
	milestone[objects.FieldKeyCompletionCriteria] = []string{
		"[x] backlog-item-example status is complete",
		"[x] backlog-item-example-2 status is complete",
	}

	if err := fs.Create(cliCtx, secCtx, milestone); err != nil {
		t.Fatalf("create milestone: %v", err)
	}

	cmd := testEnv.CreateCLICommand("object", "get", milestoneID, "--format", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get failed: %v output=%s", err, string(out))
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal get output: %v; output=%s", err, string(out))
	}

	if got["milestone_complete_via_criteria_refs"] != false {
		t.Fatalf("expected milestone_complete_via_criteria_refs=false; got=%v", got["milestone_complete_via_criteria_refs"])
	}
	// completeCount=1 out of 2 => 50.
	if got["milestone_percent_complete_via_criteria_refs"] != float64(50) {
		t.Fatalf("expected percent=50; got=%v", got["milestone_percent_complete_via_criteria_refs"])
	}
}
