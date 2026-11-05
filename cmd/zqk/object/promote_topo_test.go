package object

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestPromoteFlags_CascadeAndFamily(t *testing.T) {
	t.Parallel()

	// 1. Check NewPromoteCmd
	promoteCmd := NewPromoteCmd()
	cascadeFlag := promoteCmd.Flags().Lookup("cascade")
	if cascadeFlag == nil {
		t.Fatal("expected --cascade flag on object promote command")
	}
	if cascadeFlag.DefValue != "false" {
		t.Fatalf("expected --cascade default value false, got %s", cascadeFlag.DefValue)
	}

	familyFlag := promoteCmd.Flags().Lookup("family")
	if familyFlag == nil {
		t.Fatal("expected --family flag on object promote command")
	}
	if familyFlag.DefValue != "false" {
		t.Fatalf("expected --family default value false, got %s", familyFlag.DefValue)
	}

	// 2. Check NewDraftPromoteCmd
	draftPromoteCmd := NewDraftPromoteCmd()
	draftCascadeFlag := draftPromoteCmd.Flags().Lookup("cascade")
	if draftCascadeFlag == nil {
		t.Fatal("expected --cascade flag on object draft promote command")
	}
	draftFamilyFlag := draftPromoteCmd.Flags().Lookup("family")
	if draftFamilyFlag == nil {
		t.Fatal("expected --family flag on object draft promote command")
	}
}

func TestSortObjectIDsForMembranePromotion_ReverseOrder(t *testing.T) {
	_ = SetupTestEnvironment(t)

	ctx := context.Background()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	secCtx := proc.SecurityContext()

	// Create objects on draft plane (status: conceptual)
	// CRI -> REQ -> BLI
	criID := "CRI-SORT-001"
	reqID := "REQ-SORT-001"
	bliID := "BLI-SORT-001"

	criObj := map[string]any{
		objects.FieldKeyID:       criID,
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyTitle:    "Test Criteria 001",
		objects.FieldKeyStatus:   "conceptual",
		objects.FieldKeyCategory: "acceptance",
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Test Requirement 001",
		objects.FieldKeyStatus:       "conceptual",
		objects.FieldKeyDescription:  "Detailed requirement description",
		objects.FieldKeyCriteriaRefs: []string{criID},
	}
	bliObj := map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Test Backlog Item 001",
		objects.FieldKeyStatus:          "conceptual",
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    []string{criID},
	}

	for _, obj := range []map[string]any{criObj, reqObj, bliObj} {
		if err := proc.Storage().Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Given reverse input: BLI, REQ, CRI
	input := []string{bliID, reqID, criID}
	sorted, err := sortObjectIDsForMembranePromotion(ctx, secCtx, proc, input)
	if err != nil {
		t.Fatalf("sortObjectIDsForMembranePromotion failed: %v", err)
	}

	expected := []string{criID, reqID, bliID}
	if len(sorted) != len(expected) {
		t.Fatalf("expected len %d, got %d: %v", len(expected), len(sorted), sorted)
	}
	for i, exp := range expected {
		if sorted[i] != exp {
			t.Errorf("at index %d: expected %s, got %s (full: %v)", i, exp, sorted[i], sorted)
		}
	}
}

