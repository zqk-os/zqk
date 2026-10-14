package object

import (
	stdctx "context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestNewRefCmd_Hierarchy(t *testing.T) {
	cmd := NewRefCmd()
	if cmd.Use != "ref" {
		t.Errorf("expected Use 'ref', got %s", cmd.Use)
	}

	subCommands := cmd.Commands()
	if len(subCommands) < 2 {
		t.Fatalf("expected at least 2 subcommands (add, remove), got %d", len(subCommands))
	}

	hasAdd := false
	hasRemove := false
	for _, sub := range subCommands {
		if sub.Name() == "add" {
			hasAdd = true
			if sub.Flags().Lookup("field") == nil {
				t.Error("add command missing --field flag")
			}
			if sub.Flags().Lookup("override") == nil {
				t.Error("add command missing --override flag")
			}
			if sub.Flags().Lookup("reason-code") == nil {
				t.Error("add command missing --reason-code flag")
			}
		}
		if sub.Name() == "remove" {
			hasRemove = true
			if sub.Flags().Lookup("field") == nil {
				t.Error("remove command missing --field flag")
			}
			if sub.Flags().Lookup("kind") == nil {
				t.Error("remove command missing --kind flag")
			}
			if sub.Flags().Lookup("all") == nil {
				t.Error("remove command missing --all flag")
			}
			if sub.Flags().Lookup("park") == nil {
				t.Error("remove command missing --park flag")
			}
		}
	}

	if !hasAdd {
		t.Error("ref command missing 'add' subcommand")
	}
	if !hasRemove {
		t.Error("ref command missing 'remove' subcommand")
	}
}

