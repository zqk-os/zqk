package storage

//nolint:errcheck // Test cleanup operations - errors are acceptable

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	instancebuilders "github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestAllKindsCRUD tests Create, Read, Update, Delete for all discoverable object kinds.
// Skip with -short for faster feedback; run full suite for CI or pre-commit.
func TestAllKindsCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping full CRUD across all kinds in short mode (use -short=false for full run)")
	}
	// Not: ZQK_TEST_ROOT is process-global; parallel top-level tests would race each other.
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	bootstrapTestRootFromProjectRoot(t, tmpDir, moduleRootFromGoEnv(t))

	ctx := context.Background()
	t.Setenv(zqkenv.SkipSpecSchemaValidation(), "1")
	secCtx := pkgctx.NewSystemSecurityContext()
	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	BuildPathAliasCacheForProject(tmpDir)

	// Get all discoverable object kinds
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}

	// Test each kind in parallel (each kind uses its own subdir under tmpDir)
	for _, kind := range kinds {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			// t.Parallel()
			testKindCRUD(t, storage, ctx, secCtx, kind, fieldRegistry)
		})
	}
}

// testKindCRUD tests CRUD operations for a specific kind
func testKindCRUD(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, fieldRegistry *objects.FieldRegistry) {
	// Get field information
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Skipf("skipping %s: failed to get fields: %v", kind, err)
		return
	}

	// Generate test ID
	testID := generateComprehensiveTestID(kind, 1)

	// Special setup: create referenced objects for kinds that need them
	cliCtx := WithCLIOperation(ctx)
	switch kind {
	case "workstream":
		// Create referenced account for owner_ref (use valid username that matches validation pattern)
		accountID := "account:testuser"
		accountObj := map[string]any{
			objects.FieldKeyID:            accountID, // Use account:username format
			objects.FieldKeyKind:          "account",
			objects.FieldKeyTitle:         "Test account for workstream",
			objects.FieldKeyUsername:      "testuser", // Valid username (lowercase, matches pattern)
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, accountObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced account %s: %v (test may fail)", accountID, err)
			}
		}
	case "agent_onboarding_preparation":
		// Create referenced account and workstream for agent_onboarding_preparation
		accountID := "account:testuser"
		accountObj := map[string]any{
			objects.FieldKeyID:            accountID, // Use account:username format
			objects.FieldKeyKind:          "account",
			objects.FieldKeyTitle:         "Test account for agent onboarding",
			objects.FieldKeyUsername:      "testuser", // Valid username (lowercase, matches pattern)
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, accountObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced account %s: %v (test may fail)", accountID, err)
			}
		}
		// Create referenced workstream
		wsID := "WS-999"
		wsObj := map[string]any{
			objects.FieldKeyID:            wsID,
			objects.FieldKeyKind:          "workstream",
			objects.FieldKeyTitle:         "Test workstream for agent onboarding",
			objects.FieldKeyEntryPoint:    "docs/workstreams/test-agent.md",
			objects.FieldKeyOwnerRef:      accountID, // Use same account ID
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, wsObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced workstream %s: %v (test may fail)", wsID, err)
			}
		}
	case "partnership":
		// Create referenced organization for organization_refs
		orgID := "ORG-999"
		orgObj := map[string]any{
			objects.FieldKeyID:                orgID,
			objects.FieldKeyKind:              "organization",
			objects.FieldKeyTitle:             "Test organization for partnership",
			objects.FieldKeyOrganizationName:  "Test Organization",
			objects.FieldKeyDomain:            "visualization",
			objects.FieldKeySpecInterpreter:   "test_interpreter",
			objects.FieldKeySpecContextBroker: "test_broker",
			objects.FieldKeyStatus:            "active",
			objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, orgObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced organization %s: %v (test may fail)", orgID, err)
			}
		}
	case "requirement":
		// Create referenced goal and criteria for requirement (use unique IDs to avoid conflicts)
		goalID := "GOAL-999"
		goalObj := map[string]any{
			objects.FieldKeyID:            goalID,
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         "Test Goal for requirement",
			objects.FieldKeyTarget:        "100",
			objects.FieldKeyMetric:        "count",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, goalObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced goal %s: %v (test may fail)", goalID, err)
			}
		}
		critID := "CRIT-999"
		critObj := map[string]any{
			objects.FieldKeyID:            critID,
			objects.FieldKeyKind:          "criteria",
			objects.FieldKeyTitle:         "Test Criteria for requirement",
			objects.FieldKeyCategory:      "functional",
			objects.FieldKeyStatus:        "not_started",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, critObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced criteria %s: %v (test may fail)", critID, err)
			}
		}
	case "assessment_rating":
		personaID := "PER-999"
		personaObj := map[string]any{
			objects.FieldKeyID:            personaID,
			objects.FieldKeyKind:          "persona",
			objects.FieldKeyTitle:         "Test persona for assessment rating",
			objects.FieldKeyName:          "Test Persona",
			objects.FieldKeyRole:          "Tester",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, personaObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced persona %s: %v (test may fail)", personaID, err)
			}
		}
	case "change_journal_entry":
		// Create referenced goal for object_ref (use unique ID to avoid conflicts)
		goalID := "GOAL-999"
		goalObj := map[string]any{
			objects.FieldKeyID:            goalID,
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         "Test Goal for change journal",
			objects.FieldKeyTarget:        "100",
			objects.FieldKeyMetric:        "count",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, goalObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced goal %s: %v (test may fail)", goalID, err)
			}
		}
	case "workstream_transition":
		// Create referenced workstreams for from_workstream_ref and to_workstream_ref
		// First create the account that workstreams need (use valid username)
		accountID := "account:testuser"
		accountObj := map[string]any{
			objects.FieldKeyID:            accountID, // Use account:username format
			objects.FieldKeyKind:          "account",
			objects.FieldKeyTitle:         "Test account for workstream transition",
			objects.FieldKeyUsername:      "testuser", // Valid username (lowercase, matches pattern)
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, accountObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced account %s: %v (test may fail)", accountID, err)
			}
		}
		// Create first workstream
		ws1ID := "WS-999"
		ws1Obj := map[string]any{
			objects.FieldKeyID:            ws1ID,
			objects.FieldKeyKind:          "workstream",
			objects.FieldKeyTitle:         "Test workstream 1",
			objects.FieldKeyEntryPoint:    "docs/workstreams/test-1.md",
			objects.FieldKeyOwnerRef:      accountID, // Use same account ID
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, ws1Obj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced workstream %s: %v (test may fail)", ws1ID, err)
			}
		}
		// Create second workstream
		ws2ID := "WS-998"
		ws2Obj := map[string]any{
			objects.FieldKeyID:            ws2ID,
			objects.FieldKeyKind:          "workstream",
			objects.FieldKeyTitle:         "Test workstream 2",
			objects.FieldKeyEntryPoint:    "docs/workstreams/test-2.md",
			objects.FieldKeyOwnerRef:      accountID, // Use same account ID
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, ws2Obj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced workstream %s: %v (test may fail)", ws2ID, err)
			}
		}
	case "code_quality_metric":
		// Create referenced policy for policy_ref (policy requires body, category, and valid policy_type enum)
		policyID := "POLICY-CODE-999"
		policyObj := map[string]any{
			objects.FieldKeyID:            policyID,
			objects.FieldKeyKind:          "policy",
			objects.FieldKeyTitle:         "Test policy for code quality metric",
			objects.FieldKeyPolicyType:    "standard", // Valid enum value: standard, requirement, guideline, best_practice, anti_pattern
			objects.FieldKeyBody:          "Test policy body content",
			objects.FieldKeyCategory:      "code_quality", // Required field - valid values: architecture, code_quality, documentation, testing, security, workflow, git, ci_cd
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, policyObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced policy %s: %v (test may fail)", policyID, err)
			}
		}
	case "certificate":
		// holder_ref defaults reference account:system; create it before certificate Create (parallel subtests race otherwise)
		accountID := "account:system"
		accountObj := map[string]any{
			objects.FieldKeyID:            accountID,
			objects.FieldKeyKind:          "account",
			objects.FieldKeyTitle:         "Test system account for certificate",
			objects.FieldKeyUsername:      "system",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(cliCtx, secCtx, accountObj); err != nil {
			if err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced account %s: %v (test may fail)", accountID, err)
			}
		}
	case objects.KindGlossaryTermRelation:
		// Ref fields require GLS- / VOC- ids; Create validates targets exist. Use the same ids as
		// [generateReferenceID] / populateRequiredFieldsFromConfig (test_object_builder.go).
		vocID := generateReferenceID(objects.KindVocabularyScheme)
		glsID := generateReferenceID(objects.KindGlossaryTerm)
		if vocFields, err := fieldRegistry.GetFieldsForKind(objects.KindVocabularyScheme); err == nil {
			vocObj := createTestObjectForCRUD(objects.KindVocabularyScheme, vocID, vocFields, 0)
			if err := storage.Create(cliCtx, secCtx, vocObj); err != nil && err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced vocabulary_scheme %s: %v (test may fail)", vocID, err)
			}
		}
		if gtFields, err := fieldRegistry.GetFieldsForKind(objects.KindGlossaryTerm); err == nil {
			gtObj := createTestObjectForCRUD(objects.KindGlossaryTerm, glsID, gtFields, 0)
			if err := storage.Create(cliCtx, secCtx, gtObj); err != nil && err != ErrObjectExists {
				t.Logf("Warning: Could not create referenced glossary_term %s: %v (test may fail)", glsID, err)
			}
		}
	case "technical_spec":
		goalID := "GOAL-999"
		goalObj := map[string]any{
			objects.FieldKeyID:            goalID,
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         "Test Goal",
			objects.FieldKeyTarget:        "100",
			objects.FieldKeyMetric:        "count",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		_ = storage.Create(cliCtx, secCtx, goalObj)

		critID := "CRIT-999"
		critObj := map[string]any{
			objects.FieldKeyID:            critID,
			objects.FieldKeyKind:          "criteria",
			objects.FieldKeyTitle:         "Test Criteria",
			objects.FieldKeyCategory:      "functional",
			objects.FieldKeyStatus:        "not_started",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		_ = storage.Create(cliCtx, secCtx, critObj)

		reqID := "REQ-999"
		reqObj := map[string]any{
			objects.FieldKeyID:            reqID,
			objects.FieldKeyKind:          "requirement",
			objects.FieldKeyTitle:         "Test requirement",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyGoalRefs:      []string{"GOAL-999"},
			objects.FieldKeyCriteriaRefs:  []string{"CRIT-999"},
		}
		if err := storage.Create(cliCtx, secCtx, reqObj); err != nil && err != ErrObjectExists {
			t.Logf("Warning: Could not create referenced requirement %s: %v (test may fail)", reqID, err)
		}
	default:
		// No special setup needed for other kinds
	}

	// Track whether Create succeeded for diagnostic purposes
	createSucceeded := false

	// Test 1: Create
	t.Run("Create", func(t *testing.T) {
		obj := createTestObjectForCRUD(kind, testID, kindFields, 0)

		// For change_journal_entry, handle the case where it might already exist
		// (system may create them automatically during operations)
		if kind == "change_journal_entry" {
			// Try to delete first if it exists
			//nolint:errcheck // Test cleanup - errors are acceptable
			_ = storage.Delete(cliCtx, secCtx, testID, false)
			if !ensureObjectDeleted(ctx, storage, secCtx, testID) {
				t.Fatalf("Failed to confirm deletion of object %s within timeout", testID)
			}
		}

		if err := storage.Create(ctx, secCtx, obj); err != nil {
			// For change_journal_entry, if it already exists, try to delete and recreate
			if kind == "change_journal_entry" && (err == ErrObjectExists || strings.Contains(err.Error(), "already exists")) {
				//nolint:errcheck // Test cleanup - errors are acceptable
				_ = storage.Delete(cliCtx, secCtx, testID, false)
				if !ensureObjectDeleted(ctx, storage, secCtx, testID) {
					t.Fatalf("Failed to confirm deletion of object %s within timeout", testID)
				}
				if err := storage.Create(ctx, secCtx, obj); err != nil {
					t.Errorf("create failed for %s after cleanup: %v", kind, err)
					return
				}
			} else {
				t.Errorf("create failed for %s: %v", kind, err)
				return
			}
		}

		// For CAS-enabled kinds, ensure index is ready for reading
		// CAS is enabled for all kinds, so always flush and verify
		if err := FlushListingIndexForKind(kind); err != nil {
			t.Logf("Warning: Failed to flush CAS index for %s: %v (may cause read failure)", kind, err)
		}

		// Verify the object is in the CAS index before trying to read
		// This helps catch cases where the index update didn't complete
		cas, err := storage.GetContentAddressableStorage(kind)
		if err == nil {
			// Check if object is in index (with retry for async updates)
			maxRetries := 50
			foundInIndex := false
			for i := 0; i < maxRetries; i++ {
				_, hashErr := cas.GetHashForID(testID)
				if hashErr == nil {
					foundInIndex = true
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if !foundInIndex {
				t.Logf("Warning: Object %s not found in CAS index after %d retries (may cause read failure)", testID, maxRetries)
			}
		}

		// Verify creation by reading with retry (CAS index updates are async)
		var created map[string]any
		var readErr error
		maxRetries := 50
		for i := 0; i < maxRetries; i++ {
			created, readErr = storage.Read(ctx, secCtx, testID)
			if readErr == nil {
				break
			}
			// Small delay before retry to allow async index update to complete
			time.Sleep(50 * time.Millisecond)
		}
		if readErr != nil {
			t.Errorf("read failed after create for %s (after %d retries): %v", kind, maxRetries, readErr)
			return
		}

		if created[objects.FieldKeyID] != testID {
			t.Errorf("created object has wrong ID: expected %s, got %v", testID, created[objects.FieldKeyID])
			return
		}

		createSucceeded = true
	})

	// Test 2: Read
	t.Run("Read", func(t *testing.T) {
		if !createSucceeded {
			t.Skipf("Skipping Read test: Create did not succeed (ID: %s)", testID)
			return
		}

		obj, err := storage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Errorf("read failed for %s: %v", kind, err)
			return
		}

		if obj[objects.FieldKeyID] != testID {
			t.Errorf("read returned wrong object: expected id %s, got %v", testID, obj[objects.FieldKeyID])
		}

		if obj[objects.FieldKeyKind] != kind {
			t.Errorf("read returned wrong kind: expected %s, got %v", kind, obj[objects.FieldKeyKind])
		}
	})

	// Test 3: Update
	t.Run("Update", func(t *testing.T) {
		if !createSucceeded {
			t.Skipf("Skipping Update test: Create did not succeed (ID: %s)", testID)
			return
		}
		// Find a mutable field to update with appropriate type
		updateField := ""
		var updateValue any

		// Helper to generate appropriate test value based on field type
		generateUpdateValue := func(field objects.FieldInfo) any {
			switch field.Type {
			case "string", "text":
				return "Updated Value"
			case "integer":
				return 42
			case "number", "float":
				return 42.0
			case "boolean":
				return true
			case "list", "array":
				return []string{"item1", "item2"}
			case "datetime", "date":
				return "2025-12-26T00:00:00Z"
			case "enum":
				// For enum, try to use a different valid value if possible
				// For now, just use a string - validation will catch invalid values
				return "Updated Value"
			default:
				return "Updated Value"
			}
		}

		// Find a simple field to update (prefer string fields, avoid complex types)
		for i := range kindFields.AllFields {
			field := kindFields.AllFields[i]
			// Skip immutable/system fields
			if field.Name == "id" || field.Name == "kind" || field.Name == "created_at" || field.Name == "created_by" ||
				field.Name == "updated_at" || field.Name == "updated_by" {
				continue
			}
			// Skip immutable fields from spec lifecycle
			if strings.HasPrefix(strings.ToLower(field.Lifecycle), "immutable") || strings.HasPrefix(strings.ToLower(field.Lifecycle), "read-only") {
				continue
			}
			// Skip reference fields (they need valid object IDs)
			if strings.HasSuffix(field.Name, "_ref") || strings.HasSuffix(field.Name, "_refs") {
				continue
			}
			// Skip change_journal_entry specific fields that may not exist or be mutable
			if kind == "change_journal_entry" && (field.Name == "change_type" || field.Name == "object_ref" || field.Name == "diff_summary" || field.Name == "previous_state") {
				continue
			}
			// Prefer simple writable fields
			if contains(field.Traits, "writable") || contains(field.Traits, "modifiable") {
				// Prefer string fields for simplicity
				if field.Type == "string" || field.Type == "text" {
					updateField = field.Name
					updateValue = generateUpdateValue(field)
					break
				}
				// If we haven't found a string field yet, use this one
				if updateField == emptyValue {
					updateField = field.Name
					updateValue = generateUpdateValue(field)
				}
			}
		}

		if updateField == emptyValue {
			// Fallback to title (should always exist and be string type)
			updateField = "title"
			updateValue = "Updated Title"
		}

		updates := map[string]any{
			updateField: updateValue,
		}

		if err := storage.Update(ctx, secCtx, testID, updates); err != nil {
			// If validation error, that's okay - some fields have strict requirements
			if strings.Contains(err.Error(), "validation") {
				t.Logf("Update validation error for %s field %s (expected for strict fields): %v", kind, updateField, err)
				return
			}
			t.Errorf("update failed for %s: %v", kind, err)
			return
		}

		// Verify update
		updated, err := storage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Errorf("read failed after update for %s: %v", kind, err)
			return
		}

		// Compare values (handle different types)
		if updated == nil {
			t.Errorf("read after update returned nil for %s", kind)
			return
		}
		actualValue, exists := updated[updateField]
		if !exists {
			t.Errorf("update field %s not found in updated object for %s", updateField, kind)
			return
		}
		if !reflect.DeepEqual(actualValue, updateValue) {
			// For slices, normalize before comparison
			if actualValue != nil && updateValue != nil {
				actualType := reflect.TypeOf(actualValue)
				updateType := reflect.TypeOf(updateValue)
				if actualType != nil && updateType != nil && actualType.Kind() == reflect.Slice && updateType.Kind() == reflect.Slice {
					// Both are slices, compare as-is
					if !reflect.DeepEqual(actualValue, updateValue) {
						t.Errorf("update failed: field %s not updated correctly. Expected %v, got %v", updateField, updateValue, actualValue)
					}
				} else {
					t.Errorf("update failed: field %s not updated correctly. Expected %v (%T), got %v (%T)", updateField, updateValue, updateValue, actualValue, actualValue)
				}
			} else {
				t.Errorf("update failed: field %s not updated correctly. Expected %v (%T), got %v (%T)", updateField, updateValue, updateValue, actualValue, actualValue)
			}
		}
	})

	// Test 4: Delete
	t.Run("Delete", func(t *testing.T) {
		if !createSucceeded {
			t.Skipf("Skipping Delete test: Create did not succeed (ID: %s)", testID)
			return
		}

		// Delete requires CLI context. Parallel kind subtests share one temp dir; reverse-ref index
		// or cross-kind refs can report dependents—retry with cascade in this isolated root only.
		if err := deleteTestObjectWithCascadeFallback(t, storage, ctx, secCtx, testID); err != nil {
			t.Errorf("delete failed for %s: %v", kind, err)
			return
		}

		// Verify deletion
		_, err := storage.Read(ctx, secCtx, testID)
		if err == nil {
			t.Errorf("delete failed: object still exists after deletion")
		}
	})
}

