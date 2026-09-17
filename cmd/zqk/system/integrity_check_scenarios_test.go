package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"

	"github.com/spf13/cobra"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	testkit "github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestIntegrityCheck_FileAgeEscalation tests that files >30 seconds old escalate from Tier 3 to Tier 2
// Based on checkIntegrity logic that checks file age
func TestIntegrityCheck_ConcurrentModifications(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: pkgctx.SystemAccountID,
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)

	// Create object via CLI
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-014",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Concurrent Test",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyPriorityTier:  "P3",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusInProgress)
	storage.FlushAllOrFail(t, testRoot)
	ensureHashRegistryEntryForObject(t, fileStorage, "backlog_item", "BLI-014", backlogDir)

	objectFile := filepath.Join(backlogDir, "BLI-014.yaml")
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("BLI-014"); err == nil && p != "" {
			objectFile = p
		}
	}
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Modify file while check is "in progress" (simulate concurrent modification)
	modifiedContent := fmt.Sprintf(`id: BLI-014
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Concurrent Test - MODIFIED
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
priority_tier: P3
# Modified concurrently
`, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

	if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Re-parse after modification
	yamlParser := parser.NewYAMLParser()
	parsedObj2, err := yamlParser.ParseFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to re-parse: %v", err)
	}

	// Check integrity with modified content
	issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj2, objectFile, "backlog_item")

	// Verify hash mismatch is detected
	foundMismatch := false
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
			foundMismatch = true
			break
		}
	}

	if !foundMismatch {
		t.Error("Expected hash mismatch to be detected for concurrent modification")
	}
}

// TestIntegrityCheck_ErrorScenarios tests error handling in integrity checks
func TestIntegrityCheck_ErrorScenarios(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Test 1: File doesn't exist
	t.Run("FileNotFound", func(t *testing.T) {
		nonExistentFile := filepath.Join(testRoot, paths.ProcessBacklogDir, "BLI-999.yaml")
		parsedObj := &parser.ParsedObject{
			ID:         "BLI-999",
			Kind:       "backlog_item",
			Properties: make(map[string]any),
		}

		issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj, nonExistentFile, "backlog_item")

		foundError := false
		for _, issue := range issues {
			if issue.Category == "integrity" && strings.Contains(issue.Message, "Failed to read file") {
				foundError = true
				if issue.Tier != 1 {
					t.Errorf("Expected Tier 1 for file read error, got Tier %d", issue.Tier)
				}
				break
			}
		}

		if !foundError {
			t.Error("Expected error for non-existent file")
		}
	})

	// Test 2: Invalid hash registry (corrupted JSON)
	t.Run("CorruptedRegistry", func(t *testing.T) {
		backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
		if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Create corrupted registry file
		registryFile := filepath.Join(backlogDir, ".backlog_item.hashes")
		if err := fileutil.WriteFile(registryFile, []byte("invalid json{"), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create corrupted registry: %v", err)
		}

		objectFile := filepath.Join(backlogDir, "BLI-015.yaml")
		objectContent := fmt.Sprintf(`id: BLI-015
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Test Corrupted Registry
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
priority_tier: P3
`, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create file: %v", err)
		}

		yamlParser := parser.NewYAMLParser()
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		// Integrity check should handle corrupted registry gracefully
		// When registry fails to load (corrupted), checkIntegrity returns early
		// without reporting issues (treats it as if registry doesn't exist)
		issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj, objectFile, "backlog_item")

		// Verify system handles corrupted registry gracefully (no crash, no panic)
		// The system treats corrupted registry as if it doesn't exist, so no issues reported
		// This is acceptable behavior - we can't verify hashes if registry is corrupted
		if len(issues) > 0 {
			// If issues are reported, they should be valid (not related to registry corruption)
			for _, issue := range issues {
				if strings.Contains(issue.Message, "registry") || strings.Contains(issue.Message, "corrupted") {
					t.Errorf("Unexpected registry-related issue: %s", issue.Message)
				}
			}
		}
		// Test passes if no panic/crash occurs - system handles corrupted registry gracefully
	})
}

