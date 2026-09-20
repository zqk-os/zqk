package object

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestObjectDemoteCommand(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	bliFields, err := fieldRegistry.GetFieldsForKind("backlog_item")
	if err != nil {
		t.Fatalf("failed to get backlog_item fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	bliID := "BLI-88888"
	bliObj := createTestObject("backlog_item", bliID, bliFields, 0)
	// planned→validated is the primary backlog demote edge (shovel_ready back to realign).
	bliObj[objects.FieldKeyStatus] = "planned"
	bliObj[objects.FieldKeyProblemStatement] = "This is a valid problem statement of sufficient length."
	bliObj[objects.FieldKeyDescription] = "This is a valid problem statement/description of sufficient length."
	bliObj[objects.FieldKeyAcceptanceConsiderations] = "Narrative only; gates are criteria_refs."
	bliObj[objects.FieldKeyPriority] = "high"
	bliObj[objects.FieldKeyPriorityTier] = "P1"
	bliObj[objects.FieldKeyPriorityPlanRef] = "PRI-TEST-PLAN"
	bliObj[objects.FieldKeyMilestoneRefs] = []any{"MIL-11111"}

	// Create in CAS with planned status for demote testing.
	// draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fs, cliCtx, secCtx, bliObj, objects.ObjectStatusPlanned)

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"}); err != nil {
		t.Fatalf("ensure visible: %v", err)
	}

	cmd1 := testEnv.CreateCLICommand("object", "demote", bliID, "--allow-degraded")
	out1, err := cmd1.CombinedOutput()
	if err != nil {
		t.Fatalf("demote command failed: %v, output=%s", err, string(out1))
	}
	t.Logf("demote output 1: %s", string(out1))

	fs, err = storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testEnv.GetTestRoot(), fs)
	got1, err := fs.Read(cliCtx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got1[objects.FieldKeyStatus] != "validated" {
		t.Errorf("expected status 'validated', got '%v'", got1[objects.FieldKeyStatus])
	}
}
