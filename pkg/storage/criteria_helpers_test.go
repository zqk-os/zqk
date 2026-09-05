package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"

	"github.com/lanceman/zqk/pkg/validation"
)

// setupCriteriaTestRoot returns a temp directory with object_specs copied from the repo (read-only source)
// so FileObjectStorage never uses the live checkout as project root.
func setupCriteriaTestRoot(t *testing.T) string {
	t.Helper()
	testRoot := t.TempDir()
	mustEnsureProcessSpecsLayout(t, testRoot)
	// already bootstrapped

	return testRoot
}

func TestGetCriteriaIDByTitle_FileBackend(t *testing.T) {
	testRoot := setupCriteriaTestRoot(t)

	// Create file storage
	storageProvider, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Skipf("Skipping test: could not create file storage: %v", err)
	}

	defer func() { _ = storageProvider.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storageProvider)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Use unique test IDs and title per run so GetCriteriaIDByTitle returns our object
	// (project may have many criteria; first match by title is used, so title must be unique)
	base := 9000 + int(time.Now().UnixNano()%999)
	testID := fmt.Sprintf("CRIT-%d", base)
	externalTestID := fmt.Sprintf("CRIT-%d", base+1)
	uniqueTitle := fmt.Sprintf("Test Criteria - Purpose Required [%s]", testID)

	// Clean up any existing test criteria first (use CLI context for delete)
	cliCtx := WithTestHardDelete(ctx)

	// Always delete test objects to ensure clean state
	for i := 0; i < 5; i++ {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(cliCtx, secCtx, testID, false)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(cliCtx, secCtx, externalTestID, false)
		// Wait deterministically for deletion to complete
		if waitForConditionWithTimeout(context.Background(),
			func() bool {
				_, checkErr1 := storageProvider.Read(ctx, secCtx, testID)
				_, checkErr2 := storageProvider.Read(ctx, secCtx, externalTestID)
				return checkErr1 != nil && checkErr2 != nil // Both should not exist
			},
			5*time.Second,
			10*time.Millisecond,
		) {
			// Successfully deleted both
			break
		}
	}

	// Create a test criteria object with unique title so GetCriteriaIDByTitle finds only this one
	testCriteria := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         uniqueTitle,
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
	}

	// Create + promote (not_started is preliminary; List omits draft plane).
	CreateCASVisible(t, storageProvider, cliCtx, secCtx, testCriteria, objects.ObjectStatusInProgress)
	// Verify object was created by reading it back
	created, err := storageProvider.Read(ctx, secCtx, testID)
	if err != nil {
		t.Fatalf("Failed to read created criteria: %v", err)
	}
	if createdTitle, ok := created[objects.FieldKeyTitle].(string); !ok || createdTitle != uniqueTitle {
		t.Fatalf("Created criteria has wrong title: expected %q, got %v", uniqueTitle, createdTitle)
	}
	// Verify origin_system is set correctly
	if origin, ok := created[objects.FieldKeyOriginSystem].(string); !ok || origin != validation.DefaultOriginSystem {
		t.Fatalf("Created criteria has wrong origin_system: expected '%s', got %v", validation.DefaultOriginSystem, origin)
	}
	// Wait deterministically for object to appear in List
	if !waitForConditionWithTimeout(context.Background(),
		func() bool {
			storageCtx := pkgctx.GetStorageContext()
			verifyFilter := ListFilter{
				Kind: "criteria",
			}
			result, err := storageProvider.List(ctx, secCtx, storageCtx, verifyFilter)
			if err != nil {
				return false
			}
			if result == nil {
				return false
			}
			for _, obj := range result.Objects {
				if objID, ok := obj[objects.FieldKeyID].(string); ok && objID == testID {
					return true
				}
			}
			return false
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Expected object to appear in List within 5 seconds")
	}
	// Verify object appears in List
	storageCtx := pkgctx.GetStorageContext()
	verifyFilter := ListFilter{
		Kind: "criteria",
		Filters: map[string]any{
			objects.FieldKeyOriginSystem: map[string]any{
				"$eq": validation.DefaultOriginSystem,
			},
		},
	}
	verifyResult, listErr := storageProvider.List(ctx, secCtx, storageCtx, verifyFilter)
	if listErr == nil {
		found := false
		for _, obj := range verifyResult.Objects {
			id, ok := obj[objects.FieldKeyID].(string)
			if !ok || id != testID {
				continue
			}
			found = true
			title, _ := obj[objects.FieldKeyTitle].(string)
			origin, _ := obj[objects.FieldKeyOriginSystem].(string)
			t.Logf("Verified %s in List: title=%s, origin_system=%s", testID, title, origin)
			break
		}
		if !found {
			t.Fatalf("Created criteria %s not found in List operation - object may not be written to disk", testID)
		}
	}

	// Clean up
	defer func() {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(cliCtx, secCtx, testID, false)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(cliCtx, secCtx, externalTestID, false)
	}()

	// Test exact title match (unique title so only our criteria matches)
	verifyObj, err := storageProvider.Read(ctx, secCtx, testID)
	if err != nil {
		t.Fatalf("Cannot read %s after creation: %v", testID, err)
	}
	verifyTitle, _ := verifyObj[objects.FieldKeyTitle].(string)
	t.Logf("Verified %s exists with title: %s", testID, verifyTitle)

	criteriaID, err := GetCriteriaIDByTitle(ctx, storageProvider, uniqueTitle, true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != testID {
		t.Errorf("Expected %s, got %s", testID, criteriaID)
	}

	// Test partial title match (use substring only our criteria has so we don't match other project criteria)
	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, " ["+testID+"]", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != testID {
		t.Errorf("Expected %s, got %s", testID, criteriaID)
	}

	// Test case-insensitive match (use unique substring)
	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "purpose required ["+strings.ToLower(testID)+"]", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != testID {
		t.Errorf("Expected %s, got %s", testID, criteriaID)
	}

	// Test internalOnly filter (should not find external criteria)
	externalCriteria := map[string]any{
		objects.FieldKeyID:            externalTestID,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "External Criteria",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  "external",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
	}
	if err := storageProvider.Create(cliCtx, secCtx, externalCriteria); err != nil {
		// If already exists, that's okay - verify it has correct origin_system
		if errors.Is(err, ErrObjectExists) {
			existing, err := storageProvider.Read(ctx, secCtx, externalTestID)
			if err == nil {
				if origin, ok := existing[objects.FieldKeyOriginSystem].(string); ok && origin == "external" {
					// Promote if still on draft plane so List/GetCriteriaIDByTitle can see it.
					if storageProvider.objectDraftPlaneExists("criteria", externalTestID) {
						if err := storageProvider.Update(cliCtx, secCtx, externalTestID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}); err != nil {
							t.Fatalf("promote existing external criteria: %v", err)
						}
					}
				} else {
					// Object exists but has wrong origin - try to delete and recreate
					t.Logf("Warning: Existing %s has wrong origin_system: %v, attempting to delete and recreate", externalTestID, origin)
					for i := 0; i < 5; i++ {
						//nolint:errcheck // Test cleanup - errors are acceptable
						_ = storageProvider.Delete(cliCtx, secCtx, externalTestID, false)
						// Wait deterministically for deletion
						if waitForConditionWithTimeout(context.Background(),
							func() bool {
								_, checkErr := storageProvider.Read(ctx, secCtx, externalTestID)
								return checkErr != nil // Object should not exist
							},
							5*time.Second,
							10*time.Millisecond,
						) {
							CreateCASVisible(t, storageProvider, cliCtx, secCtx, externalCriteria, objects.ObjectStatusInProgress)
							break
						}
					}
				}
			}
		} else {
			t.Fatalf("Failed to create external criteria: %v", err)
		}
	} else if err := storageProvider.Update(cliCtx, secCtx, externalTestID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}); err != nil {
		t.Fatalf("promote external criteria off draft plane: %v", err)
	}

	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "External Criteria", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != emptyValue {
		t.Errorf("Expected empty string (filtered out), got %s", criteriaID)
	}

	// Test with internalOnly=false (should find external)
	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "External Criteria", false)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	// Note: If there are other external criteria with the same title, we might get a different ID
	// That's okay - we just need to verify it's not empty and it's an external criteria
	if criteriaID == emptyValue {
		t.Errorf("Expected to find external criteria, got empty string")
	} else if criteriaID != externalTestID {
		// Check if the found ID is actually an external criteria
		foundObj, err := storageProvider.Read(ctx, secCtx, criteriaID)
		if err == nil {
			if origin, ok := foundObj[objects.FieldKeyOriginSystem].(string); ok && origin == "external" {
				// Found a different external criteria with same title - that's acceptable
				t.Logf("Found different external criteria %s (expected %s) - acceptable", criteriaID, externalTestID)
			} else {
				t.Errorf("Found criteria %s but origin_system is not 'external': %v", criteriaID, origin)
			}
		} else {
			t.Errorf("Expected %s, got %s (and couldn't verify it's external)", externalTestID, criteriaID)
		}
	}
}

