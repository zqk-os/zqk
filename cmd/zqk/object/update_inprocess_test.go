package object

import (
	stdctx "context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewUpdateCmd_Structure(t *testing.T) {
	cmd := NewUpdateCmd()
	if cmd == nil {
		t.Fatal("expected NewUpdateCmd to return non-nil")
	}
	if cmd.Use == "" {
		t.Error("expected Use to be set")
	}
	hasAlias := false
	for _, a := range cmd.Aliases {
		if a == "edit" {
			hasAlias = true
			break
		}
	}
	if !hasAlias {
		t.Error("expected 'edit' alias on update command")
	}
}

func TestInProcess_Update_FieldModifications_Basic(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed a backlog_item
	bliObj := plannedBLI("BLI-UPD-001", "Initial Title", map[string]any{
		objects.FieldKeyDescription: "Initial Description",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliObj, objectStatusPlanned)

	// 1. Normal field update
	out, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-001", "--field", "description=Updated Description via in-process test")
	if err != nil {
		t.Fatalf("update --field failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Updated object") && !strings.Contains(out, "BLI-UPD-001") {
		t.Errorf("expected success message, got: %s", out)
	}

	readObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-001")
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}
	if desc, _ := readObj[objects.FieldKeyDescription].(string); desc != "Updated Description via in-process test" {
		t.Errorf("expected updated description, got: %v", desc)
	}

	// 2. Dotted field update (meta.custom_key)
	outDotted, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-001", "--field", "metadata.sub_key=nested_val")
	if err != nil {
		t.Fatalf("update with dotted field failed: %v\noutput: %s", err, outDotted)
	}
	readObj, err = provider.Read(stdctx.Background(), secCtx, "BLI-UPD-001")
	if err != nil {
		t.Fatalf("failed to read after dotted field update: %v", err)
	}
	if meta, ok := readObj["metadata"].(map[string]any); !ok || meta["sub_key"] != "nested_val" {
		t.Errorf("expected metadata.sub_key to be nested_val, got: %v", readObj["metadata"])
	}

	// 3. Append operation (components+=Comp1)
	outAppend, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-001", "--field", "components+=Comp1")
	if err != nil {
		t.Fatalf("update with append failed: %v\noutput: %s", err, outAppend)
	}
	readObj, err = provider.Read(stdctx.Background(), secCtx, "BLI-UPD-001")
	if err != nil {
		t.Fatalf("failed to read after append: %v", err)
	}
	if comps, ok := readObj["components"].([]any); !ok || len(comps) != 1 || comps[0] != "Comp1" {
		t.Errorf("expected components to have ['Comp1'], got: %v", readObj["components"])
	}
}

func TestInProcess_Update_FieldModifications_Flags(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed a backlog_item
	bliObj := plannedBLI("BLI-UPD-001B", "Initial Title", map[string]any{
		objects.FieldKeyDescription: "Initial Description",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliObj, objectStatusPlanned)

	// 4. Dry-run update
	outDry, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-001B", "--field", "title=Dry Run Title", "--dry-run")
	if err != nil {
		t.Fatalf("update with --dry-run failed: %v\noutput: %s", err, outDry)
	}
	if !strings.Contains(outDry, "dry-run") && !strings.Contains(outDry, "DRY RUN") && !strings.Contains(outDry, "Dry Run") {
		t.Logf("dry run output: %s", outDry)
	}
	// Verify title was NOT changed
	readObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-001B")
	if err != nil {
		t.Fatalf("failed to read after dry run: %v", err)
	}
	if title, _ := readObj[objects.FieldKeyTitle].(string); title == "Dry Run Title" {
		t.Errorf("expected title to remain unchanged during dry-run, but was changed")
	}

	// 5. Output format json
	outJSON, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-001B", "--field", "title=JSON Title", "--format", "json")
	if err != nil {
		t.Fatalf("update with --format json failed: %v\noutput: %s", err, outJSON)
	}
	if !strings.Contains(outJSON, "\"title\": \"JSON Title\"") && !strings.Contains(outJSON, `"title":"JSON Title"`) {
		t.Errorf("expected JSON output with updated title, got: %s", outJSON)
	}

	// 6. Unset field
	outUnset, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-001B", "--unset-field", "metadata")
	if err != nil {
		t.Fatalf("update with --unset-field failed: %v\noutput: %s", err, outUnset)
	}
	readObj, err = provider.Read(stdctx.Background(), secCtx, "BLI-UPD-001B")
	if err != nil {
		t.Fatalf("failed to read after unset: %v", err)
	}
	if _, ok := readObj["metadata"]; ok {
		t.Errorf("expected metadata field to be unset, but was still present")
	}
}

func TestInProcess_Update_OptimisticLocking(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	bliObj := plannedBLI("BLI-UPD-LOCK", "Lock Test Title", map[string]any{
		objects.FieldKeyDescription: "Lock Test Description",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliObj, objectStatusPlanned)

	readObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-LOCK")
	if err != nil {
		t.Fatalf("failed to read object: %v", err)
	}
	updatedAt, ok := readObj[objects.FieldKeyUpdatedAt].(string)
	if !ok || updatedAt == "" {
		t.Fatalf("expected updated_at timestamp on object")
	}

	// 1. Correct expected_updated_at succeeds
	_, err = executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-LOCK", "--field", "title=Lock Updated", "--expected-updated-at", updatedAt)
	if err != nil {
		t.Fatalf("update with matching expected-updated-at failed: %v", err)
	}

	// 2. Stale expected_updated_at fails
	_, errStale := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-LOCK", "--field", "title=Stale Update", "--expected-updated-at", "2020-01-01T00:00:00Z")
	if errStale == nil {
		t.Errorf("expected optimistic concurrency conflict error for stale timestamp")
	}
}

func TestInProcess_Update_BulkUpdate(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	bli1 := plannedBLI("BLI-UPD-B1", "Bulk Item 1", map[string]any{
		objects.FieldKeyDescription: "Initial desc 1",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli1, objectStatusPlanned)

	bli2 := plannedBLI("BLI-UPD-B2", "Bulk Item 2", map[string]any{
		objects.FieldKeyDescription: "Initial desc 2",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli2, objectStatusPlanned)

	// Run bulk update with comma-separated IDs
	out, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-B1,BLI-UPD-B2", "--field", "description=Updated in bulk")
	if err != nil {
		t.Fatalf("bulk update failed: %v\noutput: %s", err, out)
	}
	t.Logf("bulk update output:\n%s", out)

	obj1, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-B1")
	if err != nil || obj1[objects.FieldKeyDescription] != "Updated in bulk" {
		t.Errorf("expected obj1 description to be updated in bulk, got desc=%v, err=%v", obj1[objects.FieldKeyDescription], err)
	}
	obj2, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-B2")
	if err != nil || obj2[objects.FieldKeyDescription] != "Updated in bulk" {
		t.Errorf("expected obj2 description to be updated in bulk, got desc=%v, err=%v", obj2[objects.FieldKeyDescription], err)
	}
}

func TestInProcess_Update_ForceCreate(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Update non-existent object without force should fail
	_, errNoForce := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-NONEXIST", "--field", "title=Nonexist")
	if errNoForce == nil {
		t.Errorf("expected error updating non-existent object without --force")
	}

	// Update non-existent object with --force creates it
	outForce, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-FORCE-001",
		"--force",
		"--field", "title=Created via Force",
		"--field", "description=Description created via force update",
		"--field", "priority=high",
	)
	if err != nil {
		t.Fatalf("update with --force failed: %v\noutput: %s", err, outForce)
	}

	createdObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-FORCE-001")
	if err != nil {
		t.Fatalf("failed to read object created via force: %v", err)
	}
	if title, _ := createdObj[objects.FieldKeyTitle].(string); title != "Created via Force" {
		t.Errorf("expected title 'Created via Force', got: %v", title)
	}
}

func TestInProcess_Update_Guards(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	bli := plannedBLI("BLI-UPD-GUARD", "Guard Test Item", map[string]any{
		objects.FieldKeyDescription: "Guard Test Description",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli, objectStatusPlanned)

	// 1. Guard manual status update
	_, errStatus := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-GUARD", "--field", "status=complete")
	if errStatus == nil || !strings.Contains(errStatus.Error(), "manual status updates are restricted") {
		t.Errorf("expected manual status update error, got: %v", errStatus)
	}

	// 2. Guard manual reference update
	_, errRef := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-GUARD", "--field", "requirement_refs=[\"REQ-1\"]")
	if errRef == nil || !strings.Contains(errRef.Error(), "Direct mutation of reference field") {
		t.Errorf("expected manual reference update prohibition error, got: %v", errRef)
	}

	// 3. Guard manual provenance update without override
	_, errProv := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-GUARD", "--field", "created_at=2020-01-01T00:00:00Z")
	if errProv == nil || !strings.Contains(errProv.Error(), "manual mutation of system provenance") {
		t.Errorf("expected system provenance error, got: %v", errProv)
	}

	// 4. Override with short reason code fails
	_, errShortReason := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-GUARD",
		"--field", "created_at=2020-01-01T00:00:00Z",
		"--override",
		"--reason-code=too short",
	)
	if errShortReason == nil {
		t.Errorf("expected error for short reason code (<30 chars)")
	}

	// 5. Override in non-TTY agent session is correctly blocked (or blocked by CI guard in CI)
	_, errProvNonTTY := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-GUARD",
		"--field", "created_at=2020-01-01T00:00:00Z",
		"--override",
		"--reason-code=Authorized administrative provenance timestamp correction per audit log",
	)
	if errProvNonTTY == nil || (!strings.Contains(errProvNonTTY.Error(), "lifecycle --override is blocked without an interactive TTY") &&
		!strings.Contains(errProvNonTTY.Error(), "manual overrides are completely blocked in CI environments")) {
		t.Errorf("expected non-TTY or CI override block error, got: %v", errProvNonTTY)
	}

	// 6. No updates provided error
	_, errNoUpdates := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-GUARD")
	if errNoUpdates == nil || (!strings.Contains(errNoUpdates.Error(), "no updates provided") && !strings.Contains(errNoUpdates.Error(), "no data provided")) {
		t.Errorf("expected 'no updates provided' or 'no data provided' error, got: %v", errNoUpdates)
	}

	// 7. Missing ID error
	_, errNoID := executeObjectCommand(t, tempDir, provider, "update")
	if errNoID == nil || !strings.Contains(errNoID.Error(), "object ID is required") {
		t.Errorf("expected 'object ID is required' error, got: %v", errNoID)
	}
}

func TestInProcess_Update_DataAndFile(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	bli := plannedBLI("BLI-UPD-DATA", "Data Update Item", map[string]any{
		objects.FieldKeyDescription: "Initial description",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli, objectStatusPlanned)

	// 1. Update via --data JSON
	outData, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-DATA", "--data", `{"description":"Updated via inline data"}`)
	if err != nil {
		t.Fatalf("update with --data failed: %v\noutput: %s", err, outData)
	}
	readObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-DATA")
	if err != nil {
		t.Fatalf("failed to read after --data update: %v", err)
	}
	if desc, _ := readObj[objects.FieldKeyDescription].(string); desc != "Updated via inline data" {
		t.Errorf("expected description updated via data, got: %v", desc)
	}

	// 2. Update via --file
	updateFilePath := filepath.Join(tempDir, "update_file.yaml")
	fileContent := "description: Updated via file update\nnotes: Added notes via file\n"
	if err := fileutil.WriteFile(updateFilePath, []byte(fileContent), 0644); err != nil {
		t.Fatalf("failed to write update file: %v", err)
	}

	outFile, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-DATA", "--file", updateFilePath)
	if err != nil {
		t.Fatalf("update with --file failed: %v\noutput: %s", err, outFile)
	}
	readObj, err = provider.Read(stdctx.Background(), secCtx, "BLI-UPD-DATA")
	if err != nil {
		t.Fatalf("failed to read after --file update: %v", err)
	}
	if desc, _ := readObj[objects.FieldKeyDescription].(string); desc != "Updated via file update" {
		t.Errorf("expected description updated via file, got: %v", desc)
	}
	if notes, _ := readObj["notes"].(string); notes != "Added notes via file" {
		t.Errorf("expected notes updated via file, got: %v", notes)
	}
}

func TestInProcess_Update_AddAndRemoveRef(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed requirement target
	reqObj := map[string]any{
		objects.FieldKeyID:            "REQ-UPD-TGT",
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeyTitle:         "Ref Target Requirement",
		objects.FieldKeyDescription:   "Description",
		objects.FieldKeyPriority:      "p1",
		objects.FieldKeyStatus:        "complete",
		objects.FieldKeyGoalRefs:      []string{},
		objects.FieldKeyCriteriaRefs:  []string{bulkFixtureCriteriaID},
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, reqObj, "complete")

	bli := plannedBLI("BLI-UPD-REFS", "Ref Item", map[string]any{
		objects.FieldKeyRequirementRefs: []string{},
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bli, objectStatusPlanned)

	// 1. Add ref
	outAdd, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-REFS", "--add-ref", "requirement_refs=REQ-UPD-TGT")
	if err != nil {
		t.Fatalf("update --add-ref failed: %v\noutput: %s", err, outAdd)
	}
	readObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-REFS")
	if err != nil {
		t.Fatalf("failed to read after add-ref: %v", err)
	}
	reqRefs := getRefSlice(readObj, nil, objects.FieldKeyRequirementRefs)
	if len(reqRefs) != 1 || reqRefs[0] != "REQ-UPD-TGT" {
		t.Errorf("expected requirement_refs to have REQ-UPD-TGT, got: %v", reqRefs)
	}

	// 2. Remove ref
	outRm, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-REFS", "--remove-ref", "requirement_refs=REQ-UPD-TGT")
	if err != nil {
		t.Fatalf("update --remove-ref failed: %v\noutput: %s", err, outRm)
	}
	readObj, err = provider.Read(stdctx.Background(), secCtx, "BLI-UPD-REFS")
	if err != nil {
		t.Fatalf("failed to read after remove-ref: %v", err)
	}
	reqRefs = getRefSlice(readObj, nil, objects.FieldKeyRequirementRefs)
	if len(reqRefs) != 0 {
		t.Errorf("expected empty requirement_refs after remove-ref, got: %v", reqRefs)
	}
}

func TestInProcess_Update_AutoStatus(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Backlog item starts at "conceptual"
	bliObj := plannedBLI("BLI-UPD-AUTO", "Auto Status Item", map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusConceptual,
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, bliObj, objects.ObjectStatusConceptual)

	// Update with --auto-status
	outAuto, err := executeObjectCommand(t, tempDir, provider, "update", "BLI-UPD-AUTO", "--auto-status")
	if err != nil {
		t.Fatalf("update with --auto-status failed: %v\noutput: %s", err, outAuto)
	}
	readObj, err := provider.Read(stdctx.Background(), secCtx, "BLI-UPD-AUTO")
	if err != nil {
		t.Fatalf("failed to read after auto-status: %v", err)
	}
	status, _ := readObj[objects.FieldKeyStatus].(string)
	if status != "originated" {
		t.Errorf("expected status to advance from conceptual to originated with --auto-status, got: %s", status)
	}
}

func TestInProcess_Update_UpdateAll(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// Seed 2 backlog items
	b1 := plannedBLI("BLI-ALL-1", "All 1", map[string]any{objects.FieldKeyPriority: "low"})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b1, objectStatusPlanned)

	b2 := plannedBLI("BLI-ALL-2", "All 2", map[string]any{objects.FieldKeyPriority: "low"})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b2, objectStatusPlanned)

	// 1. Missing --kind flag with --all fails
	_, errNoKind := executeObjectCommand(t, tempDir, provider, "update", "--all", "--field", "notes=Test")
	if errNoKind == nil {
		t.Error("expected error when --kind is missing with --all")
	}

	// 2. Dry run with --all
	outDry, errDry := executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item", "--field", "notes=DryAll", "--dry-run")
	if errDry != nil {
		t.Fatalf("update --all dry-run failed: %v\noutput: %s", errDry, outDry)
	}
	if !strings.Contains(outDry, "Dry-run mode") && !strings.Contains(outDry, "dry-run") {
		t.Logf("dry run output: %s", outDry)
	}

	// 3. Update all with filter and --add-missing-fields
	outAll, errAll := executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item",
		"--filter", "priority=low",
		"--field", "notes=Batch updated notes",
		"--add-missing-fields",
	)
	if errAll != nil {
		t.Fatalf("update --all failed: %v\noutput: %s", errAll, outAll)
	}
	if !strings.Contains(outAll, "Update complete for kind backlog_item") {
		t.Errorf("expected update complete message, got: %s", outAll)
	}

	o1, err := provider.Read(stdctx.Background(), secCtx, "BLI-ALL-1")
	if err != nil || o1["notes"] != "Batch updated notes" {
		t.Errorf("expected obj1 notes to be updated via update --all, got: %v", o1["notes"])
	}
	o2, err := provider.Read(stdctx.Background(), secCtx, "BLI-ALL-2")
	if err != nil || o2["notes"] != "Batch updated notes" {
		t.Errorf("expected obj2 notes to be updated via update --all, got: %v", o2["notes"])
	}

	// 4. Update all with no matching objects
	outNone, errNone := executeObjectCommand(t, tempDir, provider, "update", "--all", "--kind", "backlog_item",
		"--filter", "priority=critical_nonexistent",
		"--field", "notes=None",
	)
	if errNone != nil {
		t.Fatalf("expected clean response when no objects match filter, got: %v", errNone)
	}
	if !strings.Contains(outNone, "No objects of kind") {
		t.Errorf("expected 'No objects of kind' message, got: %s", outNone)
	}
}

func TestUpdateHelpers_ParsingAndUnset(t *testing.T) {
	// 1. parseRefFlag tests
	field, refVal, err := parseRefFlag("milestone_refs=MIL-001")
	if err != nil || field != "milestone_refs" || refVal != "MIL-001" {
		t.Errorf("parseRefFlag = (%s, %s, %v), want (milestone_refs, MIL-001, nil)", field, refVal, err)
	}

	fieldCol, refValCol, errCol := parseRefFlag("priority_plan_ref:PRI-002")
	if errCol != nil || fieldCol != "priority_plan_ref" || refValCol != "PRI-002" {
		t.Errorf("parseRefFlag with colon = (%s, %s, %v), want (priority_plan_ref, PRI-002, nil)", fieldCol, refValCol, errCol)
	}

	_, _, errBad := parseRefFlag("invalid_flag_format")
	if errBad == nil {
		t.Error("expected error parsing invalid ref flag format")
	}

	// 2. applyUnsetFieldFlags tests
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("unset-field", []string{"id", "kind", "created_at", "notes", "custom_meta", "   "}, "unset field")
	updates := map[string]any{"notes": "to be removed", "title": "keep"}
	applyUnsetFieldFlags(cmd, updates)

	// Immutable fields id, kind, created_at must NOT be set to storage.FieldUnset
	if _, ok := updates["id"]; ok {
		t.Error("expected 'id' not to be set in updates")
	}
	if _, ok := updates["kind"]; ok {
		t.Error("expected 'kind' not to be set in updates")
	}
	if _, ok := updates["created_at"]; ok {
		t.Error("expected 'created_at' not to be set in updates")
	}
	if updates["notes"] != storage.FieldUnset {
		t.Errorf("expected notes to be set to FieldUnset, got: %v", updates["notes"])
	}
	if updates["custom_meta"] != storage.FieldUnset {
		t.Errorf("expected custom_meta to be set to FieldUnset, got: %v", updates["custom_meta"])
	}

	// 3. shouldUseBulkInvalidation tests
	if shouldUseBulkInvalidation(10, 10, true) {
		t.Error("dry-run should never use bulk invalidation")
	}
	if shouldUseBulkInvalidation(10, 0, false) {
		t.Error("totalObjects 0 should not use bulk invalidation")
	}
	if shouldUseBulkInvalidation(1, 10, false) {
		t.Error("1/10 (10%) should not trigger bulk invalidation (>30% required)")
	}
	if !shouldUseBulkInvalidation(4, 10, false) {
		t.Error("4/10 (40%) should trigger bulk invalidation")
	}
}

func TestUpdateHelpers_BulkAndValidation(t *testing.T) {
	// 4. formatUpdateAllResults tests
	resSuccess := formatUpdateAllResults("backlog_item", 5, 0, nil, false)
	if !strings.Contains(string(resSuccess), "Success: 5") {
		t.Errorf("unexpected formatUpdateAllResults: %s", string(resSuccess))
	}

	resErrors := formatUpdateAllResults("backlog_item", 3, 2, []string{"err1", "err2"}, true)
	if !strings.Contains(string(resErrors), "Errors: 2") || !strings.Contains(string(resErrors), "Dry-run mode") {
		t.Errorf("unexpected formatUpdateAllResults with errors/dry-run: %s", string(resErrors))
	}

	// 5. countObjectsToUpdate tests
	queryRes := &storage.QueryResult{
		Objects: []map[string]any{
			{"id": "BLI-1", "title": "Item 1"},
			{"id": "", "title": "Missing ID"},
			{"id": "BLI-2", "title": "Item 2"},
		},
	}
	count := countObjectsToUpdate(queryRes, "backlog_item", map[string]any{"notes": "test"}, false, nil)
	if count != 2 {
		t.Errorf("countObjectsToUpdate = %d, want 2", count)
	}

	// 6. getMissingFields tests
	objWithNS := map[string]any{"id": "BLI-1", "namespace_id": "zqk:custom"}
	missingWithNS := getMissingFields(objWithNS, "backlog_item", nil)
	if len(missingWithNS) != 0 {
		t.Errorf("expected no missing fields for object with namespace_id, got: %v", missingWithNS)
	}

	objWithoutNS := map[string]any{"id": "BLI-1"}
	missingWithoutNS := getMissingFields(objWithoutNS, "backlog_item", nil)
	if _, ok := missingWithoutNS[objects.FieldKeyNamespaceID]; !ok {
		t.Error("expected namespace_id to be added for object without namespace_id")
	}

	// 7. validateFileHash tests
	// Non-hash filename (short or non-.yaml) should pass
	if err := validateFileHash("some/path/file.txt"); err != nil {
		t.Errorf("expected non-hash file to pass validation, got: %v", err)
	}
	// Non-hex 64-char filename
	fakeNonHex := strings.Repeat("z", 64) + ".yaml"
	if err := validateFileHash(fakeNonHex); err != nil {
		t.Errorf("expected non-hex file to pass validation, got: %v", err)
	}
}

func TestTypeConversion_NumbersAndFloats(t *testing.T) {
	// 1. normalizeValueForField with nil fieldInfo
	val, err := normalizeValueForField("test", nil)
	if err != nil || val != "test" {
		t.Errorf("expected unchanged value for nil fieldInfo, got (%v, %v)", val, err)
	}

	// 2. Number / Float conversions
	fNumber := &objects.FieldInfo{Type: "number"}
	// int -> float64
	vNumInt, err := normalizeValueForField(42, fNumber)
	if err != nil {
		t.Fatalf("unexpected error normalizing int: %v", err)
	}
	if f, ok := vNumInt.(float64); !ok || f != 42.0 {
		t.Errorf("expected float64 42.0, got: %T %v", vNumInt, vNumInt)
	}
	// int32 -> float64
	vNumInt32, err := normalizeValueForField(int32(10), fNumber)
	if err != nil {
		t.Fatalf("unexpected error normalizing int32: %v", err)
	}
	if f, ok := vNumInt32.(float64); !ok || f != 10.0 {
		t.Errorf("expected float64 10.0, got: %T %v", vNumInt32, vNumInt32)
	}
	// int64 -> float64
	vNumInt64, err := normalizeValueForField(int64(100), fNumber)
	if err != nil {
		t.Fatalf("unexpected error normalizing int64: %v", err)
	}
	if f, ok := vNumInt64.(float64); !ok || f != 100.0 {
		t.Errorf("expected float64 100.0, got: %T %v", vNumInt64, vNumInt64)
	}
	// float32 -> float64
	vNumFloat32, err := normalizeValueForField(float32(3.14), fNumber)
	if err != nil {
		t.Fatalf("unexpected error normalizing float32: %v", err)
	}
	if _, ok := vNumFloat32.(float64); !ok {
		t.Errorf("expected float64, got: %T %v", vNumFloat32, vNumFloat32)
	}
	// float64 -> float64
	vNumFloat64, err := normalizeValueForField(float64(5.5), fNumber)
	if err != nil {
		t.Fatalf("unexpected error normalizing float64: %v", err)
	}
	if f, ok := vNumFloat64.(float64); !ok || f != 5.5 {
		t.Errorf("expected float64 5.5, got: %T %v", vNumFloat64, vNumFloat64)
	}
	// string -> float64
	vNumStr, err := normalizeValueForField("123.45", fNumber)
	if err != nil {
		t.Fatalf("unexpected error normalizing string to float64: %v", err)
	}
	if f, ok := vNumStr.(float64); !ok || f != 123.45 {
		t.Errorf("expected float64 123.45, got: %T %v", vNumStr, vNumStr)
	}
}

func TestTypeConversion_IntegersAndCollections(t *testing.T) {
	// 3. Integer conversions
	fInt := &objects.FieldInfo{Type: "integer"}
	// float64 whole number -> int
	vIntFloat, err := normalizeValueForField(float64(99.0), fInt)
	if err != nil {
		t.Fatalf("unexpected error normalizing float to int: %v", err)
	}
	if i, ok := vIntFloat.(int); !ok || i != 99 {
		t.Errorf("expected int 99, got: %T %v", vIntFloat, vIntFloat)
	}
	// int -> int
	vIntInt, err := normalizeValueForField(77, fInt)
	if err != nil {
		t.Fatalf("unexpected error normalizing int to int: %v", err)
	}
	if i, ok := vIntInt.(int); !ok || i != 77 {
		t.Errorf("expected int 77, got: %T %v", vIntInt, vIntInt)
	}

	// 4. List / Array conversions
	fList := &objects.FieldInfo{Type: "list"}
	sliceIn := []any{"a", "b"}
	vList, err := normalizeValueForField(sliceIn, fList)
	if err != nil {
		t.Fatalf("unexpected error normalizing list: %v", err)
	}
	if sl, ok := vList.([]any); !ok || len(sl) != 2 {
		t.Errorf("expected []any with 2 elements, got: %T %v", vList, vList)
	}

	// 5. Other types passthrough
	fOther := &objects.FieldInfo{Type: "string"}
	vStr, err := normalizeValueForField("passthrough", fOther)
	if err != nil {
		t.Fatalf("unexpected error normalizing string: %v", err)
	}
	if vStr != "passthrough" {
		t.Errorf("expected passthrough, got: %v", vStr)
	}

	// 6. normalizeObjectValues with nil kindFields
	if err := normalizeObjectValues(map[string]any{"a": 1}, nil); err != nil {
		t.Errorf("expected nil error for nil kindFields, got: %v", err)
	}
}

func TestUpdate_LifecycleAndHashHelpers(t *testing.T) {
	tempDir, provider, _, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	// 1. deriveNextLifecycleStatus
	stBli, err := deriveNextLifecycleStatus(map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusConceptual,
	})
	if err != nil || stBli != "originated" {
		t.Errorf("expected originated, got %s (err %v)", stBli, err)
	}

	stCrit, err := deriveNextLifecycleStatus(map[string]any{
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
	})
	if err != nil || stCrit != objectStatusInProgress {
		t.Errorf("expected in_progress, got %s (err %v)", stCrit, err)
	}

	// 2. validateFileHash
	content := []byte("field: value\n")
	contentHash := storage.CalculateSHA256Hash(content)
	matchingPath := filepath.Join(tempDir, contentHash+".yaml")
	if err := fileutil.WriteFile(matchingPath, content, 0644); err != nil {
		t.Fatalf("failed to write hash file: %v", err)
	}
	if err := validateFileHash(matchingPath); err != nil {
		t.Errorf("expected matching hash to pass, got: %v", err)
	}

	mismatchPath := filepath.Join(tempDir, strings.Repeat("e", 64)+".yaml")
	if err := fileutil.WriteFile(mismatchPath, content, 0644); err != nil {
		t.Fatalf("failed to write mismatch file: %v", err)
	}
	if err := validateFileHash(mismatchPath); err == nil {
		t.Error("expected error for mismatched hash")
	}

	// 3. buildUpdateCacheContext with ID change and normal update
	testCmd := &cobra.Command{}
	setTestCLIContext(t, testCmd, tempDir, provider)
	errProc := cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctxWithChange := buildUpdateCacheContext(proc, "OLD-ID", map[string]any{objects.FieldKeyID: "NEW-ID"}, "goal")
		if ctxWithChange == nil {
			t.Error("expected non-nil context for ID change")
		}
		ctxNormal := buildUpdateCacheContext(proc, "SAME-ID", map[string]any{objects.FieldKeyTitle: "New Title"}, "goal")
		if ctxNormal == nil {
			t.Error("expected non-nil context for normal update")
		}
		return nil
	})(testCmd, nil)
	if errProc != nil {
		t.Fatalf("processor failed: %v", errProc)
	}
}