func TestResolveRefFieldForTarget(t *testing.T) {
	// Source object with standard fields
	sourceObj := map[string]any{
		"id":                  "BLI-100",
		"kind":                "backlog_item",
		"requirement_refs":    []any{},
		"priority_plan_ref":   "",
		"related_object_refs": []any{},
	}

	// 1. Target requirement -> requirement_refs (slice)
	field, isSlice, err := resolveRefFieldForTarget(nil, sourceObj, "requirement", "REQ-1")
	if err != nil {
		t.Fatalf("unexpected error resolving requirement: %v", err)
	}
	if field != "requirement_refs" || !isSlice {
		t.Errorf("expected (requirement_refs, true), got (%s, %v)", field, isSlice)
	}

	// 2. Target priority_plan -> priority_plan_ref (scalar)
	field, isSlice, err = resolveRefFieldForTarget(nil, sourceObj, "priority_plan", "PRI-1")
	if err != nil {
		t.Fatalf("unexpected error resolving priority_plan: %v", err)
	}
	if field != "priority_plan_ref" || isSlice {
		t.Errorf("expected (priority_plan_ref, false), got (%s, %v)", field, isSlice)
	}

	// 3. Target unknown kind without typed field -> falls back to related_object_refs
	field, isSlice, err = resolveRefFieldForTarget(nil, sourceObj, "custom_kind", "CUST-1")
	if err != nil {
		t.Fatalf("unexpected error resolving custom_kind: %v", err)
	}
	if field != "related_object_refs" || !isSlice {
		t.Errorf("expected (related_object_refs, true), got (%s, %v)", field, isSlice)
	}

	// 4. Source object without related_object_refs or compatible field -> errors
	isolatedObj := map[string]any{
		"id":   "ISO-1",
		"kind": "isolated",
	}
	_, _, err = resolveRefFieldForTarget(nil, isolatedObj, "unknown", "UNK-1")
	if err == nil {
		t.Fatal("expected error when no compatible ref field exists")
	}
	if !strings.Contains(err.Error(), "no compatible reference field found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFindInTypedRefFields(t *testing.T) {
	sourceObj := map[string]any{
		"id":                  "BLI-100",
		"requirement_refs":    []string{"REQ-1", "REQ-2"},
		"priority_plan_ref":   "PRI-99",
		"related_object_refs": []string{"REL-1"},
	}

	// Present in typed slice
	if f := findInTypedRefFields(sourceObj, nil, "REQ-1"); f != "requirement_refs" {
		t.Errorf("expected requirement_refs, got %s", f)
	}

	// Present in typed scalar
	if f := findInTypedRefFields(sourceObj, nil, "PRI-99"); f != "priority_plan_ref" {
		t.Errorf("expected priority_plan_ref, got %s", f)
	}

	// In related_object_refs should NOT be returned as typed field
	if f := findInTypedRefFields(sourceObj, nil, "REL-1"); f != "" {
		t.Errorf("expected empty for related_object_refs item, got %s", f)
	}

	// Not present anywhere
	if f := findInTypedRefFields(sourceObj, nil, "OTHER-1"); f != "" {
		t.Errorf("expected empty for unknown item, got %s", f)
	}
}

func TestIsSliceField(t *testing.T) {
	sourceObj := map[string]any{
		"requirement_refs":  []any{"REQ-1"},
		"priority_plan_ref": "PRI-1",
	}

	if !isSliceField(nil, sourceObj, "requirement_refs") {
		t.Error("expected requirement_refs to be recognized as slice")
	}
	if isSliceField(nil, sourceObj, "priority_plan_ref") {
		t.Error("expected priority_plan_ref to NOT be recognized as slice")
	}
	if !isSliceField(nil, sourceObj, "custom_refs") {
		t.Error("expected custom_refs to be recognized as slice by suffix")
	}
}

func TestGetRefSliceAndScalar(t *testing.T) {
	sourceObj := map[string]any{
		"requirement_refs":  []any{"REQ-1", "REQ-2"},
		"priority_plan_ref": "PRI-1",
	}
	updates := map[string]any{
		"requirement_refs": []string{"REQ-1", "REQ-2", "REQ-3"},
	}

	// Updates should override sourceObj
	slice := getRefSlice(sourceObj, updates, "requirement_refs")
	if len(slice) != 3 || slice[2] != "REQ-3" {
		t.Errorf("expected [REQ-1 REQ-2 REQ-3], got %v", slice)
	}

	// Fallback to sourceObj when not in updates
	scalar := getRefScalar(sourceObj, updates, "priority_plan_ref")
	if scalar != "PRI-1" {
		t.Errorf("expected PRI-1, got %s", scalar)
	}
}

func TestRefRemove_FieldFlagAllowsScalarUnset(t *testing.T) {
	cmd := NewRefRemoveCmd()
	if cmd.Flags().Lookup("field") == nil {
		t.Fatal("expected --field flag on ref remove command")
	}
}

func TestHasSpecificRefField_ParentVsChildOwned(t *testing.T) {
	// 1. Requirement object owns criteria_refs (parent-owned)
	reqObj := map[string]any{
		"id":            "REQ-100",
		"kind":          "requirement",
		"criteria_refs": []any{"CRIT-101"},
	}
	if !hasSpecificRefField(nil, reqObj, "criteria") {
		t.Error("expected requirement to have specific ref field for criteria")
	}

	// 2. Priority plan object does NOT have backlog_item_refs (child-owned on BLI)
	priObj := map[string]any{
		"id":   "PRI-100",
		"kind": "priority_plan",
	}
	if hasSpecificRefField(nil, priObj, "backlog_item") {
		t.Error("expected priority_plan to NOT have specific ref field for backlog_item")
	}

	// 3. Backlog item object DOES have priority_plan_ref (child-owned)
	bliObj := map[string]any{
		"id":                "BLI-100",
		"kind":              "backlog_item",
		"priority_plan_ref": "PRI-100",
	}
	if !hasSpecificRefField(nil, bliObj, "priority_plan") {
		t.Error("expected backlog_item to have specific ref field for priority_plan")
	}
}

func TestResolveSpecificRefField(t *testing.T) {
	bliObj := map[string]any{
		"id":                "BLI-100",
		"kind":              "backlog_item",
		"priority_plan_ref": "PRI-100",
		"requirement_refs":  []any{"REQ-1"},
	}

	f, isSlice := resolveSpecificRefField(nil, bliObj, "priority_plan")
	if f != "priority_plan_ref" || isSlice {
		t.Errorf("expected priority_plan_ref, false; got %s, %v", f, isSlice)
	}

	f2, isSlice2 := resolveSpecificRefField(nil, bliObj, "requirement")
	if f2 != "requirement_refs" || !isSlice2 {
		t.Errorf("expected requirement_refs, true; got %s, %v", f2, isSlice2)
	}
}

func TestRefAddRemoveOperations(t *testing.T) {
	addCmd := NewRefAddCmd()
	if addCmd == nil {
		t.Fatal("NewRefAddCmd returned nil")
	}
	removeCmd := NewRefRemoveCmd()
	if removeCmd == nil {
		t.Fatal("NewRefRemoveCmd returned nil")
	}

	sourceObj := map[string]any{
		"id":                  "BLI-100",
		"kind":                "backlog_item",
		"requirement_refs":    []any{"REQ-1"},
		"priority_plan_ref":   "PRI-1",
		"related_object_refs": []any{},
	}

	// 1. Resolve slice field
	f, isSlice, err := resolveRefFieldForTarget(nil, sourceObj, "requirement", "REQ-2")
	if err != nil || f != "requirement_refs" || !isSlice {
		t.Fatalf("expected requirement_refs slice, got %s, %v, err=%v", f, isSlice, err)
	}

	// 2. Resolve scalar field
	f, isSlice, err = resolveRefFieldForTarget(nil, sourceObj, "priority_plan", "PRI-2")
	if err != nil || f != "priority_plan_ref" || isSlice {
		t.Fatalf("expected priority_plan_ref scalar, got %s, %v, err=%v", f, isSlice, err)
	}

	// 3. Verify slice field extraction
	slice := getRefSlice(sourceObj, nil, "requirement_refs")
	if len(slice) != 1 || slice[0] != "REQ-1" {
		t.Fatalf("expected initial slice [REQ-1], got %v", slice)
	}
}

func TestInProcess_RefAdd_Direct_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed test requirement
	reqObj := map[string]any{
		objects.FieldKeyID:            "REQ-REF-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Ref Target Requirement",
		objects.FieldKeyDescription:   "Description for ref test requirement",
		objects.FieldKeyPriority:      "p1",
		objects.FieldKeyStatus:        "complete",
		objects.FieldKeyGoalRefs:      []string{},
		objects.FieldKeyCriteriaRefs:  []string{bulkFixtureCriteriaID},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, reqObj, "complete")

	bli1 := plannedBLI("BLI-REF-001", "Ref BLI 1", map[string]any{
		objects.FieldKeyRequirementRefs: []string{"REQ-REF-001"},
	})
	bli2 := plannedBLI("BLI-REF-002", "Ref BLI 2", map[string]any{
		objects.FieldKeyRequirementRefs: []string{},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli1, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli2, objectStatusPlanned)

	// 1. Direct Add: Add requirement to BLI-REF-002
	out, err := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-REF-002", "REQ-REF-001")
	if err != nil || (!strings.Contains(out, "Successfully added reference") && !strings.Contains(out, "Successfully")) {
		t.Fatalf("failed to add ref: %v\noutput: %s", err, out)
	}

	// 2. Direct Add (Idempotent): Add requirement again
	outIdempotent, err := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-REF-002", "REQ-REF-001")
	if err != nil || (!strings.Contains(outIdempotent, "No reference changes required") && !strings.Contains(outIdempotent, "already up to date") && !strings.Contains(outIdempotent, "Successfully")) {
		t.Fatalf("idempotent add failed: %v\noutput: %s", err, outIdempotent)
	}

	// 3. Self-Reference check: cannot reference self
	if _, errSelf := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-REF-001", "BLI-REF-001"); errSelf == nil || !strings.Contains(errSelf.Error(), "cannot reference self") {
		t.Errorf("expected cannot reference self error, got: %v", errSelf)
	}

	// 4. Missing Source check
	if _, errMissingSource := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-NONEXISTENT", "REQ-REF-001"); errMissingSource == nil || !strings.Contains(errMissingSource.Error(), "not found") {
		t.Errorf("expected source not found error, got: %v", errMissingSource)
	}

	// 5. Missing Target check
	if _, errMissingTarget := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-REF-001", "REQ-NONEXISTENT"); errMissingTarget == nil || !strings.Contains(errMissingTarget.Error(), "not found") {
		t.Errorf("expected target not found error, got: %v", errMissingTarget)
	}
}

