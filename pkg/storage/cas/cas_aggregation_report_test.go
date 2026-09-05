package cas_test

import (
	"context"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestCAS_AuditAggregation tests that audit event aggregation works with CAS
func TestCAS_AuditAggregation(t *testing.T) {
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-aggregation-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	processDir := datacell.ProcessPrimaryDir(testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-1785920548450214012-68b850c0"}
	_ = storage.InitializeGlobalBufferWithConfig(testRoot, secCtx)
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr == nil {
			defer fileutil.RemoveAll(resetDir)
			opts.TearDownGlobalAuditBuffer = true
			opts.SecCtx = secCtx
			opts.AuditBufferResetRoot = resetDir
		}
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Verify CAS is enabled for audit_event
	if !fileStorage.UsesContentAddressableStorage("audit_event") {
		t.Fatalf("CAS should be enabled for audit_event when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	storageCtx := pkgctx.GetStorageContext()

	// Create several audit events using storage.WriteSystemObjectAndRegisterHash (what scheduler uses)
	now := time.Now().UTC()
	windowStart := now.Add(-2 * time.Hour)
	windowEnd := now

	auditEvents := []map[string]any{
		{
			objects.FieldKeyID:            "AUD-AGG-001",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     windowStart.Add(30 * time.Minute).Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyUpdatedAt:     windowStart.Add(30 * time.Minute).Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject: validation.DefaultOriginProject,
			objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
			objects.FieldKeyEventType:     "cache_update",
			objects.FieldKeyOperation:     "Cache update test 1",
			objects.FieldKeyTargetKind:    "backlog_item",
			objects.FieldKeySeverity:      "low",
		},
		{
			objects.FieldKeyID:            "AUD-AGG-002",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     windowStart.Add(60 * time.Minute).Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyUpdatedAt:     windowStart.Add(60 * time.Minute).Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject: validation.DefaultOriginProject,
			objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
			objects.FieldKeyEventType:     "cache_update",
			objects.FieldKeyOperation:     "Cache update test 2",
			objects.FieldKeyTargetKind:    "backlog_item",
			objects.FieldKeySeverity:      "low",
		},
		{
			objects.FieldKeyID:            "AUD-AGG-003",
			objects.FieldKeyKind:          "audit_event",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     windowStart.Add(90 * time.Minute).Format(time.RFC3339),
			objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyUpdatedAt:     windowStart.Add(90 * time.Minute).Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject: validation.DefaultOriginProject,
			objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
			objects.FieldKeyEventType:     "cache_invalidation",
			objects.FieldKeyOperation:     "Cache invalidation test",
			objects.FieldKeyTargetKind:    "backlog_item",
			objects.FieldKeySeverity:      "low",
		},
	}

	// Create audit events in CAS
	for _, event := range auditEvents {
		data, err := storage.FormatMultiLineYAML(event)
		if err != nil {
			t.Fatalf("Failed to format audit event: %v", err)
		}

		// Determine audit directory (bucketed by month)
		month := time.Now().Format("2006-01")
		auditDir := filepath.Join(processDir, "audit", month)
		if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create audit directory: %v", err)
		}

		auditFilePath := filepath.Join(auditDir, event[objects.FieldKeyID].(string)+".yaml")
		err = storage.WriteSystemObjectAndRegisterHash(auditFilePath, data, "audit_event", auditDir, event[objects.FieldKeyID].(string), fileStorage)
		if err != nil {
			t.Fatalf("Failed to create audit event %s: %v", event[objects.FieldKeyID], err)
		}
	}

	// Wait for index updates to be processed before aggregation
	if err := storage.FlushListingIndexForProjectRoot(testRoot, "audit_event"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Create aggregation service
	service := storage.NewAuditAggregationService(fileStorage)

	// Aggregate events
	result, err := service.AggregateAuditEvents(ctx, secCtx, storageCtx, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("Failed to aggregate audit events: %v", err)
	}

	// Verify aggregation result
	if result.EventCount < len(auditEvents) {
		t.Errorf("Expected at least %d events, got %d", len(auditEvents), result.EventCount)
	}

	if result.MetricsCreated == 0 {
		t.Errorf("Expected at least one metric to be created")
	}

	// Verify the aggregation metric was created in CAS
	if result.MetricID != "" {
		metric, err := fileStorage.Read(ctx, secCtx, result.MetricID)
		if err != nil {
			t.Fatalf("Failed to read aggregation metric: %v", err)
		}

		if metric[objects.FieldKeyKind] != "audit_aggregation_metric" {
			t.Errorf("Expected kind 'audit_aggregation_metric', got %v", metric[objects.FieldKeyKind])
		}

		// Wait for index updates to be processed
		if err := storage.FlushListingIndexForProjectRoot(testRoot, "audit_aggregation_metric"); err != nil {
			t.Fatalf("Failed to flush write queue: %v", err)
		}

		// Verify metric is in CAS
		cas, err := fileStorage.GetContentAddressableStorage("audit_aggregation_metric")
		if err != nil {
			t.Fatalf("Failed to get CAS: %v", err)
		}

		_, err = cas.GetHashForID(result.MetricID)
		if err != nil {
			t.Errorf("Aggregation metric should exist in CAS index: %v", err)
		}
	}
}

// TestCAS_ReportGeneration tests that report generation works with CAS objects
func TestCAS_ReportGeneration(t *testing.T) {
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-report-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, fileStorage)); err != nil {
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
	storageCtx := pkgctx.GetStorageContext()

	// Create test objects
	testObjs := []map[string]any{
		{
			objects.FieldKeyID:            "BLI-200",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Report Test 1",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "BLI-201",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Report Test 2",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "BLI-202",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Report Test 3",
			objects.FieldKeyStatus:        objects.ObjectStatusComplete,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
	}

	// Create objects in CAS
	for _, obj := range testObjs {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")
	}

	// Test List operation (what reports use to read objects).
	// CreateCASVisible promotes preliminary exploring → roadmap.
	filter := storage.ListFilter{
		Kind: "backlog_item",
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusRoadmap,
		},
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}

	// Verify we got the correct objects
	if len(result.Objects) < 2 {
		t.Errorf("Expected at least 2 objects with status 'roadmap', got %d", len(result.Objects))
	}

	// Verify all returned objects are from CAS
	for _, obj := range result.Objects {
		objID, _ := obj[objects.FieldKeyID].(string)
		if objID == "" {
			continue
		}

		// Verify object exists in CAS
		cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
		if err != nil {
			t.Fatalf("Failed to get CAS: %v", err)
		}

		_, err = cas.GetHashForID(objID)
		if err != nil {
			t.Errorf("Object %s should exist in CAS index: %v", objID, err)
		}

		// Verify we can read it back
		readObj, err := fileStorage.Read(ctx, secCtx, objID)
		if err != nil {
			t.Errorf("Failed to read object %s: %v", objID, err)
		}

		if readObj[objects.FieldKeyID] != objID {
			t.Errorf("Expected ID %s, got %v", objID, readObj[objects.FieldKeyID])
		}
	}
}
