package object

import (
	"encoding/json"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestObjectPromoteCommand(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	bliFields, err := fieldRegistry.GetFieldsForKind("backlog_item")
	if err != nil {
		t.Fatalf("failed to get backlog_item fields: %v", err)
	}

	root := testEnv.GetTestRoot()
	storage.SetCacheOperationHandler(func(*pkgctx.CacheContext) error { return nil })
	fs, err := storage.NewFileObjectStorage(root)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	// Single teardown registration — reopening must not stack cleanups that delete root early.
	testkit.RegisterStorageTestCleanup(t, root, fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()

	reopen := func(t *testing.T) *storage.FileObjectStorage {
		t.Helper()
		if fs != nil {
			storage.FlushAllOrFail(t, root)
			_ = fs.Shutdown(ctx)
		}
		next, err := storage.NewFileObjectStorage(root)
		if err != nil {
			t.Fatalf("reopen storage: %v", err)
		}
		fs = next
		return fs
	}

	bliID := "BLI-88888"
	bliObj := createTestObject("backlog_item", bliID, bliFields, 0)
	bliObj[objects.FieldKeyStatus] = "exploring"
	delete(bliObj, objects.FieldKeyPriority)
	delete(bliObj, objects.FieldKeyPriorityPlanRef)
	delete(bliObj, "priority_tier")
	bliObj["description"] = "Valid test description"
	delete(bliObj, objects.FieldKeyAcceptanceCriteria)
	delete(bliObj, objects.FieldKeyCriteriaRefs)
	delete(bliObj, objects.FieldKeyProblemStatement)
	delete(bliObj, objects.FieldKeyAcceptanceConsiderations)

	storage.CreateCASVisible(t, fs, cliCtx, secCtx, bliObj, "exploring")
	reopen(t)

	// 1. Promote when preconditions are NOT met (problem statement / acceptance considerations are empty)
	cmd1 := testEnv.CreateCLICommand("object", "promote", bliID, "--allow-degraded")
	out1, err := cmd1.CombinedOutput()
	if err == nil {
		t.Fatalf("promote command should have failed because preconditions are not met: output=%s", string(out1))
	}
	t.Logf("promote output 1: %s", string(out1))

	got1, err := reopen(t).Read(cliCtx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got1[objects.FieldKeyStatus] != "exploring" {
		t.Errorf("expected status 'exploring', got '%v'", got1[objects.FieldKeyStatus])
	}

	bliObj[objects.FieldKeyProblemStatement] = "This is a valid problem statement of sufficient length."
	bliObj[objects.FieldKeyAcceptanceConsiderations] = "Narrative only; gates are criteria_refs."
	bliObj[objects.FieldKeyPriority] = "high"
	if err := fs.Update(cliCtx, secCtx, bliID, bliObj); err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"})
	reopen(t)

	// Promote. Since validated preconditions are met, it should promote to "validated".
	cmd2 := testEnv.CreateCLICommand("object", "promote", bliID, "--allow-degraded")
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("promote command failed: %v, output=%s", err, string(out2))
	}
	t.Logf("promote output 2: %s", string(out2))

	got2, err := reopen(t).Read(cliCtx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got2[objects.FieldKeyStatus] != "validated" {
		t.Errorf("expected status 'validated', got '%v'", got2[objects.FieldKeyStatus])
	}

	ownerFixtureID := "ACC-899"
	dependencies := []struct {
		object      map[string]any
		leaveStatus string
	}{
		{
			object: map[string]any{
				objects.FieldKeyID:       ownerFixtureID,
				objects.FieldKeyKind:     objects.KindAccount,
				objects.FieldKeyTitle:    "Backlog owner fixture",
				objects.FieldKeyUsername: "backlog-owner-fixture",
				objects.FieldKeyStatus:   objects.ObjectStatusActive,
			},
			leaveStatus: objects.ObjectStatusActive,
		},
		{
			object: map[string]any{
				objects.FieldKeyID:       "CRIT-TEST-1",
				objects.FieldKeyKind:     objects.KindCriteria,
				objects.FieldKeyTitle:    "Backlog acceptance fixture",
				objects.FieldKeyCategory: "acceptance",
				objects.FieldKeyStatus:   objects.ObjectStatusAwaitingVerification,
			},
			leaveStatus: objects.ObjectStatusValidated,
		},
		{
			object: map[string]any{
				objects.FieldKeyID:     "PRI-TEST-PLAN",
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyTitle:  "Priority plan fixture",
				objects.FieldKeyStatus: objects.ObjectStatusPlanning,
			},
			leaveStatus: objects.ObjectStatusActive,
		},
		{
			object: map[string]any{
				objects.FieldKeyID:     "MIL-11111",
				objects.FieldKeyKind:   objects.KindMilestone,
				objects.FieldKeyTitle:  "Milestone fixture",
				objects.FieldKeyStatus: objects.ObjectStatusNotStarted,
			},
			leaveStatus: objects.ObjectStatusBlocked,
		},
		{
			object: map[string]any{
				objects.FieldKeyID:          "REQ-TEST-1",
				objects.FieldKeyKind:        objects.KindRequirement,
				objects.FieldKeyTitle:       "Promote fixture requirement",
				objects.FieldKeyDescription: "Satisfies CRI-SHOVEL-READY requirement_refs",
				objects.FieldKeyStatus:      objects.ObjectStatusActive,
				objects.FieldKeyPriority:    "p1",
			},
			leaveStatus: objects.ObjectStatusActive,
		},
	}
	for _, dependency := range dependencies {
		storage.CreateCASVisible(t, fs, cliCtx, secCtx, dependency.object, dependency.leaveStatus)
	}

	// 3. Satisfy planned *and* CRI-SHOVEL-READY. The validated→planned edge names
	// CRI-SHOVEL-READY; without requirement_refs and a stakeholder/persona lane that
	// gate fails, and promote then accepts in_progress on status preconditions alone
	// (skipping shovel-ready). estimated_effort is an in_progress status field, not
	// the planned hop.
	updates := map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-TEST-PLAN",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-11111"},
		objects.FieldKeyPriorityTier:    "P1",
		objects.FieldKeyOwnerRef:        ownerFixtureID,
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-TEST-1"},
		objects.FieldKeyRequirementRefs: []any{"REQ-TEST-1"},
		objects.FieldKeyStakeholderType: "builder",
		objects.FieldKeyEstimatedEffort: "3d",
	}
	if err := fs.Update(cliCtx, secCtx, bliID, updates); err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"})
	// Shut down in-process storage before CLI promote so write-behind cannot rewrite after the CLI hop.
	reopen(t)
	_ = fs.Shutdown(ctx)
	fs = nil
	storage.FlushAllOrFail(t, root)
	if err := storage.WaitForWALProcessing(root, 5*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}

	// Promote is strictly forward by percent_complete (one hop): validated → planned.
	cmd3 := testEnv.CreateCLICommand("object", "promote", bliID, "--allow-degraded")
	out3, err := cmd3.CombinedOutput()
	if err != nil {
		t.Fatalf("promote command failed: %v, output=%s", err, string(out3))
	}
	t.Logf("promote output 3: %s", string(out3))

	// Assert via a fresh CLI process so we verify the cross-process contract
	// instead of the parent process's possibly stale CAS listing-index view.
	getCmd := testEnv.CreateCLICommand("object", "get", bliID, "--format", "json", "--allow-degraded")
	getOut, err := getCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object get after promote failed: %v, output=%s", err, string(getOut))
	}
	// in_progress needs a live priority_plan + active milestone; planned is the hop this fixture can prove.
	var got3 map[string]any
	if err := json.Unmarshal([]byte(stripJSONOutputForParse(string(getOut))), &got3); err != nil {
		t.Fatalf("parse object get json: %v, output=%s", err, string(getOut))
	}
	if got3[objects.FieldKeyStatus] != "planned" {
		t.Errorf("expected status 'planned', got '%v' (get output=%s)", got3[objects.FieldKeyStatus], string(getOut))
	}
}
