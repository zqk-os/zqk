package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"context"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestAuditIDGenerator_CASAware tests that audit ID generator correctly finds existing IDs from CAS
// This verifies the CAS-aware ID generation works correctly with bucketed storage
func TestAuditIDGenerator_CASAware(t *testing.T) {
	disableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-id-gen-test")
	processDir := datacell.ProcessPrimaryDir(testRoot)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

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

	// Verify CAS is enabled for audit_event
	if !fileStorage.usesContentAddressableStorage("audit_event") {
		t.Fatalf("CAS should be enabled for audit_event when path contains 'test-scenarios'")
	}

	// Create audit events in a specific month bucket using CAS (hash-based filenames)
	now := time.Now().UTC()
	month := now.Format("2006-01")
	auditDir := filepath.Join(processDir, "audit", month)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	// Create 3 audit events directly using CAS (simulating existing events)
	// These will have hash-based filenames, not AUD-*.yaml
	baseAuditDir := filepath.Join(processDir, "audit")
	for i := 1; i <= 3; i++ {
		eventID := "AUD-001" // All will have same ID initially, but CAS will handle it
		if i > 1 {
			// For subsequent events, we need unique IDs
			eventID = "AUD-002"
			if i > 2 {
				eventID = "AUD-003"
			}
		}

		event := map[string]any{
			objects.FieldKeyID:            eventID,
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     now.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyUpdatedAt:     now.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject: validation.DefaultOriginProject,
			objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
			objects.FieldKeyEventType:     "test_event",
			objects.FieldKeyOperation:     "Test operation",
			objects.FieldKeySeverity:      "low",
		}

		data, err := FormatMultiLineYAML(event)
		if err != nil {
			t.Fatalf("Failed to format audit event %s: %v", eventID, err)
		}

		// Write using WriteSystemObjectAndRegisterHash which handles CAS
		// This creates hash-based filenames
		auditFilePath := filepath.Join(auditDir, eventID+".yaml")
		err = WriteSystemObjectAndRegisterHash(auditFilePath, data, "audit_event", baseAuditDir, eventID, fileStorage)
		if err != nil {
			t.Fatalf("Failed to create audit event %s: %v", eventID, err)
		}
	}

	// Wait for CAS index updates to be processed
	writeQueue := caspkg.GetGlobalListingIndexWriteQueue()
	if err := writeQueue.FlushKind("audit_event", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify events are in CAS index
	cas, err := fileStorage.getContentAddressableStorage("audit_event")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	allIDs, err := cas.ListIDs()
	if err != nil {
		t.Fatalf("Failed to list CAS IDs: %v", err)
	}

	// Verify we have at least 3 IDs in CAS
	auditIDCount := 0
	for _, id := range allIDs {
		if len(id) >= 4 && id[:4] == "AUD-" {
			auditIDCount++
		}
	}

	if auditIDCount < 3 {
		t.Errorf("Expected at least 3 audit IDs in CAS, got %d", auditIDCount)
	}

	// Now test that the ID generator correctly finds these IDs from CAS
	// and generates the next ID (should be AUD-004, not AUD-001)
	generator := GetAuditIDGenerator(pkgctx.NewSystemContext(), auditDir, fileStorage)

	// Reset generator to force a rescan
	generator.Reset()

	// Generate next ID - should be AUD-004 (since AUD-001, AUD-002, AUD-003 exist)
	nextID, err := generator.GenerateNextID()
	if err != nil {
		t.Fatalf("Failed to generate next ID: %v", err)
	}

	// Verify it's not AUD-001 (which already exists)
	if nextID == "AUD-001" {
		t.Errorf("Generated ID should not be AUD-001 (already exists), got %s", nextID)
	}

	// Verify the sequence is correct (should be at least 4, could be higher if other IDs exist)
	lastSeq := generator.GetLastSequence()
	if lastSeq < 3 {
		t.Errorf("Expected last sequence to be at least 3 (found AUD-001, AUD-002, AUD-003), got %d", lastSeq)
	}

	// The key test: verify that the generator found existing IDs from CAS
	// and is generating the next sequential ID (not reusing AUD-001)
	t.Logf("Generated next ID: %s (last sequence: %d)", nextID, lastSeq)

	// Verify the ID is in the expected format
	if len(nextID) < 4 || nextID[:4] != "AUD-" {
		t.Errorf("Generated ID should start with 'AUD-', got %s", nextID)
	}

	// Most importantly: verify it's not one of the existing IDs
	existingIDs := map[string]bool{"AUD-001": true, "AUD-002": true, "AUD-003": true}
	if existingIDs[nextID] {
		t.Errorf("Generated ID %s should not be one of the existing IDs (AUD-001, AUD-002, AUD-003)", nextID)
	}
}
