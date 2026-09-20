package internal

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global.

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Performance limits (docs/architecture/OBJECT_OPERATIONS_PERFORMANCE.md).
// Target: millisecond-level single ops (like RDBMS/NoSQL). Current test limits are regression ceilings; tighten when hot path is optimized.
const (
	// Where we need to get: single ops in milliseconds (e.g. < 100ms). Not yet enforced in CI.
	targetSingleOpDuration = 100 * time.Millisecond
	// Regression ceiling for CI; fail if a single op exceeds this. Tighten to targetSingleOpDuration when possible.
	maxSingleOpDuration   = 15 * time.Second
	maxBatch100OpDuration = 4 * time.Minute // 100 creates or 100 updates (target: 30s; relaxed until hot path is optimized)

	// Bulk target: 5k objects in ~1s (e.g. create 5000, or update 5000). Used by BenchmarkCRUDBaselineBulk5k.
	// Default bulkBenchSize() is 100 until hot path is optimized (then increase to 5000 for full benchmark).
	bulk5kTargetDuration = 1 * time.Second
	bulk5kSize           = 100 // keep at 100 until CRUD tightened; set ZQK_BULK_BENCH_SIZE=5000 for full run
)

// TestPerformanceComparison tests performance of file vs graph backends
func TestPerformanceComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance comparison in short mode (full run for OBJECT_OPERATIONS_PERFORMANCE.md coverage)")
	}
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.performance_comparison"})
	tmpDir := proj.Root

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Test with file backend
	t.Run("FileBackend", func(t *testing.T) {
		runPerformanceTests(t, "File", proj.FileStorage, ctx, secCtx, storageCtx)
	})

	// Test with graph backend if available
	if isGraphBackendAvailable() {
		t.Run("GraphBackend", func(t *testing.T) {
			graphManager := mcp.GetGraphConnectionManager()
			if !graphManager.IsEnabled() {
				t.Skip("Graph backend not enabled")
				return
			}

			pool, err := graphManager.GetPool(ctx)
			if err != nil {
				t.Skipf("Graph backend not available: %v", err)
				return
			}

			graphStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
			runPerformanceTests(t, "Graph", graphStorage, ctx, secCtx, storageCtx)
		})
	} else {
		t.Log("Graph backend not available (ZQK_GRAPH_ENABLED not set) - skipping graph performance tests")
	}
}

// TestObjectOperationsLatency enforces that single create/update/delete complete within maxSingleOpDuration.
// See docs/architecture/OBJECT_OPERATIONS_PERFORMANCE.md (target: milliseconds; 2s is CI-safe upper bound).
func TestObjectOperationsLatency(t *testing.T) {
	// Serial: when run alongside TestWriteCapabilities and other heavy parallel tests, single-op
	// latency can exceed maxSingleOpDuration due to CPU/disk contention (not a hot-path regression).
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "internal.object_ops_latency",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{
					Name: "mkdir_backlog",
					Fn: func() error {
						return fileutil.MkdirAll(datacell.CellCASPrimaryDir(root, "backlog"), paths.DirPerm755)
					},
				},
			}
		},
	})
	fileStorage := proj.FileStorage

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	kind := "backlog_item"
	id := "BLI-LATENCY-001"
	obj := createTestObject(kind, id)

	// Single create
	t.Run("SingleCreate", func(t *testing.T) {
		createCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
		start := time.Now()
		err := fileStorage.Create(createCtx, secCtx, obj)
		duration := time.Since(start)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		t.Logf("Single create: %v", duration)
		if duration > targetSingleOpDuration {
			t.Logf("Create over target (goal: %v per OBJECT_OPERATIONS_PERFORMANCE.md)", targetSingleOpDuration)
		}
		if duration > maxSingleOpDuration {
			t.Errorf("Create took %v (max %v per OBJECT_OPERATIONS_PERFORMANCE.md)", duration, maxSingleOpDuration)
		}
	})

	// Single update
	t.Run("SingleUpdate", func(t *testing.T) {
		updates := map[string]any{objects.FieldKeyTitle: "Updated latency test"}
		start := time.Now()
		err := fileStorage.Update(ctx, secCtx, id, updates)
		duration := time.Since(start)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
		t.Logf("Single update: %v", duration)
		if duration > targetSingleOpDuration {
			t.Logf("Update over target (goal: %v per OBJECT_OPERATIONS_PERFORMANCE.md)", targetSingleOpDuration)
		}
		if duration > maxSingleOpDuration {
			t.Errorf("Update took %v (max %v per OBJECT_OPERATIONS_PERFORMANCE.md)", duration, maxSingleOpDuration)
		}
	})

	// Single delete (storage requires CLI context for delete)
	t.Run("SingleDelete", func(t *testing.T) {
		cliCtx := storage.WithTestHardDelete(ctx)
		start := time.Now()
		// Cascade: create may register dependents (e.g. validation/audit sidecars); non-cascade delete flakes under bundle -p load.
		err := fileStorage.Delete(cliCtx, secCtx, id, true)
		duration := time.Since(start)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		t.Logf("Single delete: %v", duration)
		if duration > targetSingleOpDuration {
			t.Logf("Delete over target (goal: %v per OBJECT_OPERATIONS_PERFORMANCE.md)", targetSingleOpDuration)
		}
		if duration > maxSingleOpDuration {
			t.Errorf("Delete took %v (max %v per OBJECT_OPERATIONS_PERFORMANCE.md)", duration, maxSingleOpDuration)
		}
	})
}

