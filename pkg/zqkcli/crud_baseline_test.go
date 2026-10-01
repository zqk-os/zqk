// CRUD baseline test: isolated environment, multiple kinds, duration assertions and optional baseline capture.
// See test-scenarios/crud-baseline/README.md and docs/architecture/OBJECT_OPERATIONS_PERFORMANCE.md.

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// Env vars for profiling during CRUD baseline test (see test-scenarios/crud-baseline/README.md).
// ZQK_CRUD_PROFILE=1   write CPU profile for the CRUD loop to baselines/crud_cpu_<timestamp>.pprof
// ZQK_CRUD_HEAP_PROFILE=1  write heap profile at end to baselines/crud_mem_<timestamp>.pprof

// crudBaselineRecord holds per-kind, per-operation timings for one run.
type crudBaselineRecord struct {
	RunAt   string                    `json:"run_at"`
	Kind    string                    `json:"kind"`
	Create  string                    `json:"create_ms,omitempty"`
	Read    string                    `json:"read_ms,omitempty"`
	Update  string                    `json:"update_ms,omitempty"`
	Delete  string                    `json:"delete_ms,omitempty"`
	Summary map[string]map[string]int `json:"summary,omitempty"` // kind -> op -> ms
}

// startCRUDProfile starts CPU (and optionally heap) profiling when ZQK_CRUD_PROFILE=1 / ZQK_CRUD_HEAP_PROFILE=1.
// Returns a stop function; call it after the CRUD work so the profile covers only the hot path.
// Profiles are written to test-scenarios/crud-baseline/baselines/ or current dir.
func startCRUDProfile(t *testing.T, tag string) (stop func()) {
	profileDir := getProfileDir()
	ts := time.Now().UTC().Format("20060102T150405")
	var cpuFile *fileutil.File
	if zqkenv.CRUDProfile().Get() == "1" {
		cpuPath := filepath.Join(profileDir, tag+"_cpu_"+ts+".pprof")
		var err error
		cpuFile, err = fileutil.Create(cpuPath)
		if err != nil {
			t.Logf("CRUD profile: could not create CPU profile %s: %v", cpuPath, err)
		} else if err := pprof.StartCPUProfile(cpuFile); err != nil {
			_ = cpuFile.Close()
			t.Logf("CRUD profile: could not start CPU profile: %v", err)
			cpuFile = nil
		} else {
			t.Logf("CRUD profile: writing CPU profile to %s", cpuPath)
		}
	}
	doHeap := zqkenv.CRUDHeapProfile().Get() == "1"
	return func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			_ = cpuFile.Close()
			cpuFile = nil
		}
		if doHeap {
			memPath := filepath.Join(profileDir, tag+"_mem_"+ts+".pprof")
			f, err := fileutil.Create(memPath)
			if err != nil {
				t.Logf("CRUD profile: could not create heap profile %s: %v", memPath, err)
				return
			}
			if err := pprof.WriteHeapProfile(f); err != nil {
				t.Logf("CRUD profile: could not write heap profile: %v", err)
			} else {
				t.Logf("CRUD profile: wrote heap profile to %s", memPath)
			}
			_ = f.Close()
		}
	}
}

func getProfileDir() string {
	if root := findProjectRootForBaseline(); root != emptyValue {
		baselinesDir := filepath.Join(root, "test-scenarios", "crud-baseline", "baselines")
		if fi, err := fileutil.Stat(baselinesDir); err == nil && fi.IsDir() {
			return baselinesDir
		}
	}
	return "."
}

