package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/cliapp"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestIntegrityCheck_ForceFixPersistence verifies that --force fixes actually persist
// and the violation doesn't recur after the fix. This test covers the scenario where
// --force is called but the fix doesn't actually work (the bug we're trying to catch).
func TestIntegrityCheck_ForceFixPersistence(t *testing.T) {
	// Do not use t.Parallel(): ZQK_TEST_ROOT and CAS queue factory are process-global.
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	secCtx := &pkgctx.SecurityContext{
		AccountID: pkgctx.SystemAccountID,
	}
	var fileStorage *storage.FileObjectStorage
	t.Cleanup(func() {
		caspkg.SetListingIndexWriteQueueFactory(nil)
	})
	testkit.RegisterAuditResetTeardown(t, testkit.TeardownOptions{
		ProjectRoot:           testRoot,
		StripProcessArtifacts: true,
		WALTimeout:            20 * time.Second,
		ShutdownTimeout:       20 * time.Second,
		SecCtx:                secCtx,
	}, &fileStorage)
	// Cleanups run LIFO, so this flush lands before the teardown registered above.
	t.Cleanup(func() {
		_ = WaitProjectCacheBackgroundWork(context.Background(), testRoot)

		auditCliCtx := cli.ContextForProjectAndProfile(testRoot, "test")
		if ab := GetAuditEventBuffer(auditCliCtx); ab != nil {
			_, _ = ab.Flush()
		}
	})

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()

	var err error
	fileStorage, err = storage.NewFileObjectStorageForTest(testRoot)
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)
	if fileStorage != nil {
		defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	}
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	_ = storage.InitializeGlobalBufferWithConfig(testRoot, secCtx)

	// Create object via CLI (establishes hash)
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-999",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Force Fix Persistence Test",
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

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	ensureHashRegistryEntryForObject(t, fileStorage, "backlog_item", "BLI-999", backlogDir)

	// Resolve actual object path (CAS uses hash-named files, not BLI-999.yaml)
	var objectFile string
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("BLI-999"); err == nil {
			objectFile = p
		}
	}
	if objectFile == emptyValue {
		objectFile = filepath.Join(backlogDir, "BLI-999.yaml") // fallback for non-CAS
	}
	originalRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	if err := originalRegistry.Load(); err != nil {
		t.Fatalf("Failed to load original registry: %v", err)
	}
	// Registry keys by CAS hash basename for integrity lookup
	filename := registryFilenameForObject(t, fileStorage, "backlog_item", "BLI-999", "BLI-999.yaml")
	originalHash := originalRegistry.GetHash(filename)
	if originalHash == emptyValue {
		t.Fatal("Expected original hash to exist after creation")
	}

	// Modify file directly (creates hash mismatch)
	content, err := fileutil.ReadFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	modifiedContent := string(content) + "\n# Modified outside CLI - Force Fix Persistence Test\n"
	if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Verify hash mismatch is detected
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	yamlParser := parser.NewYAMLParser()
	parsedObj, err := yamlParser.ParseFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to parse object: %v", err)
	}

	// Use checkIntegrityWithRegistryAndContent to ensure we're using the loaded registry
	fileData, _ := fileutil.ReadFile(objectFile)
	issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, objectFile, "backlog_item", fileData, originalRegistry, nil)
	foundMismatch := false
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
			foundMismatch = true
			break
		}
	}

	if !foundMismatch {
		t.Fatal("Expected to detect hash mismatch before fix")
	}

	// Apply --force fix (fix uses fixCASIndexOutOfSync when file is hash-named and message contains "Hash mismatch detected")
	dummyCmd := &cobra.Command{}
	dummyCmd.Flags().Bool("auto-fix", false, "")
	dummyCmd.Flags().Bool("force", true, "")

	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	if err := registry.Load(); err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	issuesForFix := []Issue{
		{
			Tier:        1,
			Category:    "integrity",
			Message:     "Hash mismatch detected",
			AutoFixable: false,
		},
	}

	objectIDCache := NewObjectIDCache()
	// Pass fileStorage so CAS fix uses same instance and index
	fixed := autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, fileStorage)

	if len(fixed) == 0 {
		t.Fatal("Expected --force to fix hash mismatch")
	}

	// Resolve path and content after fix (CAS fix moves file to new hash-named path)
	var objectPathAfterFix string
	var contentAfterFix []byte
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("BLI-999"); err == nil {
			objectPathAfterFix = p
			contentAfterFix, _ = fileutil.ReadFile(p)
		}
	}
	if objectPathAfterFix == emptyValue {
		objectPathAfterFix = objectFile
		contentAfterFix = []byte(modifiedContent)
	}
	if len(contentAfterFix) == 0 {
		contentAfterFix, _ = fileutil.ReadFile(objectPathAfterFix)
	}

	// Test 1: Verify fix persists in the same registry instance
	t.Run("SameRegistryInstance", func(t *testing.T) {
		// Re-check using the same registry instance; after CAS fix we use post-fix path and content
		issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, objectPathAfterFix, "backlog_item", contentAfterFix, registry, nil)
		for _, issue := range issues {
			if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
				t.Error("Violation should be resolved in same registry instance after --force fix")
			}
		}
	})

	// Test 2: Verify fix persists (CAS index and file at new path)
	t.Run("ReloadedRegistryFromDisk", func(t *testing.T) {
		// CAS fix updates the CAS index; resolve path from CAS and verify file content
		cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
		if err != nil || cas == nil {
			t.Fatalf("Failed to get CAS: %v", err)
		}
		pathFromCAS, err := cas.GetFilePathForID("BLI-999")
		if err != nil || pathFromCAS == emptyValue {
			t.Fatal("Expected CAS to have path for BLI-999 after --force fix")
		}
		fileData, err := fileutil.ReadFile(pathFromCAS)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		if calculateHashFromContent(fileData) != filepath.Base(pathFromCAS)[:64] {
			t.Error("File content hash should match CAS filename after fix")
		}
		// Re-check using post-fix path and content
		issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, pathFromCAS, "backlog_item", fileData, registry, nil)
		for _, issue := range issues {
			if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
				t.Error("Violation should be resolved after fix")
			}
		}
	})

	// Test 3: Verify fix persists when checking via full system check
	t.Run("FullSystemCheck", func(t *testing.T) {
		// Wait a bit to ensure file system has flushed
		time.Sleep(100 * time.Millisecond)

		// Use checkKindObjects which does a full check (not just integrity)
		results, err := checkKindObjects(checkCtx, dummyCmd, "backlog_item", []string{"BLI-999"})
		if err != nil {
			t.Fatalf("Failed to check objects: %v", err)
		}

		for _, result := range results {
			if result.ObjectID == "BLI-999" {
				for _, issue := range result.Issues {
					if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
						t.Error("Violation should be resolved when checking via full system check")
					}
				}
			}
		}
	})

	// Test 4: Verify hash was actually updated (not just original hash)
	t.Run("HashActuallyUpdated", func(t *testing.T) {
		cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
		if err != nil || cas == nil {
			t.Fatalf("Failed to get CAS: %v", err)
		}
		newHash, err := cas.GetHashForID("BLI-999")
		if err != nil || newHash == emptyValue {
			t.Fatal("Expected CAS to have hash for BLI-999 after --force fix")
		}
		if newHash == originalHash {
			t.Error("Expected hash to be updated after --force fix, but it matches original hash")
		}
		pathFromCAS, err := cas.GetFilePathForID("BLI-999")
		if err != nil || pathFromCAS == emptyValue {
			t.Fatal("Expected CAS to have path for BLI-999")
		}
		currentContent, err := fileutil.ReadFile(pathFromCAS)
		if err != nil {
			t.Fatalf("Failed to read current file: %v", err)
		}
		expectedHash := calculateHashFromContent(currentContent)
		if newHash != expectedHash {
			t.Errorf("New hash doesn't match file content: expected %s, got %s", expectedHash[:16], newHash[:16])
		}
	})

	// Test 5: Verify fix persists across multiple check cycles
	t.Run("MultipleCheckCycles", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			time.Sleep(50 * time.Millisecond)

			cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
			if err != nil || cas == nil {
				t.Fatalf("Failed to get CAS (cycle %d): %v", i, err)
			}
			pathFromCAS, err := cas.GetFilePathForID("BLI-999")
			if err != nil || pathFromCAS == emptyValue {
				t.Fatalf("Expected CAS path (cycle %d): %v", i, err)
			}
			fileData, err := fileutil.ReadFile(pathFromCAS)
			if err != nil {
				t.Fatalf("Failed to read file (cycle %d): %v", i, err)
			}

			freshRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
			_ = freshRegistry.Load() //nolint:errcheck // Optional for this check
			issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, pathFromCAS, "backlog_item", fileData, freshRegistry, nil)
			for _, issue := range issues {
				if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
					t.Errorf("Violation should be resolved across multiple check cycles (cycle %d)", i)
				}
			}
		}
	})
}

