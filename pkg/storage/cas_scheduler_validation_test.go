package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestCAS_SchedulerAuditEvents tests that scheduler audit events work with CAS
func TestCAS_SchedulerAuditEvents(t *testing.T) {
	disableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-scheduler-test")
	mustEnsureProcessSpecsLayout(t, testRoot)
	processDir := datacell.ProcessPrimaryDir(testRoot)

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

	// Verify CAS is enabled for audit_event when path contains "test-scenarios"
	if !fileStorage.usesContentAddressableStorage("audit_event") {
		t.Fatalf("CAS should be enabled for audit_event when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	// Create an audit event (simulating scheduler job execution)
	auditEvent := map[string]any{
		objects.FieldKeyID:            "AUD-001",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2030-01-05T10:00:00Z",
		objects.FieldKeyCreatedBy:     "account:system",
		objects.FieldKeyUpdatedAt:     "2030-01-05T10:00:00Z",
		objects.FieldKeyUpdatedBy:     "account:system",
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyStatus:        "completed",
		objects.FieldKeyEventType:     "job_execution",
		objects.FieldKeyOperation:     "Test scheduler job execution",
		objects.FieldKeyTargetKind:    "scheduler_job",
		objects.FieldKeyTargetID:      "SCH-001",
		objects.FieldKeySeverity:      "low",
		objects.FieldKeyMetadata: map[string]any{
			objects.FieldKeySource: "scheduler",
			"job_id":               "SCH-001",
		},
	}

	// Use WriteSystemObjectAndRegisterHash (what scheduler uses)
	// First, format the audit event to YAML
	data, err := FormatMultiLineYAML(auditEvent)
	if err != nil {
		t.Fatalf("Failed to format audit event: %v", err)
	}

	// Determine audit directory (bucketed by month)
	month := "2030-01" // Use a fixed month for testing
	auditDir := filepath.Join(processDir, "audit", month)
	if err := os.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	auditFilePath := filepath.Join(auditDir, "AUD-001.yaml")
	err = WriteSystemObjectAndRegisterHash(auditFilePath, data, "audit_event", auditDir, "AUD-001", fileStorage)
	if err != nil {
		t.Fatalf("Failed to create audit event: %v", err)
	}

	// Verify object exists in CAS
	cas, err := fileStorage.getContentAddressableStorage("audit_event")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	hash, err := cas.GetHashForID("AUD-001")
	if err != nil {
		t.Fatalf("Audit event should exist in CAS index: %v", err)
	}

	if hash == emptyValue {
		t.Errorf("Hash should not be empty")
	}

	// Verify we can read it back
	readObj, err := fileStorage.Read(ctx, secCtx, "AUD-001")
	if err != nil {
		t.Fatalf("Failed to read audit event: %v", err)
	}

	if readObj[objects.FieldKeyID] != "AUD-001" {
		t.Errorf("Expected ID AUD-001, got %v", readObj[objects.FieldKeyID])
	}

	// Verify getObjectFilePath returns the correct hash-based path
	filePath, err := fileStorage.getObjectFilePath("AUD-001", "audit_event")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// For CAS objects, the path should be the hash-based file
	expectedHashFile := filepath.Join(processDir, "audit", hash+".yaml")
	if filePath != expectedHashFile {
		t.Errorf("Expected hash-based file path %s, got %s", expectedHashFile, filePath)
	}

	// Verify the file actually exists at that path
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("Hash-based file should exist at %s: %v", filePath, err)
	}
}

// TestCAS_ValidationCacheFilePath tests that validation cache gets correct file paths for CAS objects
func TestCAS_ValidationCacheFilePath(t *testing.T) {
	// Do not use: CAS index queue + WAL activity must finish before t.TempDir cleanup.
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-validation-test")
	mustEnsureProcessSpecsLayout(t, testRoot)

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		q := GetListingIndexWriteQueueForProjectRoot(testRoot)
		if q != nil {
			_ = q.FlushAll(5 * time.Second)
			_ = q.Shutdown()
		}
		_ = FlushAllListingIndexesForProjectRoot(testRoot)
		_ = WaitForWALProcessing(testRoot, 15*time.Second)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = fileStorage.Shutdown(shutdownCtx)
	})

	// Verify CAS is enabled for backlog_item
	if !fileStorage.usesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	// Create an object
	obj := map[string]any{
		objects.FieldKeyID:            "ITEM-100",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Validation Cache Test",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	err = fileStorage.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Get the file path using getObjectFilePath (what validation uses)
	filePath, err := fileStorage.getObjectFilePath("ITEM-100", "backlog_item")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// For CAS objects, this should return the hash-based path
	// Verify it's a hash-based filename (64-char hex)
	fileName := filepath.Base(filePath)
	ext := filepath.Ext(fileName)
	if len(fileName) < 68 || ext != ".yaml" { // 64 chars hash + .yaml = 69 chars minimum
		t.Errorf("Expected hash-based filename (64+ chars with .yaml extension), got %s (len=%d, ext=%s)", fileName, len(fileName), ext)
	}

	// Verify the file exists and can be read
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("File should exist at %s: %v", filePath, err)
	}

	// Verify we can read the file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("Failed to read file: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("File should not be empty")
	}
}