// TestCRUDBaselineConsistency runs full CRUD (create, read, update, delete) for multiple object kinds
// in an isolated temp environment (specs copied from project; no project data modified).
// Each single operation must complete within maxSingleOpDuration.
// Optionally writes a baseline JSON to test-scenarios/crud-baseline/baselines/ when run from repo root.
// Set ZQK_CRUD_PROFILE=1 (and optionally ZQK_CRUD_HEAP_PROFILE=1) to write pprof files for optimization.
func TestCRUDBaselineConsistency(t *testing.T) {
	// Not t.Parallel(): SetupCompleteTestEnvironment sets ZQK_TEST_ROOT via os.Setenv (process-global).
	factory := storage.NewTestingFactory()
	env := setupCompleteTestEnvironmentInternal(t, factory)
	defer env.Cleanup()

	str, ok := env.Storage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatalf("storage is not ObjectStorageProvider")
	}

	// Pre-warm globals for test root so we measure warm path (ZQK_TEST_ROOT already set by SetupCompleteTestEnvironment).
	objects.PrewarmGlobalsForProjectRoot(env.TestRoot)
	// Eager-init bucket strategy registry on this storage so first Create does not pay (same as CLI OnStorageCreated).
	if fileStorage, ok := str.(*storage.FileObjectStorage); ok && fileStorage != nil {
		fileStorage.EnsureBucketStrategyRegistryReady(pkgctx.NewSystemContext())
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := env.SecurityContext
	cliCtx := storage.WithTestHardDelete(ctx)

	// Start profiling the CRUD loop only (stop after deferred deletes).
	stopProfile := startCRUDProfile(t, "crud")
	defer stopProfile()

	// Order matters: requirement needs goal_refs and criteria_refs; goal and criteria must exist when requirement is created.
	// We defer deleting goal and criteria until after requirement is created and deleted.
	kinds := []struct {
		kind        string
		id          string
		obj         func() map[string]any
		deferDelete bool // if true, delete after all kinds processed (so requirement can reference goal/criteria)
	}{
		{kind: "backlog_item", id: "BLI-BASELINE-001", obj: func() map[string]any { return minimalBacklogItem("BLI-BASELINE-001") }},
		{kind: "policy", id: "POL-CODE-901", obj: func() map[string]any { return minimalPolicy("POL-CODE-901") }},
		{kind: "goal", id: "GOAL-BASELINE-001", obj: func() map[string]any { return minimalGoal("GOAL-BASELINE-001") }, deferDelete: true},
		{kind: "criteria", id: "CRIT-BASELINE-001", obj: func() map[string]any { return minimalCriteria("CRIT-BASELINE-001") }, deferDelete: true},
		{kind: "milestone", id: "MIL-BASELINE-001", obj: func() map[string]any { return minimalMilestone("MIL-BASELINE-001") }},
		{kind: "requirement", id: "REQ-BASELINE-001", obj: func() map[string]any { return minimalRequirement("REQ-BASELINE-001") }},
	}

	runAt := zqktime.NowRFC3339UTC()
	var records []crudBaselineRecord
	summary := make(map[string]map[string]int)

	for _, k := range kinds {
		rec := crudBaselineRecord{RunAt: runAt, Kind: k.kind}
		createCtx := pkgctx.WithCacheUpdate(ctx, k.id, k.kind, "")

		// Create
		obj := k.obj()
		start := time.Now()
		if err := str.Create(createCtx, secCtx, obj); err != nil {
			t.Errorf("CRUD baseline %s Create: %v", k.kind, err)
			continue
		}
		d := time.Since(start)
		rec.Create = durationMs(d)
		if summary[k.kind] == nil {
			summary[k.kind] = make(map[string]int)
		}
		summary[k.kind]["create_ms"] = int(d.Milliseconds())
		if d > maxSingleOpDuration {
			t.Errorf("CRUD baseline %s Create took %v (max %v)", k.kind, d, maxSingleOpDuration)
		}

		// Read
		start = time.Now()
		if _, err := str.Read(ctx, secCtx, k.id); err != nil {
			t.Errorf("CRUD baseline %s Read: %v", k.kind, err)
			continue
		}
		d = time.Since(start)
		rec.Read = durationMs(d)
		summary[k.kind]["read_ms"] = int(d.Milliseconds())
		if d > maxSingleOpDuration {
			t.Errorf("CRUD baseline %s Read took %v (max %v)", k.kind, d, maxSingleOpDuration)
		}

		// Update
		start = time.Now()
		if err := str.Update(ctx, secCtx, k.id, map[string]any{objects.FieldKeyTitle: "Updated baseline " + k.kind}); err != nil {
			t.Errorf("CRUD baseline %s Update: %v", k.kind, err)
			continue
		}
		d = time.Since(start)
		rec.Update = durationMs(d)
		summary[k.kind]["update_ms"] = int(d.Milliseconds())
		if d > maxSingleOpDuration {
			t.Errorf("CRUD baseline %s Update took %v (max %v)", k.kind, d, maxSingleOpDuration)
		}

		// Delete (or defer for goal/criteria so requirement can reference them)
		if k.deferDelete {
			rec.Delete = "(deferred)"
			records = append(records, rec)
			t.Logf("CRUD baseline %s: create=%s read=%s update=%s delete=deferred", k.kind, rec.Create, rec.Read, rec.Update)
			continue
		}
		start = time.Now()
		if err := str.Delete(cliCtx, secCtx, k.id, true); err != nil {
			t.Errorf("CRUD baseline %s Delete: %v", k.kind, err)
			continue
		}
		d = time.Since(start)
		rec.Delete = durationMs(d)
		summary[k.kind]["delete_ms"] = int(d.Milliseconds())
		if d > maxSingleOpDuration {
			t.Errorf("CRUD baseline %s Delete took %v (max %v)", k.kind, d, maxSingleOpDuration)
		}

		records = append(records, rec)
		t.Logf("CRUD baseline %s: create=%s read=%s update=%s delete=%s", k.kind, rec.Create, rec.Read, rec.Update, rec.Delete)
	}

	// Deferred deletes: goal before criteria (requirement already deleted; goal may reference criteria).
	// Cascade clears validation/audit dependents in isolated baseline env.
	for i := 0; i < len(kinds); i++ {
		k := kinds[i]
		if !k.deferDelete {
			continue
		}
		start := time.Now()
		if err := str.Delete(cliCtx, secCtx, k.id, true); err != nil {
			t.Errorf("CRUD baseline %s deferred Delete: %v", k.kind, err)
			continue
		}
		d := time.Since(start)
		if summary[k.kind] == nil {
			summary[k.kind] = make(map[string]int)
		}
		summary[k.kind]["delete_ms"] = int(d.Milliseconds())
		if d > maxSingleOpDuration {
			t.Errorf("CRUD baseline %s deferred Delete took %v (max %v)", k.kind, d, maxSingleOpDuration)
		}
		t.Logf("CRUD baseline %s deferred delete: %s", k.kind, durationMs(d))
		// Update record for baseline file
		for j := range records {
			if records[j].Kind == k.kind {
				records[j].Delete = durationMs(d)
				break
			}
		}
	}

	// Optional: write baseline file under test-scenarios/crud-baseline/baselines/
	if projectRoot := findProjectRootForBaseline(); projectRoot != emptyValue {
		baselinesDir := filepath.Join(projectRoot, "test-scenarios", "crud-baseline", "baselines")
		if fi, err := fileutil.Stat(baselinesDir); err == nil && fi.IsDir() {
			out := struct {
				RunAt   string                    `json:"run_at"`
				Summary map[string]map[string]int `json:"summary"`
				Records []crudBaselineRecord      `json:"records"`
			}{RunAt: runAt, Summary: summary, Records: records}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Logf("baseline marshal: %v", err)
			} else {
				fname := "crud_baseline_" + time.Now().UTC().Format("20060102T150405") + ".json"
				path := filepath.Join(baselinesDir, fname)
				if err := fileutil.WriteFile(path, data, paths.FilePerm644); err != nil {
					t.Logf("baseline write %s: %v", path, err)
				} else {
					t.Logf("wrote baseline: %s", path)
				}
			}
		}
	}
}