func TestSortObjectIDsForMembranePromotion_CycleHandling(t *testing.T) {
	_ = SetupTestEnvironment(t)

	ctx := context.Background()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	secCtx := proc.SecurityContext()

	// Mutual references: CRI <-> REQ
	criID := "CRI-CYC-001"
	reqID := "REQ-CYC-001"

	criObj := map[string]any{
		objects.FieldKeyID:                criID,
		objects.FieldKeyKind:              objects.KindCriteria,
		objects.FieldKeyTitle:             "Cycle Criteria 001",
		objects.FieldKeyStatus:            "conceptual",
		objects.FieldKeyCategory:          "acceptance",
		objects.FieldKeyRelatedObjectRefs: []string{reqID},
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Cycle Requirement 001",
		objects.FieldKeyStatus:       "conceptual",
		objects.FieldKeyDescription:  "Detailed requirement description",
		objects.FieldKeyCriteriaRefs: []string{criID},
	}

	for _, obj := range []map[string]any{criObj, reqObj} {
		if err := proc.Storage().Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Sort should gracefully break cycle and prioritize criteria over requirement
	input := []string{reqID, criID}
	sorted, err := sortObjectIDsForMembranePromotion(ctx, secCtx, proc, input)
	if err != nil {
		t.Fatalf("sortObjectIDsForMembranePromotion failed on cycle: %v", err)
	}

	if len(sorted) != 2 {
		t.Fatalf("expected 2 items, got %d: %v", len(sorted), sorted)
	}
	// Criteria has lower rank (10) than Requirement (20), so Criteria should come first
	if sorted[0] != criID || sorted[1] != reqID {
		t.Fatalf("expected [CRI, REQ] after breaking cycle, got %v", sorted)
	}
}

func TestExpandCascadeDraftDependencies(t *testing.T) {
	_ = SetupTestEnvironment(t)

	ctx := context.Background()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	secCtx := proc.SecurityContext()

	// Setup: REQ references CRI (both in draft plane)
	criID := "CRI-CASC-001"
	reqID := "REQ-CASC-001"

	criObj := map[string]any{
		objects.FieldKeyID:       criID,
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyTitle:    "Cascade Criteria 001",
		objects.FieldKeyStatus:   "conceptual",
		objects.FieldKeyCategory: "acceptance",
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Cascade Requirement 001",
		objects.FieldKeyStatus:       "conceptual",
		objects.FieldKeyDescription:  "Detailed requirement description",
		objects.FieldKeyCriteriaRefs: []string{criID},
	}

	for _, obj := range []map[string]any{criObj, reqObj} {
		if err := proc.Storage().Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Expand from seed REQ only
	expanded, err := expandCascadeDraftDependencies(ctx, secCtx, proc, []string{reqID})
	if err != nil {
		t.Fatalf("expandCascadeDraftDependencies failed: %v", err)
	}

	// Expanded should contain both REQ and CRI
	hasReq, hasCri := false, false
	for _, id := range expanded {
		if id == reqID {
			hasReq = true
		}
		if id == criID {
			hasCri = true
		}
	}
	if !hasReq || !hasCri {
		t.Fatalf("expected both %s and %s in expanded list, got %v", reqID, criID, expanded)
	}

	// When sorted topologically, CRI must precede REQ
	sorted, err := sortObjectIDsForMembranePromotion(ctx, secCtx, proc, expanded)
	if err != nil {
		t.Fatalf("sort failed: %v", err)
	}
	if sorted[0] != criID || sorted[1] != reqID {
		t.Fatalf("expected [%s, %s], got %v", criID, reqID, sorted)
	}
}

func TestObjectPromote_TopologicalOrderPreventsCrossPlaneErrors(t *testing.T) {
	_ = SetupTestEnvironment(t)

	ctx := context.Background()
	promoteCmd := NewPromoteCmd()
	promoteCmd.SetContext(ctx)

	proc, err := cli.NewProcessor(promoteCmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	secCtx := proc.SecurityContext()

	// Create interconnected draft objects
	criID := "CRI-E2E-001"
	reqID := "REQ-E2E-001"
	bliID := "BLI-E2E-001"

	criObj := map[string]any{
		objects.FieldKeyID:          criID,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "E2E Criteria 001",
		objects.FieldKeyStatus:      "conceptual",
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyDescription: "Comprehensive acceptance criterion for E2E testing.",
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "E2E Requirement 001",
		objects.FieldKeyStatus:       "conceptual",
		objects.FieldKeyDescription:  "Detailed requirement description meeting all invariants.",
		objects.FieldKeyPriority:     "p1",
		objects.FieldKeyCriteriaRefs: []string{criID},
	}
	bliObj := map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "E2E Backlog Item 001",
		objects.FieldKeyDescription:     "Valid backlog item description for promotion.",
		objects.FieldKeyStatus:          "conceptual",
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    []string{criID},
	}

	for _, obj := range []map[string]any{criObj, reqObj, bliObj} {
		if err := proc.Storage().Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Verify all are in draft plane initially
	for _, id := range []string{criID, reqID, bliID} {
		if !storage.IsDraftPlaneOnly(proc.ProjectRoot(), "", id) {
			t.Fatalf("expected %s to be draft-plane-only initially", id)
		}
	}

	// Promote with reverse order passed to promoteObjectIDs
	// If topological sort is missing, promoting BLI first will fail cross-plane check!
	err = promoteObjectIDs(promoteCmd, proc, []string{bliID, reqID, criID})
	if err != nil {
		t.Fatalf("promoteObjectIDs failed: %v", err)
	}

	// Verify all three objects crossed the CAS membrane into 'originated'
	for _, id := range []string{criID, reqID, bliID} {
		obj, err := proc.Storage().Read(ctx, secCtx, id)
		if err != nil {
			t.Fatalf("Read %s: %v", id, err)
		}
		status := obj[objects.FieldKeyStatus]
		if status != "originated" {
			t.Errorf("expected status 'originated' for %s, got '%v'", id, status)
		}
		if storage.IsDraftPlaneOnly(proc.ProjectRoot(), "", id) {
			t.Errorf("expected %s to have crossed CAS membrane, still on draft plane", id)
		}
	}
}

func TestObjectPromote_CascadeFlagPromotesChildDrafts(t *testing.T) {
	_ = SetupTestEnvironment(t)

	ctx := context.Background()
	promoteCmd := NewPromoteCmd()
	_ = promoteCmd.Flags().Set("cascade", "true")
	promoteCmd.SetContext(ctx)

	proc, err := cli.NewProcessor(promoteCmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	secCtx := proc.SecurityContext()

	// Create draft requirement and criterion
	criID := "CRI-CASC-E2E"
	reqID := "REQ-CASC-E2E"

	criObj := map[string]any{
		objects.FieldKeyID:          criID,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Cascade E2E Criteria",
		objects.FieldKeyStatus:      "conceptual",
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyDescription: "Comprehensive acceptance criterion for cascade testing.",
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Cascade E2E Requirement",
		objects.FieldKeyStatus:       "conceptual",
		objects.FieldKeyDescription:  "Detailed requirement description meeting all invariants.",
		objects.FieldKeyPriority:     "p1",
		objects.FieldKeyCriteriaRefs: []string{criID},
	}

	for _, obj := range []map[string]any{criObj, reqObj} {
		if err := proc.Storage().Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Pass only REQ with --cascade
	err = promoteObjectIDs(promoteCmd, proc, []string{reqID})
	if err != nil {
		t.Fatalf("promoteObjectIDs with --cascade failed: %v", err)
	}

	// Both REQ and CRI should be promoted to 'originated'
	for _, id := range []string{criID, reqID} {
		obj, err := proc.Storage().Read(ctx, secCtx, id)
		if err != nil {
			t.Fatalf("Read %s: %v", id, err)
		}
		if obj[objects.FieldKeyStatus] != "originated" {
			t.Errorf("expected status 'originated' for %s, got '%v'", id, obj[objects.FieldKeyStatus])
		}
	}
}

func TestObjectPromote_FamilyFlagAlias(t *testing.T) {
	_ = SetupTestEnvironment(t)

	ctx := context.Background()
	promoteCmd := NewPromoteCmd()
	_ = promoteCmd.Flags().Set("family", "true")
	promoteCmd.SetContext(ctx)

	proc, err := cli.NewProcessor(promoteCmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	secCtx := proc.SecurityContext()

	// Create draft requirement and criterion
	criID := "CRI-FAM-E2E"
	reqID := "REQ-FAM-E2E"

	criObj := map[string]any{
		objects.FieldKeyID:          criID,
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Family E2E Criteria",
		objects.FieldKeyStatus:      "conceptual",
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyDescription: "Comprehensive acceptance criterion for family testing.",
	}
	reqObj := map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Family E2E Requirement",
		objects.FieldKeyStatus:       "conceptual",
		objects.FieldKeyDescription:  "Detailed requirement description meeting all invariants.",
		objects.FieldKeyPriority:     "p1",
		objects.FieldKeyCriteriaRefs: []string{criID},
	}

	for _, obj := range []map[string]any{criObj, reqObj} {
		if err := proc.Storage().Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Pass only REQ with --family
	err = promoteObjectIDs(promoteCmd, proc, []string{reqID})
	if err != nil {
		t.Fatalf("promoteObjectIDs with --family failed: %v", err)
	}

	// Both REQ and CRI should be promoted to 'originated'
	for _, id := range []string{criID, reqID} {
		obj, err := proc.Storage().Read(ctx, secCtx, id)
		if err != nil {
			t.Fatalf("Read %s: %v", id, err)
		}
		if obj[objects.FieldKeyStatus] != "originated" {
			t.Errorf("expected status 'originated' for %s, got '%v'", id, obj[objects.FieldKeyStatus])
		}
	}
}