func TestInProcess_RefAdd_Inverted_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	planObj := map[string]any{
		objects.FieldKeyID:            "PRI-REF-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Ref Priority Plan",
		objects.FieldKeyDescription:   "Description for ref priority plan",
		objects.FieldKeyStatus:        objects.ObjectStatusOriginated,
		objects.FieldKeyEpicRefs:      []string{},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, planObj, objects.ObjectStatusOriginated)

	bliTarget := plannedBLI("BLI-REF-INV", "Ref BLI Inverted Target", map[string]any{
		objects.FieldKeyPriorityPlanRef: "",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliTarget, objectStatusPlanned)

	// Inverted Add: Source is PRI-REF-001, target is BLI-REF-INV
	outInv, err := executeObjectCommand(t, tempDir, provider, "ref", "add", "PRI-REF-001", "BLI-REF-INV")
	if err != nil || (!strings.Contains(outInv, "Successfully linked") && !strings.Contains(outInv, "Successfully added")) {
		t.Fatalf("inverted ref add failed: %v\noutput: %s", err, outInv)
	}

	updatedBli, err := provider.Read(stdctx.Background(), secCtx, "BLI-REF-INV")
	if err != nil || objects.GetString(updatedBli, objects.FieldKeyPriorityPlanRef) != "PRI-REF-001" {
		t.Fatalf("expected BLI-REF-INV priority_plan_ref to be PRI-REF-001, got: %v", updatedBli)
	}

	// Inverted Add (Idempotent)
	outInvIdem, err := executeObjectCommand(t, tempDir, provider, "ref", "add", "PRI-REF-001", "BLI-REF-INV")
	if err != nil || (!strings.Contains(outInvIdem, "already up to date") && !strings.Contains(outInvIdem, "Successfully linked")) {
		t.Fatalf("inverted idempotent add failed: %v", err)
	}

	// Scalar Overwrite Prevention
	planObj2 := map[string]any{
		objects.FieldKeyID:            "PRI-REF-002",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Ref Priority Plan 2",
		objects.FieldKeyDescription:   "Description for ref priority plan 2",
		objects.FieldKeyStatus:        objects.ObjectStatusOriginated,
		objects.FieldKeyEpicRefs:      []string{},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, planObj2, objects.ObjectStatusOriginated)

	if _, errOverwrite := executeObjectCommand(t, tempDir, provider, "ref", "add", "PRI-REF-002", "BLI-REF-INV"); errOverwrite == nil || !strings.Contains(errOverwrite.Error(), "cannot overwrite scalar reference") {
		t.Errorf("expected cannot overwrite scalar reference error, got: %v", errOverwrite)
	}
}