// CRUD 1k test parameters — adjust these to scale; timeouts are derived proportionally.
const (
	crud1kDefaultObjects    = 1000 // default object count; override with ZQK_CRUD_BASELINE_COUNT
	crud1kSecPerObject      = 1    // seconds per object for recommended test -timeout (CRUD loop + drain)
	crud1kShutdownSecPerObj = 1    // seconds per object for shutdown drain timeout
	crud1kTimeoutBaseMin    = 2    // base minutes for recommended test timeout
	crud1kShutdownBaseMin   = 1    // base minutes for shutdown drain timeout
)

func getCRUDBaseline1kCount() int {
	if n := config.TestingCRUDBaselineCount().OrDefault(0); n > 0 {
		return n
	}
	return crud1kDefaultObjects
}

// crud1kRecommendedTestTimeout returns the recommended -timeout for the test binary (scales with n).
func crud1kRecommendedTestTimeout(n int) time.Duration {
	return time.Duration(crud1kTimeoutBaseMin)*time.Minute + time.Duration(n*crud1kSecPerObject)*time.Second
}

// crud1kShutdownTimeout returns timeout for deferred Shutdown (scales with n).
func crud1kShutdownTimeout(n int) time.Duration {
	return time.Duration(crud1kShutdownBaseMin)*time.Minute + time.Duration(n*crud1kShutdownSecPerObj)*time.Second
}

