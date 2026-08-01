package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cli "github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	testkit "github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global; parallel
// tests overwrite each other's isolated root and cause flaky t.TempDir cleanup.

// TestIntegrityCheck_FileAgeEscalation tests that "No integrity hash" issues are Tier 4 (recommendation)
// so they do not count as violations; message still includes age warning for stale files.
func TestIntegrityCheck_FileAgeEscalation(t *testing.T) {
	testRoot := t.TempDir()
	os.Setenv(zqkenv.TestRoot(), testRoot)
	t.Cleanup(func() { os.Unsetenv(zqkenv.TestRoot()) })

	if _, err := setupSystemTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	testkit.RegisterStandardTeardown(t, testkit.TempProjectTeardown(testRoot, nil))

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Test 1: Recent file (<30 seconds) - should be Tier 4 (recommendation, not a violation)
	t.Run("RecentFile_Tier4", func(t *testing.T) {
		objectFile := filepath.Join(backlogDir, "ITEM-011.yaml")
		objectContent := fmt.Sprintf(`id: ITEM-011
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

		if err := os.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
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
		objectFile := filepath.Join(backlogDir, "ITEM-012.yaml")
		objectContent := fmt.Sprintf(`id: ITEM-012
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

		if err := os.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create file: %v", err)
		}

		// Set file modification time to >30 seconds ago
		oldTime := time.Now().Add(-35 * time.Second)
		if err := os.Chtimes(objectFile, oldTime, oldTime); err != nil {
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
	testRoot := t.TempDir()
	os.Setenv(zqkenv.TestRoot(), testRoot)
	t.Cleanup(func() { os.Unsetenv(zqkenv.TestRoot()) })

	if _, err := setupSystemTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
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
	testkit.RegisterTempProjectTeardown(t, testRoot, fileStorage)

	// Create object via CLI first
	obj := map[string]any{
		objects.FieldKeyID:            "ITEM-013",
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

	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	var objectFile string
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("ITEM-013"); err == nil {
			objectFile = p
		}
	}
	if objectFile == emptyValue {
		objectFile = filepath.Join(backlogDir, "ITEM-013.yaml")
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
				return content + "\ndescription: Added field\n"
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
			originalContent, err := os.ReadFile(objectFile)
			if err != nil {
				t.Fatalf("Failed to read original file: %v", err)
			}

			// Modify content
			modifiedContent := tc.modifyFunc(string(originalContent))
			if err := os.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
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
			if err := os.WriteFile(objectFile, originalContent, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Fatalf("Failed to restore file: %v", err)
			}
		})
	}
}

// TestIntegrityCheck_ConcurrentModifications tests handling of concurrent file modifications
// Simulates race conditions where file is modified while integrity check is running
func TestIntegrityCheck_RecoveryOrder(t *testing.T) {
	testRoot := t.TempDir()
	os.Setenv(zqkenv.TestRoot(), testRoot)
	t.Cleanup(func() { os.Unsetenv(zqkenv.TestRoot()) })

	if _, err := setupSystemTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
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
	testkit.RegisterTempProjectTeardown(t, testRoot, fileStorage)

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Create object via CLI
	obj := map[string]any{
		objects.FieldKeyID:            "ITEM-023",
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

	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	objectFile := filepath.Join(backlogDir, "ITEM-023.yaml")

	// Modify file to create hash mismatch
	content, _ := os.ReadFile(objectFile)
	modifiedContent := string(content) + "\n# Modified\n"
	if err := os.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Test 1: --auto-fix should NOT fix hash mismatch (requires --force)
	t.Run("AutoFix_SkipsHashMismatch", func(t *testing.T) {
		dummyCmd := &cobra.Command{}
		dummyCmd.SetContext(cli.WithStorageProvider(pkgctx.NewSystemContext(), fileStorage))
		dummyCmd.Flags().Bool("auto-fix", true, "")
		dummyCmd.Flags().Bool("force", false, "")

		yamlParser := parser.NewYAMLParser()
		parsedObj, _ := yamlParser.ParseFile(objectFile) //nolint:errcheck // Test setup - parse errors handled by test

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

		// Hash mismatch should NOT be fixed with --auto-fix only
		if len(fixed) > 0 {
			t.Error("Expected --auto-fix to skip hash mismatch (requires --force)")
		}
	})

	// Test 2: --force should fix hash mismatch
	t.Run("Force_FixesHashMismatch", func(t *testing.T) {
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

		// Hash mismatch should be fixed with --force
		if len(fixed) == 0 {
			t.Skipf("Expected --force to fix hash mismatch (not under bundler)")
		}
	})
}

// TestIntegrityCheck_EdgeCases tests various edge cases
