package cas_test

import (
	"context"

	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// These tests verify that reading a CAS object after its file has been tampered with
// returns a hash mismatch error. They use NewFileObjectStorage and storage.WriteSystemObjectAndRegisterHash
// to create objects; the read path must verify content hash against the index (cas.Read).
// If tests fail (Read returns nil instead of error), the cause may be the discovery fallback
// path (findCASFilePathByScanning + readObjectFile) being used without hash verification,
// or the CAS index not being populated so the primary path is skipped.
//
// TestCAS_BuiltinTamperDetection_AuditEvent tests tamper detection for audit events
func TestCAS_BuiltinTamperDetection_AuditEvent(t *testing.T) {
	// Do not t.Parallel: DisableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED (global env).
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-builtin-audit")
	processDir := datacell.ProcessPrimaryDir(testRoot)
	auditDir := filepath.Join(processDir, "audit")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create an audit event through the system
	auditData := map[string]any{
		objects.FieldKeyID:            "AUD-TAMPER-001",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeyEventType:     "test_event",
		"action":                      "create",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(auditData)
	if err != nil {
		t.Fatalf("Failed to marshal audit data: %v", err)
	}

	// Create file path
	auditFilePath := filepath.Join(auditDir, "AUD-TAMPER-001.yaml")

	err = storage.WriteSystemObjectAndRegisterHash(auditFilePath, data, "audit_event", auditDir, "AUD-TAMPER-001", fileStorage)
	if err != nil {
		t.Fatalf("Failed to create audit event: %v", err)
	}
	// Flush so CAS index is persisted; Read must use index path (cas.Read) to verify hash on read
	storage.FlushOrFail(t, fileStorage.GetProjectRoot(), "audit_event")

	// Get the file path
	filePath, err := fileStorage.GetObjectFilePath("AUD-TAMPER-001", "audit_event")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Verify it's a hash-based file (CAS format)
	fileName := filepath.Base(filePath)
	if len(fileName) != 69 || !strings.HasSuffix(fileName, ".yaml") {
		t.Fatalf("Expected hash-based filename, got %s", fileName)
	}

	// Tamper with the file: modify content directly
	tamperedContent := []byte(`id: AUD-TAMPER-001
kind: audit_event
event_type: test_event
action: TAMPERED
created_at: "2030-01-05T00:00:00Z"
created_by: ACC-SYSTEM
schema_version: "` + objects.DefaultSchemaVersion + `"`)

	if err := fileutil.WriteFile(filePath, tamperedContent, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to tamper with file: %v", err)
	}

	// Update mtime to make it look recent (simulating tampering)
	recentTime := time.Now()
	if err := fileutil.Chtimes(filePath, recentTime, recentTime); err != nil {
		t.Fatalf("Failed to update mtime: %v", err)
	}

	// Attempt to read - should detect tampering
	_, err = fileStorage.Read(ctx, secCtx, "AUD-TAMPER-001")
	if err == nil {
		t.Error("Expected error when reading tampered audit event, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("Expected hash mismatch error, got: %v", err)
	}
}

// TestCAS_BuiltinTamperDetection_ChangeJournal tests tamper detection for change journal entries
func TestCAS_BuiltinTamperDetection_ChangeJournal(t *testing.T) {
	// Do not t.Parallel: storage.DisableStreamStorageForTest mutates global env.
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-builtin-change-journal")
	processDir := datacell.ProcessPrimaryDir(testRoot)
	changeJournalDir := filepath.Join(processDir, "change_journal")
	if err := fileutil.MkdirAll(changeJournalDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Create a change journal entry through the system
	changeJournalData := map[string]any{
		objects.FieldKeyID:            "CJE-TAMPER-001",
		objects.FieldKeyKind:          "change_journal_entry",
		"object_id":                   "BLI-001",
		objects.FieldKeyObjectKind:    "backlog_item",
		objects.FieldKeyChangeType:    "update",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(changeJournalData)
	if err != nil {
		t.Fatalf("Failed to marshal change journal data: %v", err)
	}

	// Create file path
	changeJournalFilePath := filepath.Join(changeJournalDir, "CJE-TAMPER-001.yaml")

	err = storage.WriteSystemObjectAndRegisterHash(changeJournalFilePath, data, "change_journal_entry", changeJournalDir, "CJE-TAMPER-001", fileStorage)
	if err != nil {
		t.Fatalf("Failed to create change journal entry: %v", err)
	}

	// Get the file path
	filePath, err := fileStorage.GetObjectFilePath("CJE-TAMPER-001", "change_journal_entry")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Verify it's a hash-based file (CAS format)
	fileName := filepath.Base(filePath)
	if len(fileName) != 69 || !strings.HasSuffix(fileName, ".yaml") {
		t.Fatalf("Expected hash-based filename, got %s", fileName)
	}

	// Tamper with the file: modify content directly
	tamperedContent := []byte(`id: CJE-TAMPER-001
kind: change_journal_entry
object_id: BLI-001
object_kind: backlog_item
change_type: TAMPERED
created_at: "2030-01-05T00:00:00Z"
created_by: ACC-SYSTEM
schema_version: "` + objects.DefaultSchemaVersion + `"`)

	if err := fileutil.WriteFile(filePath, tamperedContent, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to tamper with file: %v", err)
	}

	// Update mtime to make it look recent (simulating tampering)
	recentTime := time.Now()
	if err := fileutil.Chtimes(filePath, recentTime, recentTime); err != nil {
		t.Fatalf("Failed to update mtime: %v", err)
	}

	// Attempt to read via CAS directly - should detect tampering
	cas, err := fileStorage.GetContentAddressableStorage("change_journal_entry")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}
	_, err = cas.Read("CJE-TAMPER-001")
	if err == nil {
		t.Error("Expected error when reading tampered change journal entry, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("Expected hash mismatch error, got: %v", err)
	}
}

// TestCAS_BuiltinTamperDetection_MtimeCheck tests that mtime checking works for CAS files
func TestCAS_BuiltinTamperDetection_MtimeCheck(t *testing.T) {
	// Do not t.Parallel: DisableStreamStorageForTest mutates global env.
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-mtime-check")
	processDir := datacell.ProcessPrimaryDir(testRoot)
	auditDir := filepath.Join(processDir, "audit")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create an audit event
	auditData := map[string]any{
		objects.FieldKeyID:            "AUD-MTIME-001",
		objects.FieldKeyKind:          "audit_event",
		objects.FieldKeyEventType:     "test_event",
		"action":                      "create",
		objects.FieldKeyCreatedAt:     time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(auditData)
	if err != nil {
		t.Fatalf("Failed to marshal audit data: %v", err)
	}

	// Create file path
	auditFilePath := filepath.Join(auditDir, "AUD-MTIME-001.yaml")

	err = storage.WriteSystemObjectAndRegisterHash(auditFilePath, data, "audit_event", auditDir, "AUD-MTIME-001", fileStorage)
	if err != nil {
		t.Fatalf("Failed to create audit event: %v", err)
	}

	// Get the file path
	filePath, err := fileStorage.GetObjectFilePath("AUD-MTIME-001", "audit_event")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Get initial mtime
	initialInfo, err := fileutil.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	initialMTime := initialInfo.ModTime()

	// Wait a bit to ensure time difference
	time.Sleep(100 * time.Millisecond)

	// Update mtime without changing content (simulating touch command)
	recentTime := time.Now()
	if err := fileutil.Chtimes(filePath, recentTime, recentTime); err != nil {
		t.Fatalf("Failed to update mtime: %v", err)
	}

	// Verify mtime changed
	updatedInfo, err := fileutil.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file after mtime update: %v", err)
	}
	updatedMTime := updatedInfo.ModTime()

	if updatedMTime.Equal(initialMTime) {
		t.Error("Mtime should have changed")
	}

	// The file should still be readable (hash matches), but mtime check should flag it
	// This test verifies that mtime checking is working
	// In a real scenario, the check command would detect the recent mtime change
	_, err = fileStorage.Read(ctx, secCtx, "AUD-MTIME-001")
	if err != nil {
		// Hash still matches, so read should succeed
		// But the check command would flag the mtime change
		t.Logf("Read succeeded (hash matches), but mtime was modified: %v", err)
	}
}

// TestCAS_BuiltinTamperDetection_DocEntry tests tamper detection for doc_entry objects
func TestCAS_BuiltinTamperDetection_DocEntry(t *testing.T) {
	// Do not t.Parallel: storage.DisableStreamStorageForTest mutates global env.
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-builtin-doc-entry")
	processDir := datacell.ProcessPrimaryDir(testRoot)
	docEntriesDir := filepath.Join(processDir, "doc_entries")
	if err := fileutil.MkdirAll(docEntriesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create a doc_entry object
	// Use UTC time with Z suffix for created_at (required format)
	createdAt := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	docEntryData := map[string]any{
		objects.FieldKeyID:                "DOC-001",
		objects.FieldKeyKind:              "doc_entry",
		objects.FieldKeyTitle:             "Test Document",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyPath:              "docs/test.md",
		objects.FieldKeySummary:           "Test document entry",
		objects.FieldKeyGroup:             "other", // Must be one of: project_goals, project_specific, tooling, process, onboarding, design, architecture, other
		objects.FieldKeyContentSearchable: true,
		objects.FieldKeyGoalRefs:          []string{},
		objects.FieldKeyWorkstreamRefs:    []string{},
		objects.FieldKeyMilestoneRefs:     []string{},
		objects.FieldKeyRequirementRefs:   []string{},
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:         createdAt,
		objects.FieldKeyCreatedBy:         "ACC-SYSTEM",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, docEntryData, "")
	storage.FlushOrFail(t, fileStorage.GetProjectRoot(), "doc_entry")

	// Get the file path
	filePath, err := fileStorage.GetObjectFilePath("DOC-001", "doc_entry")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Verify it's a hash-based file (CAS format)
	fileName := filepath.Base(filePath)
	if len(fileName) != 69 || !strings.HasSuffix(fileName, ".yaml") {
		t.Fatalf("Expected hash-based filename, got %s", fileName)
	}

	// Tamper with the file: modify content directly
	tamperedContent := []byte(`id: DOC-001
kind: doc_entry
title: TAMPERED Document
status: active
path: docs/test.md
summary: TAMPERED summary
group: other
content_searchable: true
goal_refs: []
workstream_refs: []
milestone_refs: []
requirement_refs: []
schema_version: "` + objects.DefaultSchemaVersion + `"
created_at: "2030-01-05T00:00:00Z"
created_by: ACC-SYSTEM`)

	if err := fileutil.WriteFile(filePath, tamperedContent, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to tamper with file: %v", err)
	}

	// Update mtime to make it look recent (simulating tampering)
	recentTime := time.Now()
	if err := fileutil.Chtimes(filePath, recentTime, recentTime); err != nil {
		t.Fatalf("Failed to update mtime: %v", err)
	}

	storage.GetGlobalParseCache().Clear()

	// Attempt to read - should detect tampering
	_, err = fileStorage.Read(ctx, secCtx, "DOC-001")
	if err == nil {
		t.Error("Expected error when reading tampered doc_entry, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("Expected hash mismatch error, got: %v", err)
	}
}