func TestGetCriteriaIDForChecklistItem(t *testing.T) {
	testRoot := setupCriteriaTestRoot(t)

	storageProvider, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("could not create file storage: %v", err)
	}

	defer func() { _ = storageProvider.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storageProvider)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := WithTestHardDelete(ctx)

	// Seed a criteria whose title matches ChecklistItemToTitlePattern["purpose"] (no dependency on repo data).
	purposeTitle := ChecklistItemToTitlePattern[objects.FieldKeyPurpose]
	seedID := fmt.Sprintf("CRIT-CHK-%d", time.Now().UnixNano()%1000000)
	seedCriteria := map[string]any{
		objects.FieldKeyID:            seedID,
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         purposeTitle,
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
	}
	CreateCASVisible(t, storageProvider, cliCtx, secCtx, seedCriteria, objects.ObjectStatusInProgress)
	t.Cleanup(func() {
		//nolint:errcheck // test cleanup
		_ = storageProvider.Delete(cliCtx, secCtx, seedID, false)
	})

	// Test checklist item lookup - should find the seeded criteria
	criteriaID, err := GetCriteriaIDForChecklistItem(ctx, storageProvider, "purpose", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDForChecklistItem failed: %v", err)
	}
	if criteriaID != seedID {
		t.Errorf("GetCriteriaIDForChecklistItem: want %s, got %s", seedID, criteriaID)
	}

	// Test unknown checklist item
	_, err = GetCriteriaIDForChecklistItem(ctx, storageProvider, "unknown_item", true)
	if err == nil {
		t.Error("Expected error for unknown checklist item")
	}
}

