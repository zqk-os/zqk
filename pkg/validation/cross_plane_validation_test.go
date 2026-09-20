package validation

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestValidateCrossPlaneReferences_OptionsCallback(t *testing.T) {
	gv := NewGoValidator()

	draftOnlyID := "CRIT-draft-only-001"
	casID := "CRIT-cas-backed-001"

	opts := &ValidationOptions{
		ValidateLifecycle:     false,
		ValidateSemanticTypes: false,
		IsDraftPlaneOnly: func(id string) bool {
			return id == draftOnlyID
		},
	}

	// 1. CAS requirement referencing draft-plane-only criterion -> must FAIL
	casReq := map[string]any{
		objects.FieldKeyID:           "REQ-COMMUNITY-001",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       objects.ObjectStatusOriginated,
		objects.FieldKeyCriteriaRefs: []any{draftOnlyID},
	}
	errs := gv.validateCrossPlaneReferences(casReq, objects.KindRequirement, opts)
	if len(errs) == 0 {
		t.Fatalf("expected cross-plane validation error for CAS requirement referencing draft criterion, got none")
	}
	if errs[0].Rule != "no_draft_plane_refs_from_cas" {
		t.Errorf("expected rule 'no_draft_plane_refs_from_cas', got %q", errs[0].Rule)
	}

	// 2. CAS requirement referencing CAS criterion -> must PASS
	casReqValid := map[string]any{
		objects.FieldKeyID:           "REQ-COMMUNITY-001",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       objects.ObjectStatusOriginated,
		objects.FieldKeyCriteriaRefs: []any{casID},
	}
	errsValid := gv.validateCrossPlaneReferences(casReqValid, objects.KindRequirement, opts)
	if len(errsValid) != 0 {
		t.Fatalf("expected no errors for CAS requirement referencing CAS criterion, got: %v", errsValid)
	}

	// 3. Draft-plane requirement referencing draft criterion -> must PASS
	draftReq := map[string]any{
		objects.FieldKeyID:           "REQ-DRAFT-001",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       objects.ObjectStatusConceptual,
		objects.FieldKeyCriteriaRefs: []any{draftOnlyID},
	}
	errsDraft := gv.validateCrossPlaneReferences(draftReq, objects.KindRequirement, opts)
	if len(errsDraft) != 0 {
		t.Fatalf("expected no errors for draft requirement referencing draft criterion, got: %v", errsDraft)
	}

	// 4. Draft criteria referencing CAS requirement -> must PASS
	draftCrit := map[string]any{
		objects.FieldKeyID:              draftOnlyID,
		objects.FieldKeyKind:            objects.KindCriteria,
		objects.FieldKeyStatus:          objects.ObjectStatusConceptual,
		objects.FieldKeyRequirementRefs: []any{"REQ-COMMUNITY-001"},
	}
	errsDraftCrit := gv.validateCrossPlaneReferences(draftCrit, objects.KindCriteria, opts)
	if len(errsDraftCrit) != 0 {
		t.Fatalf("expected no errors for draft criteria referencing CAS requirement, got: %v", errsDraftCrit)
	}
}

func TestValidateCrossPlaneReferences_FilesystemFallback(t *testing.T) {
	tmpDir := t.TempDir()
	draftID := "CRIT-fallback-draft-001"

	sum := sha256.Sum256([]byte(draftID))
	shard := fmt.Sprintf("%x", sum[:])[:2]

	draftFile := filepath.Join(tmpDir, paths.ProjectDataDir, paths.ObjectDraftsDir, objects.KindCriteria, shard, draftID+".yaml")
	if err := fileutil.MkdirAll(filepath.Dir(draftFile), 0755); err != nil {
		t.Fatalf("failed to create draft dir: %v", err)
	}
	if err := fileutil.WriteFile(draftFile, []byte("id: "+draftID+"\nkind: criteria\nstatus: conceptual\n"), 0644); err != nil {
		t.Fatalf("failed to write draft file: %v", err)
	}

	gv := NewGoValidator()
	opts := &ValidationOptions{
		ProjectRoot: tmpDir,
	}

	casBLI := map[string]any{
		objects.FieldKeyID:           "BLI-CAS-001",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       objects.ObjectStatusOriginated,
		objects.FieldKeyCriteriaRefs: []any{draftID},
	}

	errs := gv.validateCrossPlaneReferences(casBLI, objects.KindBacklogItem, opts)
	if len(errs) == 0 {
		t.Fatalf("expected cross-plane validation error via filesystem fallback, got none")
	}
	if errs[0].Rule != "no_draft_plane_refs_from_cas" {
		t.Errorf("expected rule 'no_draft_plane_refs_from_cas', got %q", errs[0].Rule)
	}
}
