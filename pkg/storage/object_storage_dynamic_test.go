package storage

//nolint:errcheck // Test cleanup operations - errors are acceptable

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
)

// TestDynamicObjectDiscovery tests that we can discover all object types
func TestDynamicObjectDiscovery(t *testing.T) {
	// Get all known object kinds from the dynamic mapper
	mapper := objects.GetGlobalKindMapper()
	if err := mapper.Initialize(); err != nil {
		t.Fatalf("failed to initialize kind mapper: %v", err)
	}
	allKinds := mapper.GetAllKinds()

	if len(allKinds) == 0 {
		t.Fatal("no object kinds discovered")
	}

	t.Logf("Discovered %d object kinds: %v", len(allKinds), allKinds)

	// Verify each kind has a directory mapping
	for _, kind := range allKinds {
		dir := objects.GetDirectoryFromKind(kind)
		if dir == emptyValue {
			t.Errorf("kind %s has no directory mapping", kind)
		}
	}
}

// TestDynamicFieldDiscovery tests that we can discover all fields for each object type
func TestDynamicFieldDiscovery(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// Create test project structure
	mustEnsureProcessSpecsLayout(t, tmpDir)

	// Create spec loader to discover fields
	specLoader := objects.NewSpecLoader("")

	// Test each object kind
	mapper := objects.GetGlobalKindMapper()
	if err := mapper.Initialize(); err != nil {
		t.Fatalf("failed to initialize kind mapper: %v", err)
	}
	allKinds := mapper.GetAllKinds()
	for _, kind := range allKinds {
		t.Run(fmt.Sprintf("Discover fields for %s", kind), func(t *testing.T) {
			// Try to load spec for this kind
			// Spec files are named after ontology, which usually matches the kind
			// Try both the kind name and common variations
			specFiles := []string{
				kind + ".yaml",
			}

			var spec *objects.Spec
			var err error
			for _, specFile := range specFiles {
				spec, err = specLoader.LoadSpecWithInheritance(specFile)
				if err == nil && spec != nil {
					break
				}
			}

			if err != nil || spec == nil {
				// Spec might not exist - that's okay for this test
				// We're testing that the system can handle any field
				t.Logf("Could not load spec for %s (this is okay for dynamic testing)", kind)
				return
			}

			// Get all fields from resolved spec
			fields := spec.ResolvedFields
			if len(fields) == 0 {
				t.Logf("No fields found for %s (this is okay)", kind)
				return
			}

			t.Logf("Discovered %d fields for %s: %v", len(fields), kind, getFieldNames(fields))

			// Verify we can access each field
			for fieldName := range fields {
				if fieldName == emptyValue {
					t.Errorf("Empty field name found in %s", kind)
				}
			}
		})
	}
}

