package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// BLI-DRAFT-LAND-TEMPLATE-FILL-001 / CRIT-BLI-DRAFT-LAND-TEMPLATE-FILL-001-AUTO:
// When create (or --promote) fails to materialize into CAS and the object lands or
// remains on .zqk/object_drafts/, overwrite/merge the persisted draft with a fully
// compliant kind template, layering user supplied fields on top so user values win
// and missing required next-status slots appear as editable placeholders.

func TestWriteObjectToDraftPlane_FillsCompliantTemplate(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "BLI-1777000000000000000-tplfill01"
	sparseObj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Sparse draft item",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	sparseData, err := yaml.Marshal(sparseObj)
	if err != nil {
		t.Fatalf("marshal sparse object: %v", err)
	}

	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindBacklogItem, sparseData); err != nil {
		t.Fatalf("WriteObjectToDraftPlane failed: %v", err)
	}

	// 1. Verify file exists on draft plane
	draftPath := fileStorage.objectDraftPlanePath(objects.KindBacklogItem, id)
	data, err := fileutil.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft file: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal draft YAML: %v\nContent:\n%s", err, string(data))
	}

	// 2. Verify user supplied values win
	if parsed[objects.FieldKeyTitle] != "Sparse draft item" {
		t.Fatalf("expected title %q, got %v", "Sparse draft item", parsed[objects.FieldKeyTitle])
	}
	if parsed[objects.FieldKeyID] != id {
		t.Fatalf("expected id %q, got %v", id, parsed[objects.FieldKeyID])
	}
	if parsed[objects.FieldKeyStatus] != objects.ObjectStatusConceptual {
		t.Fatalf("expected status %q, got %v", objects.ObjectStatusConceptual, parsed[objects.FieldKeyStatus])
	}

	// 3. Verify required next-status template keys appear as editable placeholders
	expectedKeys := []string{
		objects.FieldKeyProblemStatement,
		objects.FieldKeyAcceptanceConsiderations,
		objects.FieldKeyPriority,
		objects.FieldKeyPriorityTier,
		objects.FieldKeyPriorityPlanRef,
		objects.FieldKeyMilestoneRefs,
		objects.FieldKeyCriteriaRefs,
		objects.FieldKeyRequirementRefs,
		objects.FieldKeyStakeholders,
		objects.FieldKeyEstimatedEffort,
	}

	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("expected draft to contain template key %q for next-status completion, but missing", key)
		}
	}

	// 4. Verify read via storage Read API also returns these template keys
	readObj, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("storage.Read draft object failed: %v", err)
	}
	for _, key := range expectedKeys {
		if _, ok := readObj[key]; !ok {
			t.Errorf("expected storage.Read to return template key %q, but missing", key)
		}
	}

	// 5. Verify no CAS hash file was written for preliminary draft
	kindDir := fileStorage.GetKindDir(objects.KindBacklogItem)
	if kindDir != "" {
		casFiles, _ := fileutil.ReadDir(kindDir)
		for _, f := range casFiles {
			if filepath.Ext(f.Name()) == ".yaml" {
				// Ensure none is our id
				content, _ := fileutil.ReadFile(filepath.Join(kindDir, f.Name()))
				var casObj map[string]any
				if err := yaml.Unmarshal(content, &casObj); err == nil && casObj[objects.FieldKeyID] == id {
					t.Fatalf("preliminary draft %s was materialized into CAS %s unexpectedly", id, f.Name())
				}
			}
		}
	}
}

func TestWriteObjectToDraftPlane_PreservesUserSuppliedFields(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	id := "BLI-1777000000000000000-tplfill02"
	userObj := map[string]any{
		objects.FieldKeyID:               id,
		objects.FieldKeyKind:             objects.KindBacklogItem,
		objects.FieldKeyTitle:            "User provided title",
		objects.FieldKeyStatus:           objects.ObjectStatusConceptual,
		objects.FieldKeyProblemStatement: "User supplied problem statement",
		objects.FieldKeyMilestoneRefs:    []any{"MIL-1777000000000000000-custom01"},
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
	}

	userData, err := yaml.Marshal(userObj)
	if err != nil {
		t.Fatalf("marshal user object: %v", err)
	}

	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindBacklogItem, userData); err != nil {
		t.Fatalf("WriteObjectToDraftPlane failed: %v", err)
	}

	draftPath := fileStorage.objectDraftPlanePath(objects.KindBacklogItem, id)
	data, err := fileutil.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft file: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal draft YAML: %v", err)
	}

	// User fields preserved
	if parsed[objects.FieldKeyTitle] != "User provided title" {
		t.Fatalf("user title overwritten: got %v", parsed[objects.FieldKeyTitle])
	}
	if parsed[objects.FieldKeyProblemStatement] != "User supplied problem statement" {
		t.Fatalf("user problem statement overwritten: got %v", parsed[objects.FieldKeyProblemStatement])
	}
	milestones, ok := parsed[objects.FieldKeyMilestoneRefs].([]any)
	if !ok || len(milestones) != 1 || milestones[0] != "MIL-1777000000000000000-custom01" {
		t.Fatalf("user milestone_refs overwritten or corrupted: got %v", parsed[objects.FieldKeyMilestoneRefs])
	}

	// Missing fields filled with placeholders
	if _, ok := parsed[objects.FieldKeyAcceptanceConsiderations]; !ok {
		t.Errorf("expected missing acceptance_considerations to be filled from template")
	}
	if _, ok := parsed[objects.FieldKeyCriteriaRefs]; !ok {
		t.Errorf("expected missing criteria_refs to be filled from template")
	}
}

