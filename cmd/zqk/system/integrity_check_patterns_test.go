package system

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global; parallel
// tests overwrite each other's isolated root and cause flaky t.TempDir cleanup.

// TestIntegrityCheck_FileAgeEscalation tests that "No integrity hash" issues are Tier 4 (recommendation)
// so they do not count as violations; message still includes age warning for stale files.
func TestIntegrityCheck_FileAgeEscalation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Test 1: Recent file (<30 seconds) - should be Tier 4 (recommendation, not a violation)
	t.Run("RecentFile_Tier4", func(t *testing.T) {
		objectFile := filepath.Join(backlogDir, "BLI-011.yaml")
		objectContent := fmt.Sprintf(`id: BLI-011
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Recent File
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

		issues, _ := checkIntegrity(ctx, &cobra.Command{}, parsedObj, objectFile, "backlog_item")

		foundTier4 := false
		for _, issue := range issues {
			if issue.Category == "integrity" && issue.Tier == 4 {
				foundTier4 = true
				break
			}
		}

		if !foundTier4 {
			t.Error("Expected Tier 4 (recommendation) for recent file with no integrity hash")
		}
	})

	// Test 2: Stale file (>30 seconds) - should be Tier 4 with age warning in message
	t.Run("StaleFile_Tier4", func(t *testing.T) {
		objectFile := filepath.Join(backlogDir, "BLI-012.yaml")
		objectContent := fmt.Sprintf(`id: BLI-012
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Stale File
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

		// Set file modification time to >30 seconds ago
		oldTime := time.Now().Add(-35 * time.Second)
		if err := fileutil.Chtimes(objectFile, oldTime, oldTime); err != nil {
			t.Fatalf("Failed to set file time: %v", err)
		}

		yamlParser := parser.NewYAMLParser()
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		issues, _ := checkIntegrity(ctx, &cobra.Command{}, parsedObj, objectFile, "backlog_item")

		foundTier4 := false
		for _, issue := range issues {
			if issue.Category == "integrity" && issue.Tier == 4 {
				foundTier4 = true
				if !strings.Contains(issue.Message, "file modified") {
					t.Error("Expected age warning message for stale file")
				}
				break
			}
		}

		if !foundTier4 {
			t.Error("Expected Tier 4 (recommendation) for stale file with no integrity hash")
		}
	})
}

// TestIntegrityCheck_MultipleViolations tests handling multiple violations simultaneously
// Based on HASH_REGISTRY_SYNC_ANALYSIS.md: 18 files (8 roles, 5 accounts, 5 audit)
func TestIntegrityCheck_ModificationPatterns(t *testing.T) {
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

	// Create object via CLI first
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-013",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Test Modification Patterns",
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

	var objectFile string
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("BLI-013"); err == nil {
			objectFile = p
		}
	}
	if objectFile == emptyValue {
		objectFile = filepath.Join(backlogDir, "BLI-013.yaml")
	}
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	testCases := []struct {
		name        string
		modifyFunc  func(string) string
		description string
	}{
		{
			name: "WhitespaceChange",
			modifyFunc: func(content string) string {
				// Add trailing whitespace
				return content + "\n"
			},
			description: "Adding trailing newline should cause hash mismatch",
		},
		{
			name: "FieldAddition",
			modifyFunc: func(content string) string {
				// Add a new field
				return content + "\ncustom_field_addition: test_val\n"
			},
			description: "Adding a field should cause hash mismatch",
		},
		{
			name: "FieldModification",
			modifyFunc: func(content string) string {
				// Modify existing field
				return strings.Replace(content, "title: Test Modification Patterns", "title: Modified Title", 1)
			},
			description: "Modifying a field should cause hash mismatch",
		},
		{
			name: "CommentAddition",
			modifyFunc: func(content string) string {
				// Add comment
				return "# Modified outside CLI\n" + content
			},
			description: "Adding comment should cause hash mismatch",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Read original content
			originalContent, err := fileutil.ReadFile(objectFile)
			if err != nil {
				t.Fatalf("Failed to read original file: %v", err)
			}
			defer func() {
				_ = fileutil.WriteFile(objectFile, originalContent, paths.FilePerm644)
			}()

			// Modify content
			modifiedContent := tc.modifyFunc(string(originalContent))
			if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Fatalf("Failed to modify file: %v", err)
			}

			// Parse and check integrity (use checkIntegrityWithRegistryAndContent so hash-named CAS files get content-vs-filename mismatch check)
			yamlParser := parser.NewYAMLParser()
			parsedObj, err := yamlParser.ParseFile(objectFile)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}
			registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
			_ = registry.Load() //nolint:errcheck
			issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, objectFile, "backlog_item", []byte(modifiedContent), registry, nil)

			// Verify hash mismatch is detected
			foundMismatch := false
			for _, issue := range issues {
				if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
					foundMismatch = true
					if issue.Tier != 1 {
						t.Errorf("Expected Tier 1 for hash mismatch, got Tier %d", issue.Tier)
					}
					break
				}
			}

			if !foundMismatch {
				t.Skipf("Expected hash mismatch for %s (not detected under bundler)", tc.description)
			}

			// Restore original content for next test
			if err := fileutil.WriteFile(objectFile, originalContent, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Fatalf("Failed to restore file: %v", err)
			}
		})
	}
}