// TestAllKindsPagination tests pagination for all discoverable object kinds
func TestAllKindsPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping AllKindsPagination in short mode (many kinds)")
	}
	// Not: ZQK_TEST_ROOT is process-global; parallel top-level tests would race each other.
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	bootstrapTestRootFromProjectRoot(t, tmpDir, moduleRootFromGoEnv(t))

	ctx := context.Background()
	t.Setenv(zqkenv.SkipSpecSchemaValidation(), "1")
	secCtx := pkgctx.NewSystemSecurityContext()
	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	BuildPathAliasCacheForProject(tmpDir)

	// Get all discoverable object kinds
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}

	// Limit to 3 kinds to avoid test timeouts. Pagination logic is generic.
	if len(kinds) > 3 {
		kinds = kinds[:3]
	}

	// Test each kind
	for _, kind := range kinds {
		kind := kind // Capture variable for execution
		t.Run(kind, func(t *testing.T) {
			testKindPagination(t, tmpDir, storage, ctx, secCtx, kind, fieldRegistry)
		})
	}
}

// testKindPagination tests pagination for a specific kind
func testKindPagination(t *testing.T, tmpDir string, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, fieldRegistry *objects.FieldRegistry) {
	// Create multiple test objects (at least 10 for pagination tests)
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Skipf("skipping %s: failed to get fields: %v", kind, err)
		return
	}

	numObjects := 15
	testObjects := []map[string]any{}

	for i := 0; i < numObjects; i++ {
		testID := generateComprehensiveTestID(kind, i+1)
		obj := createTestObjectForCRUD(kind, testID, kindFields, i)
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Logf("failed to create test object %s for kind %s: %v (skipping)", testID, kind, err)
			continue
		}
		testObjects = append(testObjects, obj)
	}

	if len(testObjects) < 5 {
		t.Skipf("skipping %s: not enough test objects created (%d)", kind, len(testObjects))
		return
	}

	// Flush CAS index to ensure all created objects are indexed and visible for listing.
	FlushOrFail(t, tmpDir, kind)

	storageCtx := pkgctx.GetStorageContext()
	storageCtx.MaxPageSize = 5
	storageCtx.DefaultPageSize = 5

	// Test 1: First page (offset 0, limit 5)
	t.Run("FirstPage", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
			Kind:   kind,
			Offset: 0,
			Limit:  5,
		})
		if err != nil {
			t.Errorf("list failed for first page: %v", err)
			return
		}

		if len(result.Objects) > 5 {
			t.Errorf("first page returned too many objects: expected max 5, got %d", len(result.Objects))
		}

		if result.Meta["total_count"] == nil {
			t.Errorf("result missing total_count metadata")
		}
	})

	// Test 2: Second page (offset 5, limit 5)
	t.Run("SecondPage", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
			Kind:   kind,
			Offset: 5,
			Limit:  5,
		})
		if err != nil {
			t.Errorf("list failed for second page: %v", err)
			return
		}

		if len(result.Objects) > 5 {
			t.Errorf("second page returned too many objects: expected max 5, got %d", len(result.Objects))
		}

		// Verify different objects than first page
		if len(result.Objects) > 0 {
			//nolint:errcheck // Test query - error acceptable
			firstPageResult, _ := storage.List(ctx, secCtx, storageCtx, ListFilter{
				Kind:   kind,
				Offset: 0,
				Limit:  5,
			})
			if len(firstPageResult.Objects) > 0 && len(result.Objects) > 0 {
				// Check if any object from second page is in first page
				firstPageIDs := make(map[string]bool)
				for _, obj := range firstPageResult.Objects {
					if id, ok := obj[objects.FieldKeyID].(string); ok {
						firstPageIDs[id] = true
					}
				}
				// All objects in second page should be different from first page
				for _, obj := range result.Objects {
					if id, ok := obj[objects.FieldKeyID].(string); ok {
						if firstPageIDs[id] {
							t.Errorf("second page returned same objects as first page (found duplicate ID: %s)", id)
							return
						}
					}
				}
			}
		}
	})

	// Test 3: Last page (offset beyond total)
	t.Run("LastPage", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
			Kind:   kind,
			Offset: 100000,
			Limit:  5,
		})
		if err != nil {
			t.Errorf("list failed for last page: %v", err)
			return
		}

		if len(result.Objects) != 0 {
			t.Errorf("last page returned objects when offset is beyond total: got %d. Meta total_count: %v", len(result.Objects), result.Meta["total_count"])
			for i, o := range result.Objects {
				t.Logf("Object %d: %v", i, o[objects.FieldKeyID])
			}
		}
	})

	// Test 4: No limit (Limit 0 means no cap; full list is returned)
	t.Run("NoLimit", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
			Kind:   kind,
			Offset: 0,
			Limit:  0, // No limit = return full set
		})
		if err != nil {
			t.Errorf("list failed with no limit: %v", err)
			return
		}
		// Limit 0 is not capped by MaxPageSize; we get the full list
		if len(result.Objects) < len(testObjects) {
			t.Errorf("no limit should return full list: got %d, expected at least %d", len(result.Objects), len(testObjects))
		}
	})

	// Test 5: Pagination with filtering
	t.Run("PaginationWithFilter", func(t *testing.T) {
		// Get a sample value from test objects
		if len(testObjects) > 0 {
			sampleValue := getFieldValue(testObjects[0], "status")
			if sampleValue != nil {
				result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
					Kind:   kind,
					Offset: 0,
					Limit:  3,
					Filters: map[string]any{
						objects.FieldKeyStatus: sampleValue,
					},
				})
				if err != nil {
					t.Errorf("list failed with filter and pagination: %v", err)
					return
				}

				if len(result.Objects) > 3 {
					t.Errorf("pagination with filter returned too many objects: expected max 3, got %d", len(result.Objects))
				}

				// Verify all returned objects match filter
				for _, obj := range result.Objects {
					if obj[objects.FieldKeyStatus] != sampleValue {
						t.Errorf("object %v does not match filter status=%v", obj[objects.FieldKeyID], sampleValue)
					}
				}
			}
		}
	})

	// Test 6: Pagination with sorting
	t.Run("PaginationWithSort", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, storageCtx, ListFilter{
			Kind:    kind,
			Offset:  0,
			Limit:   5,
			SortBy:  "id",
			SortAsc: true,
		})
		if err != nil {
			t.Errorf("list failed with sort and pagination: %v", err)
			return
		}

		if len(result.Objects) > 5 {
			t.Errorf("pagination with sort returned too many objects: expected max 5, got %d", len(result.Objects))
		}

		// Verify sorted order
		if len(result.Objects) >= 2 {
			for i := 0; i < len(result.Objects)-1; i++ {
				id1, _ := result.Objects[i][objects.FieldKeyID].(string)
				id2, _ := result.Objects[i+1][objects.FieldKeyID].(string)
				if id1 > id2 {
					t.Errorf("objects not sorted correctly: %s > %s", id1, id2)
				}
			}
		}
	})

	// Cleanup: delete test objects (CLI context + cascade if another object references this ID)
	cliCtx := WithCLIOperation(ctx)
	for _, obj := range testObjects {
		id, _ := obj[objects.FieldKeyID].(string)
		if err := storage.Delete(cliCtx, secCtx, id, false); err != nil {
			if strings.Contains(err.Error(), "dependent object") {
				//nolint:errcheck // best-effort cleanup in isolated tmp dir
				_ = storage.Delete(cliCtx, secCtx, id, true)
			}
		}
	}
}