func TestInProcess_RefAdd_ScopeLock_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed sealed priority plan in progress
	sealedPlan := map[string]any{
		objects.FieldKeyID:            "PRI-SEALED-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Sealed In-Progress Plan",
		objects.FieldKeyDescription:   "Description for sealed plan",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyEpicRefs:      []string{},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, sealedPlan, objects.ObjectStatusInProgress)

	// Seed unassigned BLI
	unassignedBLI := plannedBLI("BLI-SEALED-001", "Unassigned BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, unassignedBLI, objectStatusPlanned)

	// 1. Ref Add without --override should fail due to scope lock
	_, errLock := executeObjectCommand(t, tempDir, provider, "ref", "add", "PRI-SEALED-001", "BLI-SEALED-001")
	if errLock == nil || !strings.Contains(errLock.Error(), "sealed / execution-facing") {
		t.Errorf("expected sealed plan scope-lock error, got: %v", errLock)
	}

	// 2. Ref Add with --override but without --reason-code should fail
	_, errNoReason := executeObjectCommand(t, tempDir, provider, "ref", "add", "PRI-SEALED-001", "BLI-SEALED-001", "--override")
	if errNoReason == nil || !strings.Contains(errNoReason.Error(), "--reason-code is required") {
		t.Errorf("expected missing reason-code error, got: %v", errNoReason)
	}

	// 3. Ref Add with --override and --reason-code should succeed (>= 30 chars)
	outOverride, err := executeObjectCommand(t, tempDir, provider, "ref", "add", "PRI-SEALED-001", "BLI-SEALED-001", "--override", "--reason-code=emergency-hotfix-addition-to-sealed-priority-plan-for-critical-bug")
	if err != nil {
		t.Fatalf("override add failed: %v\noutput: %s", err, outOverride)
	}
	if !strings.Contains(outOverride, "Successfully linked") && !strings.Contains(outOverride, "Successfully added") {
		t.Errorf("expected successful override link, got: %s", outOverride)
	}
}