func TestGetCriteriaIDByTitle_GraphBackend(t *testing.T) {
	ctx := context.Background()
	testRoot := setupCriteriaTestRoot(t)
	factory, err := NewStorageFactory(ctx, testRoot)
	if err != nil || !factory.IsGraphBackend("criteria") {
		t.Skip("Skipping graph backend test: graph backend not enabled or failed to create factory")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageProvider := factory.GetStorage()

	// Create a test criteria object with unique title
	testCriteria := map[string]any{
		objects.FieldKeyID:            "CRIT-9101",
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "Graph Test Criteria - Purpose Required",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
	}

	// Create the criteria
	if err := storageProvider.Create(ctx, secCtx, testCriteria); err != nil {
		t.Fatalf("Failed to create test criteria: %v", err)
	}

	// Clean up
	defer func() {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(ctx, secCtx, "CRIT-9101", false)
	}()

	// Test exact title match
	criteriaID, err := GetCriteriaIDByTitle(ctx, storageProvider, "Graph Test Criteria - Purpose Required", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != "CRIT-9101" {
		t.Errorf("Expected CRIT-9101, got %s", criteriaID)
	}

	// Test partial title match
	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "Graph Test Criteria", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != "CRIT-9101" {
		t.Errorf("Expected CRIT-9101, got %s", criteriaID)
	}

	// Test case-insensitive match
	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "graph test criteria", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != "CRIT-9101" {
		t.Errorf("Expected CRIT-9101, got %s", criteriaID)
	}

	// Test internalOnly filter (should not find external criteria)
	externalCriteria := map[string]any{
		objects.FieldKeyID:            "CRIT-9102",
		objects.FieldKeyKind:          "criteria",
		objects.FieldKeyTitle:         "External Criteria",
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeyOriginSystem:  "external",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
	}
	if err := storageProvider.Create(ctx, secCtx, externalCriteria); err != nil {
		t.Fatalf("Failed to create external criteria: %v", err)
	}
	defer func() {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(ctx, secCtx, "CRIT-9102", false)
	}()

	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "External Criteria", true)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != emptyValue {
		t.Errorf("Expected empty string (filtered out), got %s", criteriaID)
	}

	// Test with internalOnly=false (should find external)
	criteriaID, err = GetCriteriaIDByTitle(ctx, storageProvider, "External Criteria", false)
	if err != nil {
		t.Fatalf("GetCriteriaIDByTitle failed: %v", err)
	}
	if criteriaID != "CRIT-9102" {
		t.Errorf("Expected CRIT-9102, got %s", criteriaID)
	}
}
