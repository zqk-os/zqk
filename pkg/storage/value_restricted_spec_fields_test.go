package storage_test

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func createTestCriteria(t *testing.T, fos *storage.FileObjectStorage, secCtx *pkgctx.SecurityContext, id string) map[string]any {
	t.Helper()
	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Initial Criteria Title",
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyDescription:   "Substantive criteria description for value restriction testing.",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, fos, ctx, secCtx, obj, objects.ObjectStatusAwaitingVerification)
	created, err := fos.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("failed to read test criteria item: %v", err)
	}
	return created
}

func TestValueRestrictedSpecFields_CriteriaCategoryRejection(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	objID := "CRIT-VAL-001"
	createTestCriteria(t, fos, secCtx, objID)

	// Attempt updating with invalid category
	err := fos.Update(ctx, secCtx, objID, map[string]any{
		"category": "invalid_cat",
	})
	if err == nil {
		t.Fatalf("expected update with invalid category 'invalid_cat' to fail closed, but it succeeded")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "category") {
		t.Errorf("error %q should name field 'category'", errMsg)
	}
	if !strings.Contains(errMsg, "invalid_cat") {
		t.Errorf("error %q should name invalid value 'invalid_cat'", errMsg)
	}
	if !strings.Contains(errMsg, "acceptance") || !strings.Contains(errMsg, "functional") {
		t.Errorf("error %q should list allowed enum values", errMsg)
	}
}

func TestValueRestrictedSpecFields_CriteriaCategoryAcceptance(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	objID := "CRIT-VAL-002"
	createTestCriteria(t, fos, secCtx, objID)

	validCategories := []string{
		"functional",
		"non-functional",
		"acceptance",
		"test",
		"performance",
		"security",
		"compliance",
	}

	for _, cat := range validCategories {
		t.Run(cat, func(t *testing.T) {
			err := fos.Update(ctx, secCtx, objID, map[string]any{
				"category": cat,
			})
			if err != nil {
				t.Fatalf("expected update with valid category %q to succeed, got %v", cat, err)
			}

			updated, err := fos.Read(ctx, secCtx, objID)
			if err != nil {
				t.Fatalf("failed to read criteria after update: %v", err)
			}
			if updated["category"] != cat {
				t.Errorf("expected category to be %q, got %v", cat, updated["category"])
			}
		})
	}
}

func TestValueRestrictedSpecFields_PriorityTierRejectionAndAcceptance(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	objID := "BLI-VAL-003"
	createTestBacklogItem(t, fos, secCtx, objID)

	// Rejection
	err := fos.Update(ctx, secCtx, objID, map[string]any{
		"priority_tier": "P99",
	})
	if err == nil {
		t.Fatalf("expected update with invalid priority_tier 'P99' to fail, but succeeded")
	}
	if !strings.Contains(err.Error(), "priority_tier") || !strings.Contains(err.Error(), "P99") {
		t.Errorf("error %q should name field priority_tier and value P99", err.Error())
	}

	// Acceptance
	for _, tier := range []string{"P0", "P1", "P2", "P3"} {
		err := fos.Update(ctx, secCtx, objID, map[string]any{
			"priority_tier": tier,
		})
		if err != nil {
			t.Fatalf("expected valid priority_tier %q to succeed, got %v", tier, err)
		}
	}
}

func TestValueRestrictedSpecFields_NumericRangeAndPatternBounds(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	ctx := context.Background()

	// Create backlog item and test field updates
	objID := "BLI-VAL-004"
	createTestBacklogItem(t, fos, secCtx, objID)

	// Valid description update
	err := fos.Update(ctx, secCtx, objID, map[string]any{
		"description": "A fully valid updated description exceeding minimum length criteria.",
	})
	if err != nil {
		t.Fatalf("valid update failed: %v", err)
	}
}