func TestDraftPlane_PromoteStickFailsClosedUntilValid(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "BLI-1777000000000000000-tplfill03"
	sparseObj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Incomplete draft item",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	sparseData, err := yaml.Marshal(sparseObj)
	if err != nil {
		t.Fatalf("marshal sparse object: %v", err)
	}

	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindBacklogItem, sparseData); err != nil {
		t.Fatalf("WriteObjectToDraftPlane failed: %v", err)
	}

	// Attempting to promote/update to "planned" while placeholders are unfilled must fail closed
	err = fileStorage.Update(ctx, secCtx, id, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
	})
	if err == nil {
		t.Fatalf("expected promote/update to planned to fail closed due to empty placeholders, but succeeded")
	}

	// Draft remains on draft plane
	if !fileStorage.objectDraftPlaneExists(objects.KindBacklogItem, id) {
		t.Fatalf("expected draft to remain on draft plane after failed promote")
	}
}

func TestWriteObjectToDraftPlane_FullTemplateOfPossiblesAndMembraneTimestamps(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "BLI-1777000000000000000-tplfill04"
	sparseObj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Full possibles draft test",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	sparseData, err := yaml.Marshal(sparseObj)
	if err != nil {
		t.Fatalf("marshal sparse object: %v", err)
	}

	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindBacklogItem, sparseData); err != nil {
		t.Fatalf("WriteObjectToDraftPlane failed: %v", err)
	}

	draftPath := fileStorage.objectDraftPlanePath(objects.KindBacklogItem, id)
	data, err := fileutil.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft file: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal draft YAML: %v", err)
	}

	// 1. Verify membrane provenance fields are NOT on the draft plane
	for _, membraneKey := range []string{objects.FieldKeyCreatedAt, objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedAt, objects.FieldKeyUpdatedBy, "hash"} {
		if val, exists := parsed[membraneKey]; exists && val != "" {
			t.Errorf("expected membrane key %q to be absent from draft plane, but found %v", membraneKey, val)
		}
	}

	// 2. Verify non-required spec fields appear as placeholders (full template of possibles).
	// epic_refs is intentionally omitted when empty — empty [] blocks conceptual→originated
	for _, expectedSlot := range []string{"persona_refs", "workstream_refs", "code_location", "commit_hashes"} {
		if _, exists := parsed[expectedSlot]; !exists {
			t.Errorf("expected full template of possibles to include %q on draft plane, but missing", expectedSlot)
		}
	}
	if _, exists := parsed[objects.FieldKeyEpicRefs]; exists {
		t.Errorf("empty epic_refs must be omitted from draft plane (got %#v)", parsed[objects.FieldKeyEpicRefs])
	}

	// 3. Complete fields and promote to originated (crossing CAS membrane)
	validUpdates := map[string]any{
		objects.FieldKeyStatus:      objects.ObjectStatusOriginated,
		objects.FieldKeyDescription: "Valid description for originated object",
	}

	if err := fileStorage.Update(ctx, secCtx, id, validUpdates); err != nil {
		t.Fatalf("update/promote to originated failed: %v", err)
	}

	// 4. Verify draft file was removed after CAS materialization
	if fileStorage.objectDraftPlaneExists(objects.KindBacklogItem, id) {
		t.Fatalf("draft still exists on draft plane after materializing to CAS")
	}

	// 5. Verify CAS object now has created_at and created_by stamped
	casObj, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("storage.Read materialized CAS object failed: %v", err)
	}
	if casObj[objects.FieldKeyCreatedAt] == nil || casObj[objects.FieldKeyCreatedAt] == "" {
		t.Errorf("expected created_at to be stamped upon CAS origination, but was empty")
	}
	if casObj[objects.FieldKeyCreatedBy] == nil || casObj[objects.FieldKeyCreatedBy] == "" {
		t.Errorf("expected created_by to be stamped upon CAS origination, but was empty")
	}
}

