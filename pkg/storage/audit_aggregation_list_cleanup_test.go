package storage

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestListObjectsFromPaths verifies that listObjectsFromPaths reads YAML files, applies filters, and returns matching objects.
func TestListObjectsFromPaths(t *testing.T) {
	dir := t.TempDir()
	if err := paths.EnsureProcessAndObjectSpecsLayout(dir); err != nil {
		t.Fatalf("test layout: %v", err)
	}
	f, err := NewFileObjectStorageForTest(dir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = f.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(dir, f)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Write three audit_event YAML files (ID-named for test simplicity)
	base := time.Date(2030, 2, 15, 10, 0, 0, 0, time.UTC)
	files := []struct {
		name      string
		createdAt string
	}{
		{"a.yaml", base.Add(-2 * time.Hour).Format(time.RFC3339)},
		{"b.yaml", base.Add(-1 * time.Hour).Format(time.RFC3339)},
		{"c.yaml", base.Add(1 * time.Hour).Format(time.RFC3339)},
	}
	var filePaths []string
	for _, ff := range files {
		obj := map[string]any{
			objects.FieldKeyID:        "AUD-TEST-" + ff.name,
			objects.FieldKeyKind:      "audit_event",
			objects.FieldKeyCreatedAt: ff.createdAt,
			objects.FieldKeyStatus:    objects.ObjectStatusCompleted,
		}
		data, err := yaml.Marshal(obj)
		if err != nil {
			t.Fatalf("yaml.Marshal: %v", err)
		}
		path := filepath.Join(dir, ff.name)
		if err := fileutil.WriteFile(path, data, paths.FilePerm644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		filePaths = append(filePaths, path)
	}

	ctx := context.Background()

	// No filter: all three
	got, err := f.listObjectsFromPaths(ctx, filePaths, "audit_event", nil)
	if err != nil {
		t.Fatalf("listObjectsFromPaths(no filter): %v", err)
	}
	if len(got) != 3 {
		t.Errorf("listObjectsFromPaths(no filter) len = %d, want 3", len(got))
	}

	// created_at in window that includes only two (base-2h to base)
	windowStart := base.Add(-2 * time.Hour)
	windowEnd := base
	filters := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{
			"$gte": windowStart.Format(time.RFC3339),
			"$lte": windowEnd.Format(time.RFC3339),
		},
	}
	got, err = f.listObjectsFromPaths(ctx, filePaths, "audit_event", filters)
	if err != nil {
		t.Fatalf("listObjectsFromPaths(with filter): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("listObjectsFromPaths(created_at window) len = %d, want 2 (only a and b in window)", len(got))
	}
}

// TestList_audit_event_created_at_range_returns_events_via_date_range_walk verifies that when the
// high-volume cache is not populated (or returns 0), List with audit_event and created_at range
// uses the date-range walk and returns events from bucketed audit dirs.
func TestList_audit_event_created_at_range_returns_events_via_date_range_walk(t *testing.T) {
	// Do not t.Parallel: disableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED.
	disableStreamStorageForTest(t)
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "audit-list-date-walk")
	mustEnsureProcessSpecsLayout(t, testRoot)
	processDir := datacell.ProcessPrimaryDir(testRoot)

	f, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = f.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, f)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	if !f.usesContentAddressableStorage("audit_event") {
		t.Fatal("CAS required for audit_event in test-scenarios path")
	}

	// Create two audit events in a bucket (month) within the window
	windowStart := time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2030, 2, 28, 23, 59, 59, 0, time.UTC)
	month := "2030-02"
	baseAuditDir := filepath.Join(processDir, "audit")
	auditDir := filepath.Join(baseAuditDir, month)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll audit: %v", err)
	}

	events := []map[string]any{
		{
			objects.FieldKeyID: "AUD-DW-001", objects.FieldKeyKind: "audit_event", objects.FieldKeyCreatedAt: "2030-02-10T12:00:00Z",
			objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyEventType: "object_creation", objects.FieldKeyOperation: "op1",
			objects.FieldKeyCreatedBy: "ACC-SYSTEM", objects.FieldKeyUpdatedAt: "2030-02-10T12:00:00Z", objects.FieldKeyUpdatedBy: "ACC-SYSTEM",
			objects.FieldKeyOriginSystem: validation.DefaultOriginSystem, objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		},
		{
			objects.FieldKeyID: "AUD-DW-002", objects.FieldKeyKind: "audit_event", objects.FieldKeyCreatedAt: "2030-02-11T12:00:00Z",
			objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyEventType: "object_creation", objects.FieldKeyOperation: "op2",
			objects.FieldKeyCreatedBy: "ACC-SYSTEM", objects.FieldKeyUpdatedAt: "2030-02-11T12:00:00Z", objects.FieldKeyUpdatedBy: "ACC-SYSTEM",
			objects.FieldKeyOriginSystem: validation.DefaultOriginSystem, objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		},
	}
	for _, ev := range events {
		data, err := FormatMultiLineYAML(ev)
		if err != nil {
			t.Fatalf("FormatMultiLineYAML: %v", err)
		}
		err = WriteSystemObjectAndRegisterHash(
			filepath.Join(auditDir, ev[objects.FieldKeyID].(string)+".yaml"),
			data, "audit_event", baseAuditDir, ev[objects.FieldKeyID].(string), f)
		if err != nil {
			t.Fatalf("WriteSystemObjectAndRegisterHash: %v", err)
		}
	}
	if err := FlushListingIndexForProjectRoot(testRoot, "audit_event"); err != nil {
		t.Fatalf("FlushListingIndexForProjectRoot: %v", err)
	}

	// List with created_at range (high-volume cache not populated in test → date-range walk used)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), "/tmp/nonexistent")
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()
	filter := ListFilter{
		Kind: "audit_event",
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$gte": windowStart.Format(time.RFC3339),
				"$lte": windowEnd.Format(time.RFC3339),
			},
		},
		SortBy:  "created_at",
		SortAsc: true,
		Limit:   100,
	}
	result, err := f.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Objects) < 2 {
		t.Errorf("List(audit_event created_at range) returned %d objects, want at least 2 (date-range walk path should find events)", len(result.Objects))
	}

	// Drain per-project CAS queues so t.TempDir cleanup does not race with background index writers.
	FlushAllOrFail(t, f.GetProjectRoot())
}

