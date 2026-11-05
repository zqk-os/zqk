package object

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestLifecycleAllowsTransition_WildcardAndExact(t *testing.T) {
	life := &objects.Lifecycle{
		Transitions: []objects.Transition{
			{From: "exploring", To: "deferred", Manual: true},
			{From: "*", To: "archived", Manual: true},
		},
	}
	if !lifecycleAllowsTransition(life, "exploring", "deferred") {
		t.Fatal("expected exploring→deferred")
	}
	if !lifecycleAllowsTransition(life, "rejected", "archived") {
		t.Fatal("expected *→archived from rejected")
	}
	if lifecycleAllowsTransition(life, "rejected", "deferred") {
		t.Fatal("rejected→deferred must be false without an edge")
	}
	if lifecycleAllowsTransition(life, "exploring", "planned") {
		t.Fatal("missing edge must be false")
	}
}

func TestObjectParkCommand_PriorityPlanUnsetsActiveOrder(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	planFields, err := fieldRegistry.GetFieldsForKind(objects.KindPriorityPlan)
	if err != nil {
		t.Fatalf("failed to get priority_plan fields: %v", err)
	}

	root := testEnv.GetTestRoot()
	fs, err := storage.NewFileObjectStorage(root)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, root, fs)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	planID := "PRI-PARK-001"
	plan := createTestObject(objects.KindPriorityPlan, planID, planFields, 0)
	plan[objects.FieldKeyStatus] = objects.ObjectStatusActive
	plan[objects.FieldKeyActiveOrder] = 57
	plan[objects.FieldKeyTitle] = "Priority Plan for Parking Test"

	storage.CreateCASVisible(t, fs, cliCtx, secCtx, plan, objects.ObjectStatusActive)

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{objects.KindPriorityPlan}); err != nil {
		t.Fatalf("ensure visible: %v", err)
	}

	cmd := testEnv.CreateCLICommand("object", "park", planID, "--to", "archived")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("object park failed: %v, output=%s", err, string(out))
	}
	t.Logf("park output: %s", string(out))

	fsReopened, err := storage.NewFileObjectStorage(root)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, root, fsReopened)

	got, err := fsReopened.Read(cliCtx, secCtx, planID)
	if err != nil {
		t.Fatalf("read parked plan: %v", err)
	}

	if st, _ := got[objects.FieldKeyStatus].(string); st != objects.ObjectStatusArchived {
		t.Errorf("expected status 'archived', got '%v'", st)
	}

	if val, ok := got[objects.FieldKeyActiveOrder]; ok && val != nil {
		t.Errorf("expected active_order to be unset/removed, got %v", val)
	}
}