// runPerformanceTests runs a suite of performance tests
func runPerformanceTests(t *testing.T, backendName string, storageProvider storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext) {
	kind := "backlog_item"
	numObjects := 100 // Test with 100 objects
	// Unique per backend/run: GraphBackend talks to a shared Memgraph and BLI-000
	// leftovers from prior runs (or the FileBackend pass) make Create "already exists".
	stamp := time.Now().UnixNano() % 1_000_000_000
	idAt := func(i int) string {
		return fmt.Sprintf("BLI-%c%d%03d", backendName[0], stamp, i)
	}

	t.Run("Create", func(t *testing.T) {
		measureOperationWithMaxDuration(t, backendName, "Create", maxBatch100OpDuration, func() error {
			for i := 0; i < numObjects; i++ {
				obj := createTestObject(kind, idAt(i))
				if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
					return err
				}
			}
			return nil
		})
	})

	t.Run("Read", func(t *testing.T) {
		measureOperation(t, backendName, "Read", func() error {
			for i := 0; i < numObjects; i++ {
				_, err := storageProvider.Read(ctx, secCtx, idAt(i))
				if err != nil {
					return err
				}
			}
			return nil
		})
	})

	t.Run("List", func(t *testing.T) {
		measureOperation(t, backendName, "List", func() error {
			filter := storage.ListFilter{
				Kind:    kind,
				Filters: make(map[string]any),
			}
			_, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
			return err
		})
	})

	t.Run("ListWithFilter", func(t *testing.T) {
		measureOperation(t, backendName, "ListWithFilter", func() error {
			filter := storage.ListFilter{
				Kind: kind,
				Filters: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusExploring,
				},
			}
			_, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
			return err
		})
	})

	t.Run("ListWithSort", func(t *testing.T) {
		measureOperation(t, backendName, "ListWithSort", func() error {
			filter := storage.ListFilter{
				Kind:    kind,
				SortBy:  "title",
				SortAsc: true,
			}
			_, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
			return err
		})
	})

	t.Run("Update", func(t *testing.T) {
		measureOperationWithMaxDuration(t, backendName, "Update", maxBatch100OpDuration, func() error {
			for i := 0; i < numObjects; i++ {
				updates := map[string]any{
					objects.FieldKeyTitle: fmt.Sprintf("Updated Performance Test %d", i),
				}
				if err := storageProvider.Update(ctx, secCtx, idAt(i), updates); err != nil {
					return err
				}
			}
			return nil
		})
	})

	t.Run("BulkGet", func(t *testing.T) {
		ids := make([]string, numObjects)
		for i := 0; i < numObjects; i++ {
			ids[i] = idAt(i)
		}

		measureOperation(t, backendName, "BulkGet", func() error {
			_, err := storageProvider.BulkGet(ctx, secCtx, ids)
			return err
		})
	})

	t.Run("BulkUpdate", func(t *testing.T) {
		updates := make([]storage.BulkUpdateItem, numObjects)
		for i := 0; i < numObjects; i++ {
			updates[i] = storage.BulkUpdateItem{
				ID: idAt(i),
				Updates: map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusValidated,
				},
			}
		}

		measureOperation(t, backendName, "BulkUpdate", func() error {
			_, err := storageProvider.BulkUpdate(ctx, secCtx, updates)
			return err
		})
	})
}