func TestInProcess_RefAdd_CycleAndRedundancy_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed requirement
	req := map[string]any{
		objects.FieldKeyID:            "REQ-CYCLE-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Cycle Requirement",
		objects.FieldKeyDescription:   "Description for cycle requirement",
		objects.FieldKeyPriority:      "p1",
		objects.FieldKeyStatus:        "complete",
		objects.FieldKeyGoalRefs:      []string{},
		objects.FieldKeyCriteriaRefs:  []string{bulkFixtureCriteriaID},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, req, "complete")

	// Seed BLI with REQ-CYCLE-001 in requirement_refs
	bli := plannedBLI("BLI-CYCLE-001", "Cycle BLI", map[string]any{
		objects.FieldKeyRequirementRefs: []string{"REQ-CYCLE-001"},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli, objectStatusPlanned)

	// 1. Redundant reference test: target REQ-CYCLE-001 is already in requirement_refs
	_, errRedundant := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-CYCLE-001", "REQ-CYCLE-001", "--field", "related_object_refs")
	if errRedundant == nil || !strings.Contains(errRedundant.Error(), "already referenced in typed field") {
		t.Errorf("expected redundant reference error, got: %v", errRedundant)
	}

	// 2. Direct cycle check: Obj A references Obj B in related_object_refs
	bliA := plannedBLI("BLI-CYCLE-A", "BLI A", map[string]any{
		objects.FieldKeyRelatedObjectRefs: []string{"BLI-CYCLE-B"},
	})
	bliB := plannedBLI("BLI-CYCLE-B", "BLI B", map[string]any{
		objects.FieldKeyRelatedObjectRefs: []string{},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliA, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliB, objectStatusPlanned)

	_, errCycle := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-CYCLE-B", "BLI-CYCLE-A", "--field", "related_object_refs")
	if errCycle == nil || !strings.Contains(errCycle.Error(), "cycle detected") {
		t.Errorf("expected cycle detected error, got: %v", errCycle)
	}

	// 3. Transitive cycle check: Obj A -> Obj B -> Obj C. Adding Obj A to Obj C creates a cycle.
	bliC := plannedBLI("BLI-CYCLE-C", "BLI C", map[string]any{
		objects.FieldKeyRelatedObjectRefs: []string{},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliC, objectStatusPlanned)

	// Link B -> C (A already has B, so A -> B -> C)
	outBC, errLinkBC := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-CYCLE-B", "BLI-CYCLE-C", "--field", "related_object_refs")
	if errLinkBC != nil {
		t.Fatalf("failed linking B to C: %v\noutput: %s", errLinkBC, outBC)
	}

	// Try to link C -> A (creates transitive cycle C -> A -> B -> C)
	_, errTransCycle := executeObjectCommand(t, tempDir, provider, "ref", "add", "BLI-CYCLE-C", "BLI-CYCLE-A", "--field", "related_object_refs")
	if errTransCycle == nil || !strings.Contains(errTransCycle.Error(), "cycle detected") {
		t.Errorf("expected transitive cycle detected error, got: %v", errTransCycle)
	}
}

func TestInProcess_RefRemove_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	plan := map[string]any{
		objects.FieldKeyID:            "PRI-RM-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Remove Test Plan",
		objects.FieldKeyDescription:   "Description for remove test plan",
		objects.FieldKeyStatus:        objects.ObjectStatusOriginated,
		objects.FieldKeyEpicRefs:      []string{},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, plan, objects.ObjectStatusOriginated)

	req := map[string]any{
		objects.FieldKeyID:            "REQ-RM-001",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Remove Test Requirement",
		objects.FieldKeyDescription:   "Description for remove test requirement",
		objects.FieldKeyPriority:      "p1",
		objects.FieldKeyStatus:        "complete",
		objects.FieldKeyGoalRefs:      []string{},
		objects.FieldKeyCriteriaRefs:  []string{bulkFixtureCriteriaID},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, req, "complete")

	bli := plannedBLI("BLI-RM-001", "Remove Test BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-RM-001",
		objects.FieldKeyRequirementRefs: []string{"REQ-RM-001"},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli, objectStatusPlanned)

	verifyRefRemoveDirectAndInverted(t, tempDir, provider)
	verifyRefRemoveFilters(t, tempDir, provider, secCtx)
}

func verifyRefRemoveDirectAndInverted(t *testing.T, tempDir string, provider storage.ObjectStorageProvider) {
	t.Helper()
	if _, err := executeObjectCommand(t, tempDir, provider, "ref", "remove", "BLI-RM-001"); err == nil {
		t.Fatal("expected missing argument error")
	}

	outDirect, err := executeObjectCommand(t, tempDir, provider, "ref", "remove", "BLI-RM-001", "REQ-RM-001")
	if err != nil || (!strings.Contains(outDirect, "Successfully removed reference") && !strings.Contains(outDirect, "Successfully")) {
		t.Fatalf("failed direct remove: out=%s, err=%v", outDirect, err)
	}

	_, err = executeObjectCommand(t, tempDir, provider, "ref", "remove", "BLI-RM-001", "REQ-RM-001")
	if err != nil {
		t.Fatalf("noop remove failed: %v", err)
	}

	outInv, err := executeObjectCommand(t, tempDir, provider, "ref", "remove", "PRI-RM-001", "BLI-RM-001")
	if err != nil || (!strings.Contains(outInv, "Successfully removed reference") && !strings.Contains(outInv, "Successfully")) {
		t.Fatalf("inverted remove failed: out=%s, err=%v", outInv, err)
	}
}

func verifyRefRemoveFilters(t *testing.T, tempDir string, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	bliField := plannedBLI("BLI-RM-002", "Remove Field BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-RM-001",
		objects.FieldKeyRequirementRefs: []string{"REQ-RM-001"},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliField, objectStatusPlanned)

	outField, err := executeObjectCommand(t, tempDir, provider, "ref", "remove", "BLI-RM-002", "--field", "priority_plan_ref")
	if err != nil || (!strings.Contains(outField, "Successfully removed reference") && !strings.Contains(outField, "Successfully")) {
		t.Fatalf("remove by field failed: out=%s, err=%v", outField, err)
	}

	bliKind := plannedBLI("BLI-RM-003", "Remove Kind BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-RM-001",
		objects.FieldKeyRequirementRefs: []string{"REQ-RM-001"},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliKind, objectStatusPlanned)

	outKind, err := executeObjectCommand(t, tempDir, provider, "ref", "remove", "BLI-RM-003", "--kind", "requirement")
	if err != nil || (!strings.Contains(outKind, "Successfully removed reference") && !strings.Contains(outKind, "Successfully")) {
		t.Fatalf("remove by kind failed: out=%s, err=%v", outKind, err)
	}

	bliAll := plannedBLI("BLI-RM-004", "Remove All BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-RM-001",
		objects.FieldKeyRequirementRefs: []string{"REQ-RM-001"},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliAll, objectStatusPlanned)

	outAll, err := executeObjectCommand(t, tempDir, provider, "ref", "remove", "BLI-RM-004", "--all")
	if err != nil || (!strings.Contains(outAll, "Successfully removed all references") && !strings.Contains(outAll, "Successfully")) {
		t.Fatalf("remove all failed: out=%s, err=%v", outAll, err)
	}
}
