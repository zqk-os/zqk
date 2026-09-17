package cas_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestCAS_CheckCommandOutput tests that check command correctly handles CAS vs non-CAS objects
func TestCAS_CheckCommandOutput(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-check-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create a CAS object
	casObj := map[string]any{
		objects.FieldKeyID:            "BLI-600",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Check Test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, casObj, "")

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify object is in CAS
	cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	hash, err := cas.GetHashForID("BLI-600")
	if err != nil {
		t.Fatalf("Object should exist in CAS index: %v", err)
	}

	// Get file path using getObjectFilePath (what check command would use)
	filePath, err := fileStorage.GetObjectFilePath("BLI-600", "backlog_item")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Verify file path is hash-based (CAS format)
	fileName := filepath.Base(filePath)
	if len(fileName) < 68 || !strings.HasSuffix(fileName, ".yaml") {
		t.Errorf("Expected hash-based filename for CAS object, got %s", fileName)
	}

	// Verify filename matches hash
	expectedHashFile := hash + ".yaml"
	if fileName != expectedHashFile {
		t.Errorf("Expected filename %s, got %s", expectedHashFile, fileName)
	}

	// Verify file exists at that path
	if _, err := fileutil.Stat(filePath); err != nil {
		t.Errorf("CAS file should exist at %s: %v", filePath, err)
	}

	// Test that Read still works (check command reads objects)
	readObj, err := fileStorage.Read(ctx, secCtx, "BLI-600")
	if err != nil {
		t.Fatalf("Failed to read CAS object: %v", err)
	}

	if readObj[objects.FieldKeyID] != "BLI-600" {
		t.Errorf("Expected ID BLI-600, got %v", readObj[objects.FieldKeyID])
	}

	// Test that Exists works correctly
	exists, err := fileStorage.Exists(ctx, secCtx, "BLI-600")
	if err != nil {
		t.Fatalf("Exists() should not return error: %v", err)
	}
	if !exists {
		t.Errorf("Exists() should return true for CAS object")
	}
}

// TestCAS_CheckCommandDistinguishesCAS tests that check command can distinguish CAS vs non-CAS files
func TestCAS_CheckCommandDistinguishesCAS(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS.
	// Use a dedicated scenario under the global test root so that we don't
	// fight with TempDir cleanup semantics.
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-distinguish-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Create a CAS object
	casObj := map[string]any{
		objects.FieldKeyID:            "BLI-700",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Object",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, casObj, "")

	// Flush CAS index so getObjectFilePath sees hash-based path (avoids flakiness from async index updates)
	if err := storage.FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Get file path
	casFilePath, err := fileStorage.GetObjectFilePath("BLI-700", "backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS file path: %v", err)
	}

	// Verify it's a hash-based filename (CAS format: 64 hex chars + ".yaml")
	casFileName := filepath.Base(casFilePath)
	isCASFile := len(casFileName) >= 69 && strings.HasSuffix(casFileName, ".yaml") && len(casFileName) >= 64 && isHexString(casFileName[:64])
	if !isCASFile {
		t.Errorf("Expected CAS file (hash-based filename), got %s (len=%d)", casFileName, len(casFileName))
	}

	// Verify we can detect CAS vs non-CAS
	// CAS files have 64-char hex filenames
	// Non-CAS files have ID-based filenames (e.g., "BLI-700.yaml")
	if strings.HasPrefix(casFileName, "BLI-") {
		t.Errorf("CAS file should not have ID-based filename, got %s", casFileName)
	}

	// Verify the file path is correct (hash-based)
	if !filepath.IsAbs(casFilePath) {
		t.Errorf("File path should be absolute, got %s", casFilePath)
	}
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