// TestCRUDBaseline1k runs full CRUD (create, read, update, delete) for N backlog_item objects
// with write-behind enabled. N = crud1kDefaultObjects (1000) or ZQK_CRUD_BASELINE_COUNT.
// Timeouts scale with N; log at start shows recommended: go test ... -timeout <duration>.
// ZQK_CRUD_PROFILE=1 writes crud_1k_cpu_<timestamp>.pprof and baseline JSON.
// Skipped in short mode or when test deadline is too low (e.g. scheduler job 300s) so the bundle can finish.
func TestCRUDBaseline1k(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CRUD 1k baseline in short mode (use full run for performance baseline)")
	}
	n := getCRUDBaseline1kCount()
	recommended := crud1kRecommendedTestTimeout(n)
	// If the test has a deadline (e.g. go test -timeout 300s), skip when it's insufficient for this test.
	if deadline, ok := t.Deadline(); ok && time.Until(deadline) < recommended {
		t.Skipf("skipping CRUD 1k baseline: remaining time %s < recommended %s (run with -timeout %s or -short)",
			time.Until(deadline).Round(time.Second), recommended.Round(time.Second), recommended)
	}
	t.Logf("CRUD 1k: N=%d; use -timeout %s (or proportional) so test + drain complete", n, recommended)

	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	testkit.RegisterStandardTeardown(t, testkit.TempProjectTeardown(tmpDir, nil))
	if _, err := testenvroot.Setup(tmpDir); err != nil {
		t.Fatalf("testenvroot.Setup: %v", err)
	}
	if err := copySpecsToInternalTestRoot(tmpDir); err != nil {
		t.Skipf("CopySpecsToTestRoot (run from repo root): %v", err)
	}

	st, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, st)
	shutdownTimeout := crud1kShutdownTimeout(n)
	defer func() {
		ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), shutdownTimeout)
		defer cancel()
		_ = st.Shutdown(ctx)
	}()

	objects.PrewarmGlobalsForProjectRoot(tmpDir)
	st.EnsureBucketStrategyRegistryReady(pkgctx.NewSystemContext())

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithTestHardDelete(ctx)
	kind := "backlog_item"

	stopProfile := startCRUDProfile(t, "crud_1k")
	defer stopProfile()

	var createTotal, readTotal, updateTotal, deleteTotal time.Duration
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("BLI-BASELINE-%04d", i+1)
		obj := minimalBacklogItem(id)
		createCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")

		start := time.Now()
		if err := st.Create(createCtx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
		d := time.Since(start)
		createTotal += d
		if d > maxSingleOpDuration {
			t.Errorf("Create %s took %v (max %v)", id, d, maxSingleOpDuration)
		}

		start = time.Now()
		if _, err := st.Read(ctx, secCtx, id); err != nil {
			t.Fatalf("Read %s: %v", id, err)
		}
		d = time.Since(start)
		readTotal += d
		if d > maxSingleOpDuration {
			t.Errorf("Read %s took %v (max %v)", id, d, maxSingleOpDuration)
		}

		start = time.Now()
		if err := st.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyTitle: "Updated baseline 1k " + id}); err != nil {
			t.Fatalf("Update %s: %v", id, err)
		}
		d = time.Since(start)
		updateTotal += d
		if d > maxSingleOpDuration {
			t.Errorf("Update %s took %v (max %v)", id, d, maxSingleOpDuration)
		}

		start = time.Now()
		if err := st.Delete(cliCtx, secCtx, id, false); err != nil {
			t.Fatalf("Delete %s: %v", id, err)
		}
		d = time.Since(start)
		deleteTotal += d
		if d > maxSingleOpDuration {
			t.Errorf("Delete %s took %v (max %v)", id, d, maxSingleOpDuration)
		}
	}

	createAvg := createTotal.Milliseconds() / int64(n)
	readAvg := readTotal.Milliseconds() / int64(n)
	updateAvg := updateTotal.Milliseconds() / int64(n)
	deleteAvg := deleteTotal.Milliseconds() / int64(n)
	t.Logf("CRUD 1k (%d objects): create_avg=%d read_avg=%d update_avg=%d delete_avg=%d ms", n, createAvg, readAvg, updateAvg, deleteAvg)

	// Write baseline JSON when run from repo root
	if projectRoot := findProjectRootForBaseline(); projectRoot != emptyValue {
		baselinesDir := filepath.Join(projectRoot, "test-scenarios", "crud-baseline", "baselines")
		if fi, err := fileutil.Stat(baselinesDir); err == nil && fi.IsDir() {
			runAt := zqktime.NowRFC3339UTC()
			out := struct {
				RunAt     string `json:"run_at"`
				Objects   int    `json:"object_count"`
				CreateAvg int64  `json:"create_avg_ms"`
				ReadAvg   int64  `json:"read_avg_ms"`
				UpdateAvg int64  `json:"update_avg_ms"`
				DeleteAvg int64  `json:"delete_avg_ms"`
			}{RunAt: runAt, Objects: n, CreateAvg: createAvg, ReadAvg: readAvg, UpdateAvg: updateAvg, DeleteAvg: deleteAvg}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Logf("baseline marshal: %v", err)
			} else {
				fname := "crud_1k_baseline_" + time.Now().UTC().Format("20060102T150405") + ".json"
				path := filepath.Join(baselinesDir, fname)
				if err := fileutil.WriteFile(path, data, paths.FilePerm644); err != nil {
					t.Logf("baseline write %s: %v", path, err)
				} else {
					t.Logf("wrote baseline: %s", path)
				}
			}
		}
	}
}