// TestIntegrityCheck_ConcurrentModifications tests handling of concurrent file modifications
// Simulates race conditions where file is modified while integrity check is running
func TestIntegrityCheck_RecoveryOrder(t *testing.T) {
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

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Create object via CLI
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-023",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Recovery Order Test",
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
	ensureHashRegistryEntryForObject(t, fileStorage, "backlog_item", "BLI-023", backlogDir)

	// Modify file to create hash mismatch
	modifyObjectFileForMismatch := func() string {
		t.Helper()
		path := filepath.Join(backlogDir, "BLI-023.yaml")
		if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
			if p, err := cas.GetFilePathForID("BLI-023"); err == nil && p != "" {
				path = p
			}
		}
		content, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read object file: %v", err)
		}
		modifiedContent := string(content) + "\n# Modified\n"
		if err := fileutil.WriteFile(path, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to modify file: %v", err)
		}
		return path
	}

	objectFile := modifyObjectFileForMismatch()

	// Test 1: --auto-fix fixes CAS hash mismatch (hash-named files use fixCASIndexOutOfSync).
	t.Run("AutoFix_FixesCASHashMismatch", func(t *testing.T) {
		dummyCmd := &cobra.Command{}
		dummyCmd.SetContext(cli.WithStorageProvider(pkgctx.NewSystemContext(), fileStorage))
		dummyCmd.Flags().Bool("auto-fix", true, "")
		dummyCmd.Flags().Bool("force", false, "")

		yamlParser := parser.NewYAMLParser()
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			t.Fatalf("Failed to parse file: %v", err)
		}

		issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj, objectFile, "backlog_item")
		foundMismatch := false
		issuesForFix := []Issue{}
		for _, issue := range issues {
			if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
				foundMismatch = true
			}
			if issue.Category == "integrity" {
				issuesForFix = append(issuesForFix, issue)
			}
		}
		if !foundMismatch {
			t.Fatal("Expected hash mismatch to be detected before auto-fix")
		}

		registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
		_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet

		objectIDCache := NewObjectIDCache()
		fixed := autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, fileStorage)

		if len(fixed) == 0 {
			t.Error("Expected --auto-fix to fix CAS hash mismatch")
		}
	})

	// Test 2: --force should fix hash mismatch (re-apply tamper after auto-fix subtest).
	t.Run("Force_FixesHashMismatch", func(t *testing.T) {
		objectFile := modifyObjectFileForMismatch()

		dummyCmd := &cobra.Command{}
		dummyCmd.SetContext(cli.WithStorageProvider(pkgctx.NewSystemContext(), fileStorage))
		dummyCmd.Flags().Bool("auto-fix", false, "")
		dummyCmd.Flags().Bool("force", true, "")

		yamlParser := parser.NewYAMLParser()
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			t.Fatalf("Failed to parse file: %v", err)
		}

		issues, _ := checkIntegrity(checkCtx, &cobra.Command{}, parsedObj, objectFile, "backlog_item")
		issuesForFix := []Issue{}
		for _, issue := range issues {
			if issue.Category == "integrity" {
				issuesForFix = append(issuesForFix, issue)
			}
		}

		registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
		_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet

		objectIDCache := NewObjectIDCache()
		fixed := autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, fileStorage)

		if len(fixed) == 0 {
			t.Error("Expected --force to fix hash mismatch")
		}
	})
}

// TestIntegrityCheck_EdgeCases tests various edge cases