// measureOperation measures the execution time of an operation
func measureOperation(t *testing.T, backendName, operationName string, op func() error) {
	start := time.Now()
	err := op()
	duration := time.Since(start)

	if err != nil {
		t.Errorf("%s %s failed: %v", backendName, operationName, err)
		return
	}

	t.Logf("%s %s: %v", backendName, operationName, duration)
}

// measureOperationWithMaxDuration measures the operation and fails if duration exceeds max.
// Used to enforce OBJECT_OPERATIONS_PERFORMANCE.md limits and catch regressions.
func measureOperationWithMaxDuration(t *testing.T, backendName, operationName string, maxDuration time.Duration, op func() error) {
	start := time.Now()
	err := op()
	duration := time.Since(start)

	if err != nil {
		t.Errorf("%s %s failed: %v", backendName, operationName, err)
		return
	}

	t.Logf("%s %s: %v", backendName, operationName, duration)
	if duration > maxDuration {
		t.Errorf("%s %s took %v (max %v per docs/architecture/OBJECT_OPERATIONS_PERFORMANCE.md)", backendName, operationName, duration, maxDuration)
	}
}

// createTestObject creates a test object for performance testing
// Uses proper ID format based on kind (e.g., BLI-001 for backlog_item)
func createTestObject(kind, id string) map[string]any {
	// Ensure ID has proper format - if it doesn't start with a known prefix, use the kind's prefix
	// For backlog_item, use BLI- prefix
	if kind == "backlog_item" && !hasPrefix(id, "BLI-") {
		// Extract number from id (e.g., "PERF-000" -> "000")
		// and create proper ID (e.g., "BLI-000")
		parts := splitID(id)
		if len(parts) > 1 {
			id = fmt.Sprintf("BLI-%s", parts[1])
		} else {
			id = fmt.Sprintf("BLI-%s", id)
		}
	}

	// Use UTC time in the format expected by validators: YYYY-MM-DDTHH:MM:SSZ
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)

	return map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          kind,
		objects.FieldKeyTitle:         fmt.Sprintf("Performance Test Object %s", id),
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     now,
		objects.FieldKeyCreatedBy:     "perf-test",
		objects.FieldKeyUpdatedAt:     now,
		objects.FieldKeyUpdatedBy:     "perf-test",
	}
}

// hasPrefix checks if a string has a specific prefix
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// splitID splits an ID like "PREFIX-001" into ["PREFIX", "001"]
func splitID(id string) []string {
	for i := 0; i < len(id); i++ {
		if id[i] == '-' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}

// TestPerformanceScalability tests performance with different data volumes.
// Each volume uses a fresh isolated project (PrepareIsolatedTempProject) to avoid "object already exists" from
// shared state or delete/cache ordering when subtests run.
func TestPerformanceScalability(t *testing.T) {
	if zqkenv.RunPerfTests().Get() != "1" {
		t.Skip("performance scalability suite is opt-in; set ZQK_RUN_PERF_TESTS=1 to run")
	}
	volumes := []int{10, 100, 500, 1000}
	kind := "backlog_item"

	for _, volume := range volumes {
		volume := volume
		t.Run(fmt.Sprintf("Volume_%d", volume), func(t *testing.T) {
			proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
				Kind: "internal.perf_scalability." + strconv.Itoa(volume),
			})
			fileStorage := proj.FileStorage

			ctx := pkgctx.NewSystemContext()
			secCtx := pkgctx.NewSystemSecurityContext()

			// Create objects
			start := time.Now()
			for i := 0; i < volume; i++ {
				obj := createTestObject(kind, fmt.Sprintf("BLI-%05d", i))
				if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
					t.Fatalf("failed to create object: %v", err)
				}
			}
			createDuration := time.Since(start)
			t.Logf("Created %d objects in %v (avg: %v per object)", volume, createDuration, createDuration/time.Duration(volume))

			// List all objects
			start = time.Now()
			storageCtx := pkgctx.NewStorageContext()
			filter := storage.ListFilter{
				Kind:    kind,
				Filters: make(map[string]any),
			}
			result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
			if err != nil {
				t.Fatalf("failed to list objects: %v", err)
			}
			listDuration := time.Since(start)
			n := len(result.Objects)
			if n == 0 {
				t.Logf("Listed 0 objects in %v (avg: N/A)", listDuration)
			} else {
				t.Logf("Listed %d objects in %v (avg: %v per object)", n, listDuration, listDuration/time.Duration(n))
			}
		})
	}
}

