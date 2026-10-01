package scheduler

import (
	"context"
	"os"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_CapOrchestrator_IndexesAndHelpers(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-cap-orch-deep-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	h := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	// 1. openAgentInstructionIndex tests
	agiIdx := &openAgentInstructionIndex{
		byPlanPersona: make(map[string][]string),
	}
	agiIdx.remember("PRI-1", "tpm", "groom backlog")
	if !agiIdx.has("PRI-1", "tpm", "groom") {
		t.Errorf("expected to find instruction in index")
	}
	if agiIdx.has("PRI-1", "tpm", "nonexistent") {
		t.Errorf("expected not to find nonexistent instruction")
	}
	if agiIdx.has("PRI-2", "tpm", "groom") {
		t.Errorf("expected not to find instruction under different plan")
	}
	// nil index check
	var nilAGI *openAgentInstructionIndex
	if nilAGI.has("PRI-1", "tpm", "test") {
		t.Errorf("expected nilAGI.has to return false")
	}
	nilAGI.remember("P", "T", "I") // should no-op

	// 2. openAgentTaskIndex tests
	atkIdx := &openAgentTaskIndex{
		planTask: make(map[string]struct{}),
	}
	atkIdx.remember("PRI-1", "BLI-1")
	if !atkIdx.has("PRI-1", "BLI-1") {
		t.Errorf("expected to find task in index")
	}
	if !atkIdx.has("", "BLI-1") {
		t.Errorf("expected to find task with empty planID")
	}
	if atkIdx.has("PRI-1", "BLI-2") {
		t.Errorf("expected not to find unremembered task")
	}
	// nil index check
	var nilATK *openAgentTaskIndex
	if nilATK.has("P", "T") {
		t.Errorf("expected nilATK.has to return false")
	}
	nilATK.remember("P", "T") // should no-op

	// 3. rememberIfOpen tests
	atkIdx.rememberIfOpen(map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusProposed,
		capFieldPlanID:         "PRI-2",
		capFieldTaskID:         "BLI-2",
	})
	if !atkIdx.has("PRI-2", "BLI-2") {
		t.Errorf("expected BLI-2 to be remembered as open")
	}
	// Closed status should be ignored
	atkIdx.rememberIfOpen(map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusImplemented,
		capFieldPlanID:         "PRI-3",
		capFieldTaskID:         "BLI-3",
	})
	if atkIdx.has("PRI-3", "BLI-3") {
		t.Errorf("expected implemented BLI-3 to not be remembered")
	}

	// 4. hasOpenAgentInstruction and hasOpenTaskForPlan on handler
	_ = h.hasOpenAgentInstruction(ctx, "PRI-1", "tpm", "test")
	_ = h.hasOpenTaskForPlan(ctx, "PRI-1", "BLI-1")

	// 5. topOpenPlanBLIs
	// Seed some backlog items under PRI-TEST-PLAN
	planID := "PRI-TEST-PLAN"
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-top-blis")
	bgCtx = pkgctx.WithPromoteOnCreate(bgCtx)
	for i := 0; i < 4; i++ {
		tier := "P1"
		if i == 0 {
			tier = "P0"
		}
		bliID := mintAgentTaskID() // creates unique ID with ATK- prefix or similar, let's use BLI-
		bliID = "BLI-top-" + bliID[4:]
		if err := sp.Create(bgCtx, secCtx, map[string]any{
			objects.FieldKeyID:              bliID,
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyPriorityTier:    tier,
			objects.FieldKeyTitle:           "Backlog " + tier,
			objects.FieldKeyDescription:     "Backlog item description for testing " + tier,
		}); err != nil {
			t.Logf("create bli err: %v", err)
		}
	}
	top := h.topOpenPlanBLIs(ctx, planID, 2)
	t.Logf("topOpenPlanBLIs count: %d", len(top))
	if len(top) > 0 && top[0].Tier != "P0" {
		t.Errorf("expected first top item to be P0, got %s", top[0].Tier)
	}

	// 6. resolveWakePlanID
	// Direct PRI-* ID
	if h.resolveWakePlanID(ctx, "PRI-explicit") != "PRI-explicit" {
		t.Errorf("expected PRI-explicit, got %s", h.resolveWakePlanID(ctx, "PRI-explicit"))
	}
	// Fallback to active priority plan
	if err := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:            "PRI-active-discovered",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Active Plan Discovered",
		objects.FieldKeyDescription:   "Active priority plan description for testing discovery",
	}); err != nil {
		t.Logf("create plan err: %v", err)
	}
	discovered := h.resolveWakePlanID(ctx, "TASK-123")
	if discovered != "PRI-active-discovered" {
		t.Errorf("expected PRI-active-discovered, got %s", discovered)
	}

	// 7. truncateOutput and jsonOrRaw
	shortStr := truncateOutput("hello", 10)
	if shortStr != "hello" {
		t.Errorf("expected hello, got %s", shortStr)
	}
	longStr := truncateOutput("hello world this is long", 5)
	if longStr != "hello... (truncated)" {
		t.Errorf("expected truncated string, got %s", longStr)
	}

	jsonVal := jsonOrRaw([]byte(`{"k":"v"}`))
	if _, ok := jsonVal.(map[string]any); !ok {
		t.Errorf("expected map from valid json, got %T", jsonVal)
	}
	rawVal := jsonOrRaw([]byte(`not json`))
	if str, ok := rawVal.(string); !ok || str != "not json" {
		t.Errorf("expected raw string, got %v", rawVal)
	}

	// 8. wakeAgentAndScheduleHourglass
	h.wakeAgentAndScheduleHourglass(planID, "tpm")
	h.wakeAgentAndScheduleHourglass("ATK-dummy", "coder")
}
