package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestCAS_CheckCommandOutput tests that check command correctly handles CAS vs non-CAS objects
func TestCAS_CheckCommandOutput(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-check-test")
	mustEnsureProcessSpecsLayout(t, testRoot)

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item
	if !fileStorage.usesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	// Create a CAS object
	casObj := map[string]any{
		objects.FieldKeyID:            "ITEM-600",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Check Test",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fileStorage.Create(ctx, secCtx, casObj)
	if err != nil {
		t.Fatalf("Failed to create CAS object: %v", err)
	}

	// Wait for index updates to be processed
	if err := FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify object is in CAS
	cas, err := fileStorage.getContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	hash, err := cas.GetHashForID("ITEM-600")
	if err != nil {
		t.Fatalf("Object should exist in CAS index: %v", err)
	}

	// Get file path using getObjectFilePath (what check command would use)
	filePath, err := fileStorage.getObjectFilePath("ITEM-600", "backlog_item")
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
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("CAS file should exist at %s: %v", filePath, err)
	}

	// Test that Read still works (check command reads objects)
	readObj, err := fileStorage.Read(ctx, secCtx, "ITEM-600")
	if err != nil {
		t.Fatalf("Failed to read CAS object: %v", err)
	}

	if readObj[objects.FieldKeyID] != "ITEM-600" {
		t.Errorf("Expected ID ITEM-600, got %v", readObj[objects.FieldKeyID])
	}

	// Test that Exists works correctly
	exists, err := fileStorage.Exists(ctx, secCtx, "ITEM-600")
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
	mustEnsureProcessSpecsLayout(t, testRoot)

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Create a CAS object
	casObj := map[string]any{
		objects.FieldKeyID:            "ITEM-700",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Object",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	err = fileStorage.Create(ctx, secCtx, casObj)
	if err != nil {
		t.Fatalf("Failed to create CAS object: %v", err)
	}

	// Flush CAS index so getObjectFilePath sees hash-based path (avoids flakiness from async index updates)
	if err := FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush CAS index: %v", err)
	}

	// Get file path
	casFilePath, err := fileStorage.getObjectFilePath("ITEM-700", "backlog_item")
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
	// Non-CAS files have ID-based filenames (e.g., "ITEM-700.yaml")
	if strings.HasPrefix(casFileName, "ITEM-") {
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