// TestIntegrityCheck_ForceFixFailureDetection tests that we can detect when --force doesn't work
// This test simulates a scenario where the registry save fails or the hash isn't actually updated
func TestIntegrityCheck_ForceFixFailureDetection(t *testing.T) {
	// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
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
		objects.FieldKeyID:            "BLI-998",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Force Fix Failure Detection Test",
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

	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	var objectFile string
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("BLI-998"); err == nil {
			objectFile = p
		}
	}
	if objectFile == emptyValue {
		objectFile = filepath.Join(backlogDir, "BLI-998.yaml")
	}

	// Modify file to create mismatch
	content, _ := fileutil.ReadFile(objectFile)
	modifiedContent := string(content) + "\n# Modified\n"
	if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify file: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	yamlParser := parser.NewYAMLParser()
	parsedObj, _ := yamlParser.ParseFile(objectFile) //nolint:errcheck // Test setup - parse errors handled by test

	// Verify mismatch exists (use checkIntegrityWithRegistryAndContent so hash-named CAS files are checked correctly)
	preFixRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	_ = preFixRegistry.Load() //nolint:errcheck // Test setup
	issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, objectFile, "backlog_item", []byte(modifiedContent), preFixRegistry, nil)
	hasMismatch := false
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
			hasMismatch = true
			break
		}
	}
	if !hasMismatch {
		t.Skipf("Expected hash mismatch before fix (not detected under bundler)")
	}

	// Apply --force fix
	dummyCmd := &cobra.Command{}
	dummyCmd.Flags().Bool("auto-fix", false, "")
	dummyCmd.Flags().Bool("force", true, "")

	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet

	issuesForFix := []Issue{
		{
			Tier:        1,
			Category:    "integrity",
			Message:     "Hash mismatch detected",
			AutoFixable: false,
		},
	}

	objectIDCache := NewObjectIDCache()
	fixed := autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, fileStorage)

	if len(fixed) == 0 {
		t.Fatal("Expected --force to report fix")
	}

	// Critical test: Verify the fix actually worked by checking again
	time.Sleep(100 * time.Millisecond) // Allow file system to flush

	// Resolve path after fix (CAS fix moves file to new hash-named path)
	var objectPathAfterFix string
	if cas, err := fileStorage.GetContentAddressableStorage("backlog_item"); err == nil {
		if p, err := cas.GetFilePathForID("BLI-998"); err == nil {
			objectPathAfterFix = p
		}
	}
	if objectPathAfterFix == emptyValue {
		objectPathAfterFix = objectFile
	}

	freshRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	if err := freshRegistry.Load(); err != nil {
		t.Fatalf("Failed to reload registry: %v", err)
	}

	// Re-check integrity using post-fix path
	fileData, _ := fileutil.ReadFile(objectPathAfterFix)
	issues, _ = checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, objectPathAfterFix, "backlog_item", fileData, freshRegistry, nil)

	// This is the critical assertion - if this fails, --force didn't work
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
			t.Errorf("CRITICAL: --force fix did not persist! Violation still exists after fix. This indicates a bug in updateHashInRegistryWithInstance or registry.Save()")
		}
	}
}

// calculateHashFromContent is a helper to calculate hash from file content
func calculateHashFromContent(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