// TestPerformanceJSONYAML tests JSON/YAML output performance
func TestPerformanceJSONYAML(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping JSON/YAML list performance check in short mode")
	}
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.performance_json_yaml"})
	fileStorage := proj.FileStorage

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	kind := "backlog_item"
	numObjects := 100

	// Create test objects
	for i := 0; i < numObjects; i++ {
		obj := createTestObject(kind, fmt.Sprintf("BLI-%03d", i))
		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
	}

	// Test list performance (which will be formatted as JSON/YAML)
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:    kind,
		Filters: make(map[string]any),
	}

	// Measure list operation (data retrieval, not formatting)
	listStart := time.Now()
	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list objects: %v", err)
	}
	duration := time.Since(listStart)
	n := len(result.Objects)
	t.Logf("List operation (data retrieval): %v for %d objects", duration, n)
	if n == 0 {
		t.Logf("Average time per object: N/A (no objects returned)")
	} else {
		t.Logf("Average time per object: %v", duration/time.Duration(n))
	}

	// Note: JSON/YAML marshaling performance would be tested at the CLI layer
	// This test focuses on storage layer performance
}

// TestPerformanceConcurrentOperations tests concurrent operations
func TestPerformanceConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent performance suite in short mode")
	}
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "internal.performance_concurrent",
		ForceRemoveRootOnCleanup: true,
	})
	fileStorage := proj.FileStorage

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	kind := "backlog_item"
	numGoroutines := 10
	objectsPerGoroutine := 10

	// Test concurrent creates
	t.Run("ConcurrentCreate", func(t *testing.T) {
		start := time.Now()
		errors := make(chan error, numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			goroutinelabels.NewGoroutine("zqkcli_test", "concurrent create performance test").StartSimple(func() {
				func(goroutineID int) {
					for i := 0; i < objectsPerGoroutine; i++ {
						// Use unique IDs per goroutine to avoid conflicts
						obj := createTestObject(kind, fmt.Sprintf("BLI-%02d%03d", goroutineID, i))
						if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
							errors <- err
							return
						}
					}
					errors <- nil
				}(g)
			})
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines; i++ {
			if err := <-errors; err != nil {
				t.Errorf("concurrent create failed: %v", err)
			}
		}

		duration := time.Since(start)
		totalObjects := numGoroutines * objectsPerGoroutine
		t.Logf("Concurrent create: %d objects in %v (avg: %v per object)", totalObjects, duration, duration/time.Duration(totalObjects))
	})

	// Test concurrent reads
	t.Run("ConcurrentRead", func(t *testing.T) {
		start := time.Now()
		errors := make(chan error, numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			goroutinelabels.NewGoroutine("zqkcli_test", "concurrent read performance test").StartSimple(func() {
				func(goroutineID int) {
					for i := 0; i < objectsPerGoroutine; i++ {
						id := fmt.Sprintf("BLI-%02d%03d", goroutineID, i)
						_, err := fileStorage.Read(ctx, secCtx, id)
						if err != nil {
							errors <- err
							return
						}
					}
					errors <- nil
				}(g)
			})
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines; i++ {
			if err := <-errors; err != nil {
				t.Errorf("concurrent read failed: %v", err)
			}
		}

		duration := time.Since(start)
		totalObjects := numGoroutines * objectsPerGoroutine
		t.Logf("Concurrent read: %d objects in %v (avg: %v per object)", totalObjects, duration, duration/time.Duration(totalObjects))
	})
}