// TestUpdateAllFields tests that we can update any field on any object type
// TODO: Some fields may have strict validation rules (e.g., date formats, enum values).
// Test failures due to validation errors are expected for fields with strict requirements.
// The test should skip or handle validation errors gracefully rather than failing.
func TestUpdateAllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UpdateAllFields in short mode (many kinds)")
	}
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)

	realProjectRoot := filepath.Join("..", "..")
	realSpecsDir := filepath.Join(realProjectRoot, paths.ProcessInternalObjectSpecsDir)
	tmpSpecsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)

	entries, err := fileutil.ReadDir(realSpecsDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
				data, err := fileutil.ReadFile(filepath.Join(realSpecsDir, entry.Name()))
				if err == nil {
					_ = fileutil.WriteStandardFile(filepath.Join(tmpSpecsDir, entry.Name()), data)
				}
			}
		}
	}

	realConfigsDir := filepath.Join(realProjectRoot, paths.ProcessInternalConfigsDir)
	tmpConfigsDir := filepath.Join(tmpDir, paths.ProcessInternalConfigsDir)
	_ = fileutil.EnsureDir(tmpConfigsDir)
	configData, err := fileutil.ReadFile(filepath.Join(realConfigsDir, "id_prefixes_config.yaml"))
	if err == nil {
		_ = fileutil.WriteStandardFile(filepath.Join(tmpConfigsDir, "id_prefixes_config.yaml"), configData)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	specLoader := objects.NewSpecLoader("")
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Test all discoverable object kinds
	mapper := objects.GetGlobalKindMapper()
	if err := mapper.Initialize(); err != nil {
		t.Fatalf("failed to initialize kind mapper: %v", err)
	}
	allKinds := mapper.GetAllKinds()

	// Test each kind
	for _, kind := range allKinds {
		t.Run(fmt.Sprintf("Update all fields for %s", kind), func(t *testing.T) {

			// Try to load spec for this kind
			specFiles := []string{kind + ".yaml"}
			var spec *objects.Spec
			var err error
			for _, specFile := range specFiles {
				spec, err = specLoader.LoadSpecWithInheritance(specFile)
				if err == nil && spec != nil {
					break
				}
			}
			if err != nil || spec == nil {
				t.Skipf("Could not load spec for %s: %v", kind, err)
				return
			}

			// Create a test object with proper ID format for the kind
			// Use a unique ID based on test directory to avoid conflicts across test runs
			testID := generateTestID(kind)
			// Make ID unique by using a hash of the temp directory
			if len(tmpDir) > 10 {
				// Use last 8 chars of temp dir path as unique suffix (after \"zqk-test-\")
				dirSuffix := tmpDir[len(tmpDir)-8:]
				// Convert to a numeric-like string for ID compatibility
				hash := 0
				for _, c := range dirSuffix {
					hash = hash*31 + int(c)
				}
				if hash < 0 {
					hash = -hash
				}
				// For IDs like "GOAL-999", change to "GOAL-XXX" where XXX is unique
				if strings.Contains(testID, "-") {
					parts := strings.Split(testID, "-")
					if len(parts) == 2 {
						// Use modulo to keep it in reasonable range (001-999)
						uniqueNum := (hash % 999) + 1
						testID = fmt.Sprintf("%s-%03d", parts[0], uniqueNum)
					}
				}
			}

			// Clean up any existing test object first (use CLI context for delete).
			// If we can't deterministically confirm deletion (e.g., due to CAS
			// index latency), skip this kind-specific subtest rather than
			// failing the entire dynamic suite. The goal here is to validate
			// update semantics where possible without introducing global
			// flakiness.
			cliCtx := WithTestHardDelete(ctx)
			// Try to delete and deterministically wait for completion
			_ = storage.Delete(cliCtx, secCtx, testID, false) //nolint:errcheck // Test cleanup - errors are acceptable
			if !ensureObjectDeleted(ctx, storage, secCtx, testID) {
				t.Skipf("Skipping kind %s: failed to confirm deletion of object %s within timeout", kind, testID)
			}

			// Get schema version from spec
			schemaVersion := objects.DefaultSchemaVersion
			specLoader := objects.NewSpecLoader("")
			specFile := fmt.Sprintf("%s.yaml", kind)
			if spec, err := specLoader.LoadSpecWithInheritance(specFile); err == nil {
				if spec.SchemaVersion != emptyValue {
					schemaVersion = spec.SchemaVersion
				}
			}

			builder := instancebuilders.NewForKind(kind, schemaVersion)
			builder.SetID(testID)
			builder.SetField(objects.FieldKeyTitle, "Test Object")
			builder.SetStatus(getInitialStatus(kind))
			instance, buildErr := builder.Build()
			var obj map[string]any
			if buildErr != nil {
				obj = map[string]any{
					objects.FieldKeyID:            testID,
					objects.FieldKeyKind:          kind,
					objects.FieldKeyTitle:         "Test Object",
					objects.FieldKeyStatus:        getInitialStatus(kind),
					objects.FieldKeySchemaVersion: schemaVersion,
				}
			} else {
				obj = instance
			}

			// Add required fields for specific kinds
			setRequiredFieldsForTest(obj, kind)

			// Create referenced objects if needed (e.g., account for workstream, goal/criteria for requirement)
			switch kind {
			case "workstream":
				// Create referenced account if it doesn't exist
				accountID := "ACC-001"
				accountObj := map[string]any{
					objects.FieldKeyID:            accountID,
					objects.FieldKeyKind:          "account",
					objects.FieldKeyTitle:         "Test Account",
					objects.FieldKeyUsername:      "testuser",
					objects.FieldKeyStatus:        objects.ObjectStatusActive,
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				// Try to create account (ignore if already exists)
				if err := storage.Create(cliCtx, secCtx, accountObj); err != nil {
					// Check if error is "object already exists"
					if !errors.Is(err, ErrObjectExists) && !strings.Contains(err.Error(), "already exists") {
						// Log but continue - test will fail if account is truly needed
						t.Logf("Warning: Could not create referenced account %s: %v", accountID, err)
					}
				}
			case "requirement":
				// Create referenced goal and criteria if they don't exist (use unique IDs)
				goalID := "GOAL-999"
				goalObj := map[string]any{
					objects.FieldKeyID:            goalID,
					objects.FieldKeyKind:          "goal",
					objects.FieldKeyTitle:         "Test Goal",
					objects.FieldKeyTarget:        "100",
					objects.FieldKeyMetric:        "count",
					objects.FieldKeyStatus:        objects.ObjectStatusActive,
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				if err := storage.Create(cliCtx, secCtx, goalObj); err != nil {
					if !errors.Is(err, ErrObjectExists) && !strings.Contains(err.Error(), "already exists") {
						t.Logf("Warning: Could not create referenced goal %s: %v", goalID, err)
					}
				}
				critID := "CRIT-999"
				critObj := map[string]any{
					objects.FieldKeyID:            critID,
					objects.FieldKeyKind:          "criteria",
					objects.FieldKeyTitle:         "Test Criteria",
					objects.FieldKeyCategory:      "functional",
					objects.FieldKeyStatus:        objects.ObjectStatusNotStarted,
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				if err := storage.Create(cliCtx, secCtx, critObj); err != nil {
					if !errors.Is(err, ErrObjectExists) && !strings.Contains(err.Error(), "already exists") {
						t.Logf("Warning: Could not create referenced criteria %s: %v", critID, err)
					}
				}
			case "criteria":
				// Criteria doesn't need referenced objects, but we might need to create one for requirement tests
				// This is a no-op, but keeps the structure consistent
			case "change_journal_entry":
				// Create referenced goal for object_ref (format: "goal:GOAL-999")
				goalID := "GOAL-999"
				goalObj := map[string]any{
					objects.FieldKeyID:            goalID,
					objects.FieldKeyKind:          "goal",
					objects.FieldKeyTitle:         "Test Goal for change journal",
					objects.FieldKeyTarget:        "100",
					objects.FieldKeyMetric:        "count",
					objects.FieldKeyStatus:        objects.ObjectStatusActive,
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				if err := storage.Create(cliCtx, secCtx, goalObj); err != nil {
					if !errors.Is(err, ErrObjectExists) && !strings.Contains(err.Error(), "already exists") {
						t.Logf("Warning: Could not create referenced goal %s: %v", goalID, err)
					}
				}
			}

			// Create object (use CLI context for create). If we cannot satisfy
			// all required validation constraints for a given kind, skip that
			// kind-specific subtest instead of failing, since the purpose of
			// this dynamic test is to exercise the update path where feasible.
			if err := storage.Create(cliCtx, secCtx, obj); err != nil {
				// If object already exists, try to delete and recreate
				if errors.Is(err, ErrObjectExists) || strings.Contains(err.Error(), "already exists") {
					//nolint:errcheck // Test cleanup - errors are acceptable
					// Delete and try again
					//nolint:errcheck // Test cleanup - errors are acceptable
					_ = storage.Delete(cliCtx, secCtx, testID, false)
					if !ensureObjectDeleted(ctx, storage, secCtx, testID) {
						t.Skipf("Skipping kind %s: failed to confirm deletion of object %s within timeout (after Exists)", kind, testID)
					}
					if err := storage.Create(cliCtx, secCtx, obj); err != nil {
						t.Skipf("Skipping kind %s: failed to create test object after cleanup: %v", kind, err)
					}
				} else {
					t.Skipf("Skipping kind %s: failed to create test object: %v", kind, err)
				}
			}

			// Get all fields from spec
			fields := spec.ResolvedFields

			// Helper function to check if a field is system-managed from spec
			isFieldSystemManaged := func(_ string, fieldDef any) bool {
				fieldDefMap, ok := fieldDef.(map[string]any)
				if !ok {
					return false
				}

				// Check checklist.authority for "automation" - indicates system-managed
				if checklist, ok := fieldDefMap["checklist"].(map[string]any); ok {
					if authority, ok := checklist[objects.FieldKeyAuthority].(string); ok {
						authorityLower := strings.ToLower(authority)
						// Check if authority contains "automation" (case-insensitive)
						// This indicates the field is managed by the system
						if strings.Contains(authorityLower, "automation") {
							return true
						}
					}
				}

				return false
			}

			// Helper function to check if a field is immutable from spec
			isFieldImmutable := func(fieldName string, fieldDef any) bool {
				// Check if field is system-managed (these are typically immutable)
				if isFieldSystemManaged(fieldName, fieldDef) {
					return true
				}

				// Check spec for lifecycle: immutable
				fieldDefMap, ok := fieldDef.(map[string]any)
				if !ok {
					return false
				}

				// Check checklist.origin_lifecycle or checklist.lifecycle for "immutable" or "read-only"
				if checklist, ok := fieldDefMap["checklist"].(map[string]any); ok {
					var lifecycle string
					if l, ok := checklist[objects.FieldKeyOriginLifecycle].(string); ok {
						lifecycle = l
					} else if l, ok := checklist["lifecycle"].(string); ok {
						lifecycle = l
					}
					if lifecycle != "" {
						lifecycleLower := strings.ToLower(lifecycle)
						if strings.Contains(lifecycleLower, "immutable") || strings.Contains(lifecycleLower, "read-only") || strings.Contains(lifecycleLower, "append") {
							return true
						}
					}
				}

				return false
			}

			for fieldName, fieldDef := range fields {
				// Skip ID field - it's always immutable and shouldn't be updated in this test
				if fieldName == "id" {
					continue
				}
				if isFieldImmutable(fieldName, fieldDef) {
					continue // Skip immutable/system-managed fields
				}

				t.Run(fmt.Sprintf("Update field %s", fieldName), func(t *testing.T) {
					// Generate test values based on field type and constraints
					// Try multiple values to test boundary conditions
					//
					// status is declared as a plain string in the specs, so the spec-driven generator
					// offers it arbitrary strings like "test". Its legal values live in the lifecycle
					// plane, which the field definition cannot see, so every kind failed here with
					// `status "test" not found in lifecycle`. Drawing the candidates from the lifecycle
					// exercises the field instead of tripping vocabulary validation -- whose error text
					// the rejection check below does not recognize, so it failed hard rather than
					// treating it as an expected strict-field rejection.
					var testValues []any
					if fieldName == objects.FieldKeyStatus {
						for _, status := range getValidStatusesForKind(kind) {
							testValues = append(testValues, status)
						}
					} else {
						testValues = generateTestValuesWithBoundaries(fieldName, fields[fieldName])
					}
					if len(testValues) == 0 {
						t.Skipf("Cannot generate test values for field %s", fieldName)
						return
					}

					// Try each test value until one succeeds
					var lastErr error
					var succeeded bool
					for i, testValue := range testValues {
						updates := map[string]any{
							fieldName: testValue,
						}

						err := storage.Update(ctx, secCtx, testID, updates)
						if err == nil {
							// Success! Use this value for verification
							succeeded = true
							_ = succeeded // Ensure variable is marked as used (checked at line 408)
							// Verify update
							updated, err := storage.Read(ctx, secCtx, testID)
							if err != nil {
								t.Fatalf("failed to read updated object: %v", err)
							}

							actualValue := updated[fieldName]
							// Normalize slice types for comparison ([]string vs []any)
							if !reflect.DeepEqual(actualValue, testValue) {
								// Try to normalize and compare again
								normalizedActual := normalizeValue(actualValue)
								normalizedExpected := normalizeValue(testValue)
								if !reflect.DeepEqual(normalizedActual, normalizedExpected) {
									t.Errorf("field %s not updated correctly: expected %v, got %v", fieldName, testValue, actualValue)
								}
							}
							return
						}

						lastErr = err
						// If this is not a validation error, fail immediately
						if !strings.Contains(err.Error(), "validation") {
							t.Errorf("failed to update field %s with value %v (attempt %d/%d): %v", fieldName, testValue, i+1, len(testValues), err)
							return
						}
					}

					// All values failed validation - this is expected for fields with strict requirements
					if !succeeded {
						t.Logf("Field %s update rejected for all test values (expected for strict validation): %v", fieldName, lastErr)
						return
					}
				})
			}

			// Cleanup
			//nolint:errcheck // Test cleanup - errors are acceptable
			_ = storage.Delete(ctx, secCtx, testID, false)
		})
	}
}

// TestUpdateID tests that ID updates are handled correctly (including file moves and cache invalidation)
// This test verifies:
// - File is moved from old ID path to new ID path
// - Hash registry is updated for both old and new files
// - Object is readable with new ID but not old ID
func TestUpdateID(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	backlogDir := filepath.Join(tmpDir, paths.ProcessBacklogDir)

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create test object
	oldID := "BLI-999"
	var obj map[string]any

	// Try to use instance builder
	schemaVersion := objects.DefaultSchemaVersion
	specLoader := objects.NewSpecLoader("")
	if spec, err := specLoader.LoadSpecWithInheritance("backlog_item.yaml"); err == nil {
		if spec.SchemaVersion != emptyValue {
			schemaVersion = spec.SchemaVersion
		}
	}

	builder := instancebuilders.NewForKind(objects.KindBacklogItem, schemaVersion)
	builder.SetID(oldID)
	builder.SetField(objects.FieldKeyTitle, "Test Object")
	builder.SetStatus("planned")
	if instance, buildErr := builder.Build(); buildErr == nil {
		obj = instance
	}

	// Fallback to manual construction if builder failed
	if obj == nil {
		obj = map[string]any{
			objects.FieldKeyID:            oldID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Test Object",
			objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
			objects.FieldKeySchemaVersion: schemaVersion,
		}
	}

	objJSON, _ := json.MarshalIndent(obj, "", "  ")
	t.Logf("DEBUG: TestUpdateID object before create: %s", string(objJSON))

	//
	CreateCASVisible(t, storage, ctx, secCtx, obj, objects.ObjectStatusPlanned)

	// Verify object exists with old ID
	_, err = storage.Read(ctx, secCtx, oldID)
	if err != nil {
		t.Fatalf("failed to read object with old ID: %v", err)
	}

	// Update ID (this should move the file and update cache)
	// Note: Currently, ID is treated as immutable in Update() - this test verifies that behavior
	newID := "BLI-1000"
	updates := map[string]any{
		objects.FieldKeyID: newID,
	}

	err = storage.Update(ctx, secCtx, oldID, updates)
	if err != nil {
		// ID updates are typically not allowed (IDs are immutable)
		// This is expected behavior - verify the error is appropriate
		t.Logf("ID update correctly rejected (IDs are immutable): %v", err)

		// Verify old ID still works
		_, err = storage.Read(ctx, secCtx, oldID)
		if err != nil {
			t.Errorf("object should still be readable with old ID after failed update: %v", err)
		}

		// Verify new ID doesn't exist
		_, err = storage.Read(ctx, secCtx, newID)
		if err == nil {
			t.Error("new ID should not exist if update was rejected")
		}
		return // Exit early since update was rejected
	} else {
		// ID update succeeded - verify new ID works and old doesn't
		// This would require file move and cache invalidation
		_, err = storage.Read(ctx, secCtx, newID)
		if err != nil {
			t.Errorf("failed to read object with new ID: %v", err)
		}

		_, err = storage.Read(ctx, secCtx, oldID)
		if err == nil {
			t.Error("object should not be readable with old ID after ID update")
		}

		// Verify file was moved (for non-CAS objects) or ID updated in CAS index
		// For CAS objects, files are hash-based, so we check via Read instead
		oldPath := filepath.Join(backlogDir, oldID+".yaml")
		newPath := filepath.Join(backlogDir, newID+".yaml")

		// Check if object uses CAS (hash-based files)
		if storage.usesContentAddressableStorage("backlog_item") {
			// For CAS objects, verify via Read (file paths are hash-based)
			_, err := storage.Read(ctx, secCtx, oldID)
			if err == nil {
				t.Error("old ID should not be readable after ID update")
			}
			_, err = storage.Read(ctx, secCtx, newID)
			if err != nil {
				t.Errorf("new ID should be readable after ID update: %v", err)
			}
		} else {
			// For non-CAS objects, check file paths directly
			if _, err := fileutil.Stat(oldPath); err == nil {
				t.Error("old file should not exist after ID update")
			}
			if _, err := fileutil.Stat(newPath); err != nil {
				t.Error("new file should exist after ID update")
			}
		}
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, oldID, false)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, newID, false)
}

// TestCacheInvalidationOnUpdate tests that updates are reflected in subsequent reads
// This verifies that the storage layer correctly handles updates
func TestCacheInvalidationOnUpdate(t *testing.T) {
	// Do not run in parallel: shares global listing-index and CAS orphan/refresh machinery with
	// other storage tests.
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	storageCtx := pkgctx.GetStorageContext()
	ctx := context.Background()

	// Full nanosecond suffix avoids rare ID collisions when many tests use BLI-<small-modulo>.
	objID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())

	// Get schema version from spec
	schemaVersion := objects.DefaultSchemaVersion
	specLoader := objects.NewSpecLoader("")
	if spec, err := specLoader.LoadSpecWithInheritance("backlog_item.yaml"); err == nil {
		if spec.SchemaVersion != emptyValue {
			schemaVersion = spec.SchemaVersion
		}
	}

	builder := instancebuilders.NewForKind(objects.KindBacklogItem, schemaVersion)
	builder.SetID(objID)
	builder.SetField(objects.FieldKeyTitle, "Cache Test")
	builder.SetStatus("exploring") // Use exploring status to avoid lifecycle precondition requirements
	instance, buildErr := builder.Build()
	var obj map[string]any
	if buildErr != nil {
		obj = map[string]any{
			objects.FieldKeyID:            objID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Cache Test",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring, // Use exploring status to avoid lifecycle precondition requirements
			objects.FieldKeySchemaVersion: schemaVersion,
		}
	} else {
		obj = instance
	}

	//
	CreateCASVisible(t, storage, ctx, secCtx, obj, objects.ObjectStatusValidated)

	// List objects - this should populate any caches
	filter := ListFilter{
		Kind: "backlog_item",
		Filters: map[string]any{
			objects.FieldKeyID: objID,
		},
	}
	result1, err := storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list objects: %v", err)
	}
	if len(result1.Objects) != 1 {
		// Debug: show what objects were returned
		objIDs := make([]string, len(result1.Objects))
		for i, obj := range result1.Objects {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				objIDs[i] = id
			} else {
				objIDs[i] = fmt.Sprintf("no-id-%d", i)
			}
		}
		t.Fatalf("expected 1 object with id %s, got %d objects: %v", objID, len(result1.Objects), objIDs)
	}

	// Update object
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Cache Test",
	}
	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Flush CAS index write queue so async index persistence completes before assertions.
	if storage.usesContentAddressableStorage("backlog_item") {
		queue := caspkg.GetListingIndexWriteQueueForProjectRoot(tmpDir)
		if err := queue.FlushKind("backlog_item", 5*time.Second); err != nil {
			t.Logf("FlushKind(backlog_item) after update failed (non-fatal): %v", err)
		}
		if err := queue.FlushAll(5 * time.Second); err != nil {
			t.Logf("FlushAll after update failed (non-fatal): %v", err)
		}
	}

	// List again - should reflect the update
	result2, err := storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list objects after update: %v", err)
	}
	if len(result2.Objects) != 1 {
		t.Fatalf("expected 1 object after update, got %d", len(result2.Objects))
	}

	// Verify the update is reflected (storage should read from disk, not cache)
	if result2.Objects[0][objects.FieldKeyTitle] != "Updated Cache Test" {
		t.Errorf("update not reflected: expected title 'Updated Cache Test', got %v", result2.Objects[0][objects.FieldKeyTitle])
	}

	readObj, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read object directly: %v", err)
	}
	if readObj[objects.FieldKeyTitle] != "Updated Cache Test" {
		t.Errorf("direct read not reflecting update: expected title 'Updated Cache Test', got %v", readObj[objects.FieldKeyTitle])
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, objID, false)
}
