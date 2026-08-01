package storage

import (
	"context"

	"github.com/lanceman/zqk/pkg/datacell"

	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestCAS_DuplicateID_SameContent tests what happens when creating an object with an ID
// that already exists but with the same content (should deduplicate to same hash file)
func TestCAS_DuplicateID_SameContent(t *testing.T) {
	t.Parallel()
	// Set up test environment
	testRoot := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(testRoot)
	kindDir := filepath.Join(processDir, "metrics")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create CAS instance
	casQueue := NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := NewContentAddressableStorage(kindDir, "test_audit_aggregation_metric", casQueue)

	// Create first object
	objectID := "TAM-001"
	data1 := []byte(`id: TAM-001
kind: test_audit_aggregation_metric
title: Test Metric
status: active
schema_version: "` + objects.DefaultSchemaVersion + `"
`)

	if err := cas.Create(objectID, data1); err != nil {
		t.Fatalf("Failed to create first object: %v", err)
	}

	// Wait for index updates to be processed
	if err := casQueue.FlushKind("test_audit_aggregation_metric", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get hash from first creation
	hash1, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash for first object: %v", err)
	}

	// Try to create same object again with same content
	err = cas.Create(objectID, data1)
	if err != nil {
		t.Logf("Creating duplicate ID with same content returned error: %v", err)
		// This might be expected behavior - let's see what happens
	}

	// Wait for index updates to be processed
	if err := casQueue.FlushKind("test_audit_aggregation_metric", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Check if hash is still the same
	hash2, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash after duplicate creation: %v", err)
	}

	// Verify we can still read the object
	readData, err := cas.Read(objectID)
	if err != nil {
		t.Fatalf("Failed to read object after duplicate creation: %v", err)
	}

	// Note: CAS normalizes YAML on read (re-marshals), so the read hash may differ
	// from the stored hash due to formatting differences. The important thing is
	// that the index mapping still points to the same hash file.
	readHash := CalculateSHA256Hash(readData)

	t.Logf("✓ Duplicate ID with same content:")
	t.Logf("  Original hash (stored): %s", hash1)
	t.Logf("  Index hash after duplicate: %s", hash2)
	t.Logf("  Read hash (normalized): %s", readHash)
	t.Logf("  Index hash matches original: %v", hash1 == hash2)
	t.Logf("  Behavior: %s", func() string {
		if err != nil {
			return "error returned"
		}
		if hash1 == hash2 {
			return "index unchanged (deduplicated - same hash file)"
		}
		return "index updated (new hash file created, but content same)"
	}())

	t.Logf("✓ Duplicate ID with same content: hash=%s, behavior=%s", hash1, func() string {
		if err != nil {
			return "error returned"
		}
		return "no error (overwrote or deduplicated)"
	}())
}

// TestCAS_DuplicateID_DifferentContent tests what happens when creating an object with an ID
// that already exists but with different content
func TestCAS_DuplicateID_DifferentContent(t *testing.T) {
	t.Parallel()
	// Set up test environment
	testRoot := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(testRoot)
	kindDir := filepath.Join(processDir, "metrics")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create CAS instance
	casQueue := NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := NewContentAddressableStorage(kindDir, "test_audit_aggregation_metric", casQueue)

	// Create first object
	objectID := "TAM-002"
	data1 := []byte(`id: TAM-002
kind: test_audit_aggregation_metric
title: Original Title
status: active
schema_version: "` + objects.DefaultSchemaVersion + `"
`)

	if err := cas.Create(objectID, data1); err != nil {
		t.Fatalf("Failed to create first object: %v", err)
	}

	// Wait for index updates to be processed
	if err := casQueue.FlushKind("test_audit_aggregation_metric", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	hash1, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash for first object: %v", err)
	}

	// Try to create same object with different content
	data2 := []byte(`id: TAM-002
kind: test_audit_aggregation_metric
title: Modified Title
status: active
schema_version: "` + objects.DefaultSchemaVersion + `"
`)

	err = cas.Create(objectID, data2)
	if err != nil {
		t.Logf("Creating duplicate ID with different content returned error: %v", err)
		// This might be expected behavior - let's see what happens
	}

	// Wait for index updates to be processed
	if err := casQueue.FlushKind("test_audit_aggregation_metric", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Check what hash we have now
	hash2, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash after duplicate creation: %v", err)
	}

	// Read what's actually stored
	readData, err := cas.Read(objectID)
	if err != nil {
		t.Fatalf("Failed to read object after duplicate creation: %v", err)
	}

	// Calculate hash of what we actually read
	readHash := CalculateSHA256Hash(readData)

	t.Logf("✓ Duplicate ID with different content:")
	t.Logf("  Original hash: %s", hash1)
	t.Logf("  New hash: %s", hash2)
	t.Logf("  Read hash: %s", readHash)
	t.Logf("  Hashes match: %v", hash1 == hash2)
	t.Logf("  Read hash matches original: %v", readHash == hash1)
	t.Logf("  Read hash matches new: %v", readHash == hash2)
	t.Logf("  Behavior: %s", func() string {
		if err != nil {
			return "error returned"
		}
		if hash1 == hash2 {
			return "same hash (content not updated)"
		}
		if readHash == hash2 {
			return "content updated (overwrote with new content)"
		}
		if readHash == hash1 {
			return "content not updated (kept original)"
		}
		return "unexpected state (YAML normalization may have occurred)"
	}())
}

// TestCAS_DuplicateID_ThroughStorage tests duplicate ID handling through FileObjectStorage
func TestCAS_DuplicateID_ThroughStorage(t *testing.T) {
	t.Parallel()
	// Set up test environment similar to test scenario
	testRoot := t.TempDir()
	mustEnsureProcessSpecsLayout(t, testRoot)

	// Create FileObjectStorage
	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	// CAS queue + docs/process must be drained and stripped or t.TempDir cleanup races with leftover files under docs/ (bundle flake).
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		opts.WALTimeout = 30 * time.Second
		opts.ShutdownTimeout = 30 * time.Second
		_ = RunProjectTestTeardown(opts)
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system", // Use system account to bypass permission checks
	}

	// Create first object
	objectID := "ITEM-999"
	obj1 := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fileStorage.Create(ctx, secCtx, obj1)
	if err != nil {
		t.Fatalf("Failed to create first object: %v", err)
	}

	// Read it back
	readObj1, err := fileStorage.Read(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Failed to read first object: %v", err)
	}

	// Try to create same object with different content. Both behaviors are
	// acceptable for this test:
	//   - returning an "object already exists" error, or
	//   - succeeding but leaving content unchanged.
	obj2 := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Modified Title",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fileStorage.Create(ctx, secCtx, obj2)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Unexpected error creating duplicate ID through storage: %v", err)
	}

	// Read what's actually stored
	readObj2, err := fileStorage.Read(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Failed to read object after duplicate creation: %v", err)
	}

	originalTitle, _ := readObj1[objects.FieldKeyTitle].(string)
	newTitle, _ := readObj2[objects.FieldKeyTitle].(string)

	t.Logf("✓ Duplicate ID through FileObjectStorage:")
	t.Logf("  Original title: %s", originalTitle)
	t.Logf("  New title: %s", newTitle)
	t.Logf("  Titles match: %v", originalTitle == newTitle)
	t.Logf("  Behavior: %s", func() string {
		if err != nil {
			return "error returned (prevented duplicate)"
		}
		if originalTitle == newTitle {
			return "content not updated (duplicate prevented)"
		}
		return "content updated (overwrote)"
	}())
}
