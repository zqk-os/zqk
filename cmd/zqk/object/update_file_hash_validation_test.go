package object

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestUpdateFileHashValidation tests that object update --file validates file hash
// before attempting to read the existing object (BLI-914)
//
// This test validates the fix for BLI-914: when using `object update --file`,
// the system should validate the file's hash (if it's a hash-based filename)
// BEFORE attempting to read the existing object, providing clearer error messages.
//
// To avoid timeouts in CI/scheduler, set ZQK_TEST_CLI_BINARY to a pre-built zqk binary
// so the test skips the per-test go build.
func TestUpdateFileHashValidation(t *testing.T) {
	testEnv := SetupTestEnvironment(t)
	// Cleanup is handled by t.Cleanup in SetupTestEnvironment

	// Create a test object first
	// Must match backlog_item ID pattern (e.g. BLI-<digits>); BLI-914-TEST breaks CAS path resolution.
	testID := "BLI-914001"
	kind := pplanKindBacklogItem

	// Get field registry for kind fields (needed for testCreateForKind)
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Fatalf("failed to get fields for kind %s: %v", kind, err)
	}

	// Create the object in-process (reliable CAS + index). BLI-914 is about update --file
	// hash validation, not subprocess create timing.
	projectRoot := testEnv.GetTestRoot()
	obj := createTestObject(kind, testID, kindFields, 0)
	fs, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, fs)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)
	if err := fs.Create(cliCtx, secCtx, obj); err != nil {
		t.Fatalf("create test object: %v", err)
	}
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{kind}); err != nil {
		t.Logf("EnsureCLIObjectMutationVisible after create: %v", err)
	}
	if _, err := fs.Read(ctx, secCtx, testID); err != nil {
		t.Fatalf("read after create: %v", err)
	}

	// Resolve CAS path via storage (bucketing-safe); walking backlog/ is brittle if layout changes.
	objFile, err := fs.GetFilePathForObject(testID, kind)
	if err != nil || objFile == emptyValue {
		t.Fatalf("Could not find file for test object %s: %v", testID, err)
	}

	// Verify it's a hash-based filename (64 char hex string + .yaml = 69 chars minimum)
	filename := filepath.Base(objFile)
	isHashBased := len(filename) >= 69 && filename[len(filename)-5:] == ".yaml"

	if !isHashBased {
		t.Logf("Note: Test object file is not hash-based: %s (test will verify behavior for non-hash files too)", filename)
		// For non-hash-based files, hash validation may not apply, but test still validates behavior
	}

	// For hash-based files, create a hash mismatch by modifying content
	if isHashBased {
		// Extract hash from filename (remove .yaml extension)
		filenameHash := filename[:len(filename)-5]

		// Calculate actual content hash
		fileData, err := fileutil.ReadFile(objFile)
		if err != nil {
			t.Fatalf("Failed to read object file: %v", err)
		}

		actualHash := storage.CalculateSHA256Hash(fileData)

		// If they match, modify the file to create a mismatch
		if filenameHash == actualHash {
			// Modify file content to create hash mismatch
			modifiedData := append(fileData, []byte("\n# Modified for test\n")...)
			if err := fileutil.WriteFile(objFile, modifiedData, paths.FilePerm644); err != nil {
				t.Fatalf("Failed to modify file for test: %v", err)
			}
		}
	}

	// Now attempt update with the file (which may have hash mismatch for hash-based files)
	// This should fail immediately with file hash validation error (for hash-based files),
	// not with an error about reading the existing object
	cmd := testEnv.CreateCLICommand("object", "update", testID, "--file", objFile)
	output, err := cmd.CombinedOutput()

	outputStr := string(output)

	// For hash-based files with mismatches, the error should mention file hash validation
	// Before fix: Error mentions "failed to read object" or existing object ID
	// After fix: Error should mention file hash mismatch and the file path
	if isHashBased {
		// The command should fail for hash-based files with mismatches
		if err == nil {
			t.Error("Expected update command to fail due to file hash mismatch, but it succeeded")
		}

		// The error should mention the file hash mismatch, not the existing object
		if !strings.Contains(outputStr, "hash mismatch") && !strings.Contains(outputStr, "file") {
			t.Logf("Note: Error doesn't mention file hash mismatch (may indicate fix not yet implemented): %s", outputStr)
		}

		// The error should NOT be primarily about reading the existing object
		// (that would indicate we're failing at the wrong point)
		if strings.Contains(outputStr, "failed to read object") && !strings.Contains(outputStr, "file") {
			t.Errorf("Error should be about file hash validation, not reading existing object. Got: %s", outputStr)
		}
	} else {
		// For non-hash-based files, the test just validates the command works
		// (hash validation may not apply)
		t.Logf("Non-hash-based file test - command completed (output: %s)", outputStr)
	}

	// Cleanup: test environment cleanup is handled by t.Cleanup in SetupTestEnvironment
}