func durationMs(d time.Duration) string {
	return fmtDurationMs(d)
}

func fmtDurationMs(d time.Duration) string {
	ms := d.Milliseconds()
	if ms == 0 && d > 0 {
		return "<1"
	}
	return fmt.Sprintf("%d", ms)
}

// findProjectRootForBaseline returns project root by walking up from cwd; empty if not found.
func findProjectRootForBaseline() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := fileutil.Stat(filepath.Join(dir, "test-scenarios", "crud-baseline")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func minimalBacklogItem(id string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "CRUD baseline backlog item",
		objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-TEST", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-TEST",
	}
}

func minimalPolicy(id string) map[string]any {
	// Policy ID must match ^POL-[A-Z]+-\d{3,}$ (e.g. POL-CODE-901).
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "policy", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyTitle: "CRUD baseline policy", objects.FieldKeyPolicyType: "requirement", objects.FieldKeyCategory: "code_quality",
		objects.FieldKeyBody: "CRUD baseline policy body", "enforcement_level": "required",
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-TEST", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-TEST",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject, objects.FieldKeyOriginSystem: validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID: paths.KernelNamespaceID,
	}
}

func minimalGoal(id string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "CRUD baseline goal",
		objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-TEST", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-TEST",
	}
}

func minimalCriteria(id string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "criteria", objects.FieldKeyTitle: "CRUD baseline criteria",
		objects.FieldKeyDescription: "Valid criteria description exceeding ten characters",
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "functional",
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-TEST", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-TEST",
	}
}