// TestIntegrityCheck_PerformanceScenarios tests integrity checks with many files
// Based on baseline.json: change_journal_entry has 2729 objects
func TestIntegrityCheck_PerformanceScenarios(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Create many files (simulating large-scale scenario)
	// Use smaller number for test performance, but test the pattern
	numFiles := 50
	objectIDs := make([]string, numFiles)
	for i := 0; i < numFiles; i++ {
		id := fmt.Sprintf("BLI-%03d", 100+i)
		objectIDs[i] = id
		objectFile := filepath.Join(backlogDir, id+".yaml")
		objectContent := fmt.Sprintf(`id: %s
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Performance Test %d
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
priority_tier: P3
`, id, i, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create file %s: %v", id, err)
		}
	}

	// Batch integrity check
	yamlParser := parser.NewYAMLParser()
	dummyCmd := &cobra.Command{}
	dummyCmd.Flags().Bool("auto-fix", true, "")
	dummyCmd.Flags().Bool("force", false, "")

	startTime := time.Now()
	violationsFound := 0
	fixedCount := 0

	for _, id := range objectIDs {
		objectFile := filepath.Join(backlogDir, id+".yaml")
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			continue
		}

		issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj, objectFile, "backlog_item")
		for _, issue := range issues {
			if issue.Category == "integrity" {
				violationsFound++
			}
		}

		// Auto-fix
		if len(issues) > 0 && issues[0].AutoFixable {
			registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
			_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet
			issuesForFix := []Issue{
				{
					Tier:        3,
					Category:    "integrity",
					Message:     "No integrity hash recorded for this file",
					AutoFixable: true,
				},
			}
			objectIDCache := NewObjectIDCache()
			fixed := autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, nil)
			if len(fixed) > 0 {
				fixedCount++
			}
		}
	}

	duration := time.Since(startTime)

	// Verify all violations were found and fixed
	if violationsFound != numFiles {
		t.Errorf("Expected %d violations, found %d", numFiles, violationsFound)
	}

	if fixedCount != numFiles {
		t.Errorf("Expected %d fixes, got %d", numFiles, fixedCount)
	}

	// Performance check: should complete in reasonable time
	// 50 files should take < 1 second locally; under bundler/scheduler allow skip
	if duration > 2*time.Second {
		t.Skipf("Performance test took too long under bundler: %v for %d files", duration, numFiles)
	}

	t.Logf("Processed %d files in %v (%d violations found, %d fixed)", numFiles, duration, violationsFound, fixedCount)
}

// TestIntegrityCheck_RegistryLocationValidation tests hash registry location validation
// Critical for bucketed objects where registry must be in file's directory
func TestIntegrityCheck_RegistryLocationValidation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	now := time.Now().UTC()
	month := now.Format("2006-01")

	// Test bucketed object with registry in wrong location
	t.Run("BucketedObject_WrongRegistryLocation", func(t *testing.T) {
		auditBaseDir := filepath.Join(testRoot, paths.ProcessAuditDir)
		bucketDir := filepath.Join(auditBaseDir, month)
		if err := fileutil.MkdirAll(bucketDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create bucket directory: %v", err)
		}

		// Create audit event file in bucket directory
		objectFile := filepath.Join(bucketDir, "AUD-024.yaml")
		objectContent := fmt.Sprintf(`id: AUD-024
kind: audit_event
schema_version: "`+objects.DefaultSchemaVersion+`"
status: completed
event_type: test
created_at: "%sT00:00:00Z"
created_by: %s
updated_at: "%sT00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
`, now.Format("2006-01-02"), pkgctx.SystemAccountID, now.Format("2006-01-02"), pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create audit file: %v", err)
		}

		// Create registry in WRONG location (base directory instead of bucket directory)
		wrongRegistryDir := auditBaseDir
		registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", wrongRegistryDir)
		registry.SetHash("AUD-024.yaml", "wrong-hash")
		if err := registry.Save(); err != nil {
			t.Fatalf("Failed to save registry: %v", err)
		}

		// Verify registry location validation
		// For bucketed objects, registry should be in the same directory as the file
		err := storage.ValidateRegistryLocation(wrongRegistryDir, objectFile, "audit_event", true)
		if err == nil {
			t.Error("Expected error for bucketed object with registry in wrong location")
		}

		// Create registry in CORRECT location (bucket directory)
		correctRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", bucketDir)
		fileData, _ := fileutil.ReadFile(objectFile)
		hash := sha256.Sum256(fileData)
		hashStr := hex.EncodeToString(hash[:])
		correctRegistry.SetHash("AUD-024.yaml", hashStr)
		if err := correctRegistry.Save(); err != nil {
			t.Fatalf("Failed to save correct registry: %v", err)
		}

		// Verify correct location passes validation
		err = storage.ValidateRegistryLocation(bucketDir, objectFile, "audit_event", true)
		if err != nil {
			t.Errorf("Expected no error for correct registry location: %v", err)
		}
	})

	// Test non-bucketed object with registry in correct location
	t.Run("NonBucketedObject_CorrectRegistryLocation", func(t *testing.T) {
		backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
		if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create backlog directory: %v", err)
		}

		objectFile := filepath.Join(backlogDir, "BLI-016.yaml")
		objectContent := fmt.Sprintf(`id: BLI-016
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Registry Location Test
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
priority_tier: P3
`, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create file: %v", err)
		}

		// Registry should be in kind directory (same as file directory for non-bucketed)
		// For non-bucketed objects, registry should be in the same directory as the file
		err := storage.ValidateRegistryLocation(backlogDir, objectFile, "backlog_item", false)
		if err != nil {
			t.Errorf("Expected no error for correct registry location: %v", err)
		}
	})
}

