package storage

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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