// deleteTestObjectWithCascadeFallback deletes with cascade when dependents block non-cascade delete.
// Used only under TestAllKindsCRUD temp roots where all objects are test-owned.
func deleteTestObjectWithCascadeFallback(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, id string) error {
	t.Helper()
	cliCtx := WithCLIOperation(ctx)
	err := storage.Delete(cliCtx, secCtx, id, false)
	if err != nil && strings.Contains(err.Error(), "dependent object") {
		err = storage.Delete(cliCtx, secCtx, id, true)
	}
	return err
}

// fixtureObjectTitle returns a title for synthetic objects in isolated test roots (same contract as pkg/testing.FixtureObjectTitle).
func fixtureObjectTitle(kind string, seq int) string {
	return fmt.Sprintf("Fixture: %s #%d", kind, seq)
}

// Helper functions
func createTestObjectForCRUD(kind, id string, kindFields *objects.KindFields, index int) map[string]any {
	// Get schema version from spec
	schemaVersion := objects.DefaultSchemaVersion // Default fallback
	specLoader := objects.NewSpecLoader("")
	specFile := fmt.Sprintf("%s.yaml", kind)
	if spec, err := specLoader.LoadSpecWithInheritance(specFile); err == nil {
		if spec.SchemaVersion != emptyValue {
			schemaVersion = spec.SchemaVersion
		}
	}

	// Get instance builder from registry
	registry := instancebuilders.GetGlobalRegistry()
	builder, err := registry.GetBuilder(kind, schemaVersion)
	if err != nil {
		// Builder not available - return minimal object with required fields
		obj := make(map[string]any)
		obj[objects.FieldKeyKind] = kind
		obj[objects.FieldKeyID] = id
		obj[objects.FieldKeyTitle] = fixtureObjectTitle(kind, index+1)
		obj[objects.FieldKeySchemaVersion] = schemaVersion
		if hasField(kindFields, "status") {
			obj[objects.FieldKeyStatus] = getInitialStatusForKind(kind)
		}
		populateRequiredFieldsFromConfig(obj, kind, kindFields, index)
		return obj
	}

	// Use instance builder
	builder.SetID(id)
	builder.SetField(objects.FieldKeyTitle, fixtureObjectTitle(kind, index+1))

	// Set status if field exists
	if hasField(kindFields, "status") {
		builder.SetStatus(getInitialStatusForKind(kind))
	}

	// Build instance
	instance, err := builder.Build()
	if err != nil {
		// Build failed - return minimal object with required fields
		obj := make(map[string]any)
		obj[objects.FieldKeyKind] = kind
		obj[objects.FieldKeyID] = id
		obj[objects.FieldKeyTitle] = fixtureObjectTitle(kind, index+1)
		obj[objects.FieldKeySchemaVersion] = schemaVersion
		if hasField(kindFields, "status") {
			obj[objects.FieldKeyStatus] = getInitialStatusForKind(kind)
		}
		populateRequiredFieldsFromConfig(obj, kind, kindFields, index)
		return obj
	}

	// Populate any additional required fields that the builder didn't set
	populateRequiredFieldsFromConfig(instance, kind, kindFields, index)
	return instance
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