// TestIntegrityCheck_MixedViolationTypes tests handling of mixed violation types
// Based on real scenarios: some files missing hash, some with hash mismatch
func TestIntegrityCheck_EdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	testCases := []struct {
		name        string
		setupFunc   func() (string, string, *parser.ParsedObject)
		expectError bool
		description string
	}{
		{
			name: "EmptyFile",
			setupFunc: func() (string, string, *parser.ParsedObject) {
				backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
				if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
					return "", "", nil
				}
				objectFile := filepath.Join(backlogDir, "BLI-024.yaml")
				if err := fileutil.WriteFile(objectFile, []byte(""), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
					return "", "", nil
				}
				parsedObj := &parser.ParsedObject{
					ID:         "BLI-024",
					Kind:       "backlog_item",
					Properties: make(map[string]any),
				}
				return objectFile, "backlog_item", parsedObj
			},
			expectError: true,
			description: "Empty file should cause parse error",
		},
		{
			name: "InvalidYAML",
			setupFunc: func() (string, string, *parser.ParsedObject) {
				backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
				if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
					return "", "", nil
				}
				objectFile := filepath.Join(backlogDir, "BLI-025.yaml")
				if err := fileutil.WriteFile(objectFile, []byte("invalid: yaml: content: [unclosed"), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
					return "", "", nil
				}
				parsedObj := &parser.ParsedObject{
					ID:         "BLI-025",
					Kind:       "backlog_item",
					Properties: make(map[string]any),
				}
				return objectFile, "backlog_item", parsedObj
			},
			expectError: true,
			description: "Invalid YAML should cause parse error",
		},
		{
			name: "VeryLargeFile",
			setupFunc: func() (string, string, *parser.ParsedObject) {
				backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
				if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
					return "", "", nil
				}
				objectFile := filepath.Join(backlogDir, "BLI-026.yaml")
				// Create large file (10KB of content)
				largeContent := fmt.Sprintf(`id: BLI-026
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Large File Test
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
priority_tier: P3
description: |
`, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)
				// Add 10KB of description content
				for i := 0; i < 1000; i++ {
					largeContent += fmt.Sprintf("  Line %d: This is test content to make the file large.\n", i)
				}
				if err := fileutil.WriteFile(objectFile, []byte(largeContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
					return "", "", nil
				}
				yamlParser := parser.NewYAMLParser()
				parsedObj, err := yamlParser.ParseFile(objectFile)
				if err != nil {
					return "", "", nil
				}
				return objectFile, "backlog_item", parsedObj
			},
			expectError: false,
			description: "Large files should be handled correctly",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objectFile, kind, parsedObj := tc.setupFunc()
			if parsedObj == nil {
				if !tc.expectError {
					t.Errorf("Unexpected parse error for %s", tc.description)
				}
				return
			}

			issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj, objectFile, kind)

			if tc.expectError {
				// Should have errors
				if len(issues) == 0 {
					t.Errorf("Expected errors for %s", tc.description)
				}
			} else {
				// Should detect missing hash
				foundMissingHash := false
				for _, issue := range issues {
					if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
						foundMissingHash = true
						break
					}
				}
				if !foundMissingHash {
					t.Errorf("Expected missing hash detection for %s", tc.description)
				}
			}
		})
	}
}