func minimalMilestone(id string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "milestone", objects.FieldKeyTitle: "CRUD baseline milestone",
		objects.FieldKeyStatus: objects.ObjectStatusNotStarted, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-TEST", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-TEST",
	}
}

func minimalRequirement(id string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "requirement", objects.FieldKeyTitle: "CRUD baseline requirement",
		objects.FieldKeyPriority: "p2",
		objects.FieldKeyStatus:   objects.ObjectStatusProposed, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyGoalRefs: []string{"GOAL-BASELINE-001"}, objects.FieldKeyCriteriaRefs: []string{"CRIT-BASELINE-001"},
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-TEST", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-TEST",
	}
}

// bulkBenchSize returns the number of objects per batch for bulk benchmarks. Default bulk5kSize (5000);
// set ZQK_BULK_BENCH_SIZE=100 for quick profiling runs.
func bulkBenchSize() int {
	if n := config.StorageBulkBenchSize().OrDefault(0); n > 0 {
		return n
	}
	return bulk5kSize
}

// BenchmarkCRUDBaselineBulk5kCreate measures time to create N backlog_item objects then delete them
// (one "batch" per benchmark iteration). N = bulk5kSize (5000) by default; ZQK_BULK_BENCH_SIZE=100 for quick runs.
// Target: bulk5kTargetDuration (~1s for 5k ops). Run with -cpuprofile=bulk5k_cpu.pprof -benchtime=1x to profile.
func BenchmarkCRUDBaselineBulk5kCreate(b *testing.B) {
	n := bulkBenchSize()
	proj := testkit.PrepareIsolatedTempProjectForBenchmark(b, &testkit.IsolatedTempProjectOptions{
		Kind:                      "internal.benchmark_crud_bulk5k_create",
		UsePlainFileObjectStorage: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "copy_specs_to_test_root",
				Fn: func() error {
					return copySpecsToInternalTestRoot(root)
				},
			}}
		},
	})
	str := proj.FileStorage

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithTestHardDelete(ctx)
	kind := "backlog_item"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < n; j++ {
			id := fmt.Sprintf("BLI-BULK-%d-%d", i, j)
			createCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
			obj := createTestObject(kind, id)
			if err := str.Create(createCtx, secCtx, obj); err != nil {
				b.Fatalf("create %s: %v", id, err)
			}
		}
		for j := 0; j < n; j++ {
			id := fmt.Sprintf("BLI-BULK-%d-%d", i, j)
			if err := str.Delete(cliCtx, secCtx, id, false); err != nil {
				b.Fatalf("delete %s: %v", id, err)
			}
		}
	}
}

// BenchmarkCRUDBaselineBulk5kCreateOnly runs N creates per iteration (no delete). N = bulk5kSize by default;
// ZQK_BULK_BENCH_SIZE=100 for quick profiling. Use -benchtime=1x -cpuprofile=bulk5k_create.pprof to profile.
func BenchmarkCRUDBaselineBulk5kCreateOnly(b *testing.B) {
	n := bulkBenchSize()
	proj := testkit.PrepareIsolatedTempProjectForBenchmark(b, &testkit.IsolatedTempProjectOptions{
		Kind:                      "internal.benchmark_crud_bulk5k_create_only",
		UsePlainFileObjectStorage: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "copy_specs_to_test_root",
				Fn: func() error {
					return copySpecsToInternalTestRoot(root)
				},
			}}
		},
	})
	str := proj.FileStorage

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	kind := "backlog_item"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < n; j++ {
			id := fmt.Sprintf("BLI-BULK-%d-%d", i, j)
			createCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
			obj := createTestObject(kind, id)
			if err := str.Create(createCtx, secCtx, obj); err != nil {
				b.Fatalf("create %s: %v", id, err)
			}
		}
	}
}
