package internal

//nolint:errcheck // Test cleanup operations - errors are acceptable

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/testservices"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestGraphBackendIntegration tests the full graph backend integration
// This test spins up MemGraph, runs tests, and cleans up
//
//nolint:gocyclo // Test function intentionally exercises many integration scenarios
func TestGraphBackendIntegration(t *testing.T) {
	// Not t.Parallel(): PrepareIsolatedTempProject uses t.Setenv(ZQK_TEST_ROOT).
	// Skip if graph backend is not enabled
	if !config.StorageGraphEnabled().Safe() {
		t.Skip("Graph backend not enabled (set ZQK_GRAPH_ENABLED=true)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.graph_integration"})
	testRoot := proj.Root

	// Setup test services (MemGraph)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()
	ctx = pkgctx.WithAllowCoreObjectDelete(storage.WithCLIOperation(ctx))

	services, err := testservices.SetupTestServices(ctx)
	if err != nil {
		t.Fatalf("failed to setup test services: %v", err)
	}
	//nolint:errcheck // Test cleanup - errors are acceptable
	defer func() { _ = services.Cleanup() }()

	// Verify MemGraph is running
	graphManager := mcp.GetGraphConnectionManager()
	if !graphManager.IsEnabled() {
		t.Fatal("Graph backend should be enabled")
	}

	// Get connection pool
	pool, err := graphManager.GetPool(ctx)
	if err != nil {
		t.Fatalf("failed to get graph connection pool: %v", err)
	}

	// Create graph storage
	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Clear any existing test data from previous runs
	//nolint:errcheck // Test query - error acceptable
	_, _ = graphStorage.Query(ctx, secCtx, storageCtx, storage.Query{
		Type:       storage.QueryTypeCypher,
		Expression: "MATCH (n) WHERE n.id STARTS WITH 'BLI-TEST-' OR n.id STARTS WITH 'BLI-PERF-' DETACH DELETE n",
	})

	// Use unique test ID to avoid conflicts with previous test runs
	// Format must match validation pattern: ^[A-Z]+-\d{3,}$
	testID := fmt.Sprintf("BLI-%03d", time.Now().UnixNano()%1000000)

	// Test basic operations
	t.Run("Create", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Graph Backend Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "test",
		}

		err := graphStorage.Create(ctx, secCtx, obj)
		if err != nil {
			// If object already exists, try to delete and recreate
			if strings.Contains(err.Error(), "already exists") {
				//nolint:errcheck // Test cleanup - errors are acceptable
				_ = graphStorage.Delete(ctx, secCtx, testID, false)
				err = graphStorage.Create(ctx, secCtx, obj)
			}
			if err != nil {
				t.Fatalf("failed to create object in graph: %v", err)
			}
		}
	})

	t.Run("Read", func(t *testing.T) {
		obj, err := graphStorage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Fatalf("failed to read object from graph: %v", err)
		}
		if obj == nil {
			t.Fatal("read returned nil object")
		}
		if obj[objects.FieldKeyID] != testID {
			t.Errorf("expected id %s, got %v", testID, obj[objects.FieldKeyID])
		}
	})

	t.Run("List", func(t *testing.T) {
		filter := storage.ListFilter{
			Kind:    "backlog_item",
			Filters: make(map[string]any),
		}

		result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("failed to list objects from graph: %v", err)
		}
		if len(result.Objects) == 0 {
			t.Error("expected at least one object in list")
		}
	})

	t.Run("Update", func(t *testing.T) {
		updates := map[string]any{
			objects.FieldKeyTitle: "Updated Graph Backend Test",
		}

		err := graphStorage.Update(ctx, secCtx, testID, updates)
		if err != nil {
			t.Fatalf("failed to update object in graph: %v", err)
		}

		// Verify update
		obj, err := graphStorage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Fatalf("failed to read updated object: %v", err)
		}
		if obj == nil {
			t.Fatal("read returned nil object after update")
		}
		if obj[objects.FieldKeyTitle] != "Updated Graph Backend Test" {
			t.Errorf("expected updated title, got %v", obj[objects.FieldKeyTitle])
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := graphStorage.Delete(ctx, secCtx, testID, false)
		if err != nil {
			t.Fatalf("failed to delete object from graph: %v", err)
		}

		// Verify deletion
		_, err = graphStorage.Read(ctx, secCtx, testID)
		if err == nil {
			t.Error("expected error when reading deleted object")
		}
	})

	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()
	metrics.FlushGraphProviderMetricsToStorage(graphStorage, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
}

// TestGraphBackendPerformance tests graph backend performance
func TestGraphBackendPerformance(t *testing.T) {
	// Skip if graph backend is not enabled
	if !config.StorageGraphEnabled().Safe() {
		t.Skip("Graph backend not enabled (set ZQK_GRAPH_ENABLED=true)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.graph_performance"})
	testRoot := proj.Root

	// Setup test services
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Minute)
	defer cancel()
	ctx = pkgctx.WithAllowCoreObjectDelete(storage.WithCLIOperation(ctx))

	services, err := testservices.SetupTestServices(ctx)
	if err != nil {
		t.Fatalf("failed to setup test services: %v", err)
	}
	//nolint:errcheck // Test cleanup - errors are acceptable
	defer func() { _ = services.Cleanup() }()

	// Get graph storage
	graphManager := mcp.GetGraphConnectionManager()
	pool, err := graphManager.GetPool(ctx)
	if err != nil {
		t.Fatalf("failed to get graph connection pool: %v", err)
	}

	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Run performance tests
	numObjects := 100
	kind := "backlog_item"

	// Create objects
	start := time.Now()
	for i := 0; i < numObjects; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("BLI-PERF-%03d", i),
			objects.FieldKeyKind:          kind,
			objects.FieldKeyTitle:         fmt.Sprintf("Performance Test %d", i),
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "perf-test",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "perf-test",
		}
		if err := graphStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
	}
	createDuration := time.Since(start)
	t.Logf("Graph Create: %d objects in %v (avg: %v per object)", numObjects, createDuration, createDuration/time.Duration(numObjects))

	// List objects
	start = time.Now()
	filter := storage.ListFilter{
		Kind:    kind,
		Filters: make(map[string]any),
	}
	result, err := graphStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list objects: %v", err)
	}
	listDuration := time.Since(start)
	t.Logf("Graph List: %d objects in %v (avg: %v per object)", len(result.Objects), listDuration, listDuration/time.Duration(len(result.Objects)))
}