// TestAggregateAuditEvents_ThenCleanupArchive_UpdatesStatus verifies the full flow: create audit events,
// run aggregation, then cleanup with archive (status=archived). Events remain but are marked archived.
// Delete path is not tested here (BulkDelete can block in test env due to hash registry worker pool).
func TestAggregateAuditEvents_ThenCleanupArchive_UpdatesStatus(t *testing.T) {
	// Do not t.Parallel: disableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED.
	disableStreamStorageForTest(t)
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "aggregate-cleanup-archive")
	mustEnsureProcessSpecsLayout(t, testRoot)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	// BulkUpdate (archive path) needs audit_event spec chain (audit_event -> base_object -> auditable)
	specNames := []string{"auditable.yaml", "base_object.yaml", "audit_event.yaml"}
	copied := false
	for _, tryRoot := range []string{"docs", "../../docs"} {
		root := filepath.Join(tryRoot, "process", "_internal", "object_specs")
		if _, err := fileutil.Stat(root); err != nil {
			continue
		}
		for _, name := range specNames {
			data, err := fileutil.ReadFile(filepath.Join(root, name))
			if err != nil {
				break
			}
			if err := fileutil.WriteFile(filepath.Join(specsDir, name), data, paths.FilePerm644); err != nil {
				t.Fatalf("WriteFile %s: %v", name, err)
			}
		}
		if _, err := fileutil.Stat(filepath.Join(specsDir, "audit_event.yaml")); err == nil {
			copied = true
			break
		}
	}
	if !copied {
		t.Skip("audit_event spec chain not found (run from repo root); BulkUpdate validation requires spec")
	}

	f, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = f.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, f)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	if !f.usesContentAddressableStorage("audit_event") {
		t.Fatal("CAS required for audit_event")
	}

	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), "/tmp/nonexistent")
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	windowStart := time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2030, 2, 28, 23, 59, 59, 0, time.UTC)
	month := "2030-02"
	baseAuditDir := filepath.Join(processDir, "audit")
	auditDir := filepath.Join(baseAuditDir, month)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll audit: %v", err)
	}

	events := []map[string]any{
		{
			objects.FieldKeyID: "AUD-CLEAN-001", objects.FieldKeyKind: "audit_event", objects.FieldKeyCreatedAt: "2030-02-10T12:00:00Z",
			objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyEventType: "object_creation", objects.FieldKeyOperation: "op1",
			objects.FieldKeyCreatedBy: "ACC-SYSTEM", objects.FieldKeyUpdatedAt: "2030-02-10T12:00:00Z", objects.FieldKeyUpdatedBy: "ACC-SYSTEM",
			objects.FieldKeyOriginSystem: validation.DefaultOriginSystem, objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		},
		{
			objects.FieldKeyID: "AUD-CLEAN-002", objects.FieldKeyKind: "audit_event", objects.FieldKeyCreatedAt: "2030-02-11T12:00:00Z",
			objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyEventType: "object_creation", objects.FieldKeyOperation: "op2",
			objects.FieldKeyCreatedBy: "ACC-SYSTEM", objects.FieldKeyUpdatedAt: "2030-02-11T12:00:00Z", objects.FieldKeyUpdatedBy: "ACC-SYSTEM",
			objects.FieldKeyOriginSystem: validation.DefaultOriginSystem, objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		},
	}
	for _, ev := range events {
		data, err := FormatMultiLineYAML(ev)
		if err != nil {
			t.Fatalf("FormatMultiLineYAML: %v", err)
		}
		err = WriteSystemObjectAndRegisterHash(
			filepath.Join(auditDir, ev[objects.FieldKeyID].(string)+".yaml"),
			data, "audit_event", baseAuditDir, ev[objects.FieldKeyID].(string), f)
		if err != nil {
			t.Fatalf("WriteSystemObjectAndRegisterHash: %v", err)
		}
	}
	if err := FlushListingIndexForProjectRoot(testRoot, "audit_event"); err != nil {
		t.Fatalf("FlushListingIndexForProjectRoot: %v", err)
	}

	// Aggregate
	svc := NewAuditAggregationService(f)
	result, err := svc.AggregateAuditEvents(ctx, secCtx, storageCtx, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("AggregateAuditEvents: %v", err)
	}
	t.Logf("AggregateAuditEvents returned %d events processed, %d events updated", len(result.EventsProcessed), result.EventsUpdated)
	if result.EventCount < 2 {
		t.Errorf("EventCount = %d, want at least 2", result.EventCount)
	}
	if len(result.EventsProcessed) < 2 {
		t.Errorf("EventsProcessed len = %d, want at least 2", len(result.EventsProcessed))
	}
	// AggregateAuditEvents calls markEventsAsAggregated, which bulk-updates events to status=archived.
	if result.EventsUpdated < 2 {
		t.Errorf("EventsUpdated = %d, want at least 2 (aggregation must archive processed events)", result.EventsUpdated)
	}

	// Cleanup (archive) — idempotent: filter skips already-archived events, so count may be 0 after markEventsAsAggregated.
	archived, err := svc.CleanupAggregatedEvents(ctx, secCtx, result.EventsProcessed, true)
	if err != nil {
		t.Fatalf("CleanupAggregatedEvents(archive): %v", err)
	}
	t.Logf("CleanupAggregatedEvents returned archived count = %d", archived)
	if archived > 2 {
		t.Errorf("CleanupAggregatedEvents(archive) count = %d, unexpected", archived)
	}

	// Ensure all workers are finished before reading
	err = WaitForWALProcessing(f.GetProjectRoot(), 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForWALProcessing: %v", err)
	}
	FlushAllOrFail(t, f.GetProjectRoot())

	// Verify one event has status=archived (read back)
	obj, err := f.Read(ctx, secCtx, result.EventsProcessed[0])
	if err != nil {
		t.Fatalf("Read after archive: %v", err)
	}
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != "archived" {
		t.Errorf("Event status after cleanup = %q, want archived", status)
	}

	// Drain per-project CAS queues so t.TempDir cleanup does not race with background index writers.
	FlushAllOrFail(t, f.GetProjectRoot())
	_ = f.Shutdown(context.Background())
	// Nested write-behind / scenario dirs can outlive Flush under suite load; scrub before t.TempDir.
	_ = fileutil.RemoveAll(filepath.Join(baseTempDir, "test-scenarios"))
}