// PRI mint must not inject epic_refs: [] (blocks conceptual→originated under StrictMode).
func TestWriteObjectToDraftPlane_PriorityPlanOmitsEmptyEpicRefs(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	id := "PRI-1777000000000000000-epicomit01"
	sparseObj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "PRI omit empty epic_refs",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyPersonaRefs:   []any{"PER-DEFAULT-OPERATOR"},
	}
	sparseData, err := yaml.Marshal(sparseObj)
	if err != nil {
		t.Fatalf("marshal sparse object: %v", err)
	}
	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindPriorityPlan, sparseData); err != nil {
		t.Fatalf("WriteObjectToDraftPlane failed: %v", err)
	}

	draftPath := fileStorage.objectDraftPlanePath(objects.KindPriorityPlan, id)
	data, err := fileutil.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft file: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal draft YAML: %v", err)
	}
	if _, exists := parsed[objects.FieldKeyEpicRefs]; exists {
		t.Fatalf("priority_plan draft must omit empty epic_refs; got %#v", parsed[objects.FieldKeyEpicRefs])
	}
}

// Issue #935: agent_task draft plane must include artifacts: [] to satisfy deliverable latch.
func TestWriteObjectToDraftPlane_AgentTaskIncludesArtifacts(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	id := "ATK-1777000000000000000-arttest01"
	sparseObj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeyTitle:         "Agent task draft fill artifacts",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	sparseData, err := yaml.Marshal(sparseObj)
	if err != nil {
		t.Fatalf("marshal sparse object: %v", err)
	}
	if err := fileStorage.WriteObjectToDraftPlane(id, objects.KindAgentTask, sparseData); err != nil {
		t.Fatalf("WriteObjectToDraftPlane failed: %v", err)
	}

	draftPath := fileStorage.objectDraftPlanePath(objects.KindAgentTask, id)
	data, err := fileutil.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft file: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal draft YAML: %v", err)
	}
	val, exists := parsed[objects.FieldKeyArtifacts]
	if !exists {
		t.Fatalf("agent_task draft must contain artifacts field; got %#v", parsed)
	}
	artifactsSlice, ok := val.([]any)
	if !ok {
		t.Fatalf("expected artifacts to be a slice, got %T: %#v", val, val)
	}
	if len(artifactsSlice) != 0 {
		t.Fatalf("expected initial artifacts slice to be empty, got %d items", len(artifactsSlice))
	}
}

// GH-944: Optional reference fields must not scaffold empty string ("") on draft plane.
func TestWriteObjectToDraftPlane_GH944_OmitsOptionalReferenceEmptyStrings(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	// Test 1: priority_plan draft plane omits optional scalar references like release_ref & team_configuration_ref
	planID := "PRI-1777000000000000000-gh944plan01"
	sparsePlan := map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "GH-944 Plan draft",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	planBytes, err := yaml.Marshal(sparsePlan)
	require.NoError(t, err)
	err = fileStorage.WriteObjectToDraftPlane(planID, objects.KindPriorityPlan, planBytes)
	require.NoError(t, err)

	draftPlanPath := fileStorage.objectDraftPlanePath(objects.KindPriorityPlan, planID)
	data, err := fileutil.ReadFile(draftPlanPath)
	require.NoError(t, err)

	var parsedPlan map[string]any
	err = yaml.Unmarshal(data, &parsedPlan)
	require.NoError(t, err)

	if v, exists := parsedPlan["release_ref"]; exists && v == "" {
		t.Errorf("release_ref must not scaffold empty string: got %v", v)
	}
	if v, exists := parsedPlan["team_configuration_ref"]; exists && v == "" {
		t.Errorf("team_configuration_ref must not scaffold empty string: got %v", v)
	}

	// Test 2: backlog_item priority_plan_ref is null, not empty string
	bliID := "BLI-1777000000000000000-gh944bli01"
	sparseBli := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "GH-944 BLI draft",
		objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	bliBytes, err := yaml.Marshal(sparseBli)
	require.NoError(t, err)
	err = fileStorage.WriteObjectToDraftPlane(bliID, objects.KindBacklogItem, bliBytes)
	require.NoError(t, err)

	draftBliPath := fileStorage.objectDraftPlanePath(objects.KindBacklogItem, bliID)
	bliData, err := fileutil.ReadFile(draftBliPath)
	require.NoError(t, err)

	var parsedBli map[string]any
	err = yaml.Unmarshal(bliData, &parsedBli)
	require.NoError(t, err)

	if v, exists := parsedBli[objects.FieldKeyPriorityPlanRef]; exists && v != nil {
		t.Errorf("expected priority_plan_ref to be null (nil), got %#v", v)
	}
}