func TestUpdate_EnforceActivePlanMembership(t *testing.T) {
	_, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	cliCtx := stdctx.Background()

	// 4. enforceActivePlanMembership - BLI not found
	errNotFound := enforceActivePlanMembership(cliCtx, secCtx, provider, "NON-EXISTENT")
	if errNotFound == nil || !strings.Contains(errNotFound.Error(), "failed to read backlog item") {
		t.Errorf("expected failed to read error, got: %v", errNotFound)
	}

	// 5. enforceActivePlanMembership - BLI has no priority_plan_ref
	bliNoPlan := plannedBLI("BLI-MEM-NOPLAN", "No Plan BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "",
	})
	storage.CreateCASVisible(t, provider, cliCtx, secCtx, bliNoPlan, objectStatusPlanned)
	errNoPlan := enforceActivePlanMembership(cliCtx, secCtx, provider, "BLI-MEM-NOPLAN")
	if errNoPlan == nil || !strings.Contains(errNoPlan.Error(), "has no priority_plan_ref") {
		t.Errorf("expected no priority_plan_ref error, got: %v", errNoPlan)
	}

	// 6. enforceActivePlanMembership - Plan not found
	bliMissingPlan := plannedBLI("BLI-MEM-MISPLAN", "Missing Plan BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-NONEXISTENT",
	})
	storage.CreateCASVisible(t, provider, cliCtx, secCtx, bliMissingPlan, objectStatusPlanned)
	errPlanMissing := enforceActivePlanMembership(cliCtx, secCtx, provider, "BLI-MEM-MISPLAN")
	if errPlanMissing == nil || !strings.Contains(errPlanMissing.Error(), "not found for backlog item") {
		t.Errorf("expected plan not found error, got: %v", errPlanMissing)
	}

	// 7. enforceActivePlanMembership - Plan status not execution-facing (e.g. originated)
	planObj := map[string]any{
		objects.FieldKeyID:            "PRI-MEM-ORIG",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Originated Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusOriginated,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, cliCtx, secCtx, planObj, objects.ObjectStatusOriginated)

	bliOrigPlan := plannedBLI("BLI-MEM-ORIGPLAN", "Orig Plan BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-MEM-ORIG",
	})
	storage.CreateCASVisible(t, provider, cliCtx, secCtx, bliOrigPlan, objectStatusPlanned)
	errNotExec := enforceActivePlanMembership(cliCtx, secCtx, provider, "BLI-MEM-ORIGPLAN")
	if errNotExec == nil {
		t.Error("expected error for non-execution-facing plan")
	}

	// 8. enforceActivePlanMembership - Active plan succeeds
	activePlan := map[string]any{
		objects.FieldKeyID:            "PRI-MEM-ACTIVE",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Active Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, cliCtx, secCtx, activePlan, objects.ObjectStatusActive)

	bliActivePlan := plannedBLI("BLI-MEM-ACTIVEPLAN", "Active Plan BLI", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-MEM-ACTIVE",
	})
	storage.CreateCASVisible(t, provider, cliCtx, secCtx, bliActivePlan, objectStatusPlanned)
	if errActive := enforceActivePlanMembership(cliCtx, secCtx, provider, "BLI-MEM-ACTIVEPLAN"); errActive != nil {
		t.Errorf("expected active plan membership to succeed, got: %v", errActive)
	}

	// 9. enforcePriorityOrder - No active plans returns nil
	if errPrio := enforcePriorityOrder(cliCtx, secCtx, provider, "BLI-MEM-ACTIVEPLAN"); errPrio != nil {
		t.Errorf("expected enforcePriorityOrder to succeed, got: %v", errPrio)
	}
}
