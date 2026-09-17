package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestPostRetentionStreamStewardshipFiltered_emptyFilterRunsNoKinds(t *testing.T) {
	tmp := t.TempDir()
	logger := logging.GetLoggerFromProfile("system")
	// Non-nil empty filter: no kinds stewarded (dedicated job with unknown KINDS).
	PostRetentionStreamStewardshipFiltered(tmp, "filter-empty", logger, map[string]bool{})
}

func TestPostRetentionStreamStewardship_emptyRoot(t *testing.T) {
	tmp := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PostRetentionStreamStewardship(tmp, "test-cycle", logger)
}

func TestPostRetentionStreamStewardship_nilLoggerNoop(t *testing.T) {
	PostRetentionStreamStewardship(t.TempDir(), "c", nil)
}

func TestPostRetentionStreamStewardship_emptyProjectRootNoop(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PostRetentionStreamStewardship("", "c", logger)
}

// TestPostRetentionStreamStewardship_enqueuesStewardMaintenanceJSONL pins the data-cell housekeeping
// contract: after segment/registry/delta maintenance runs, one stream_steward_kind record per stream
// kind (and per runtime-delta kind when configured) for envelope-tick drain; real work already ran above.
func TestPostRetentionStreamStewardship_enqueuesStewardMaintenanceJSONL(t *testing.T) {
	tmp := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	cycle := "steward-enqueue-contract"
	PostRetentionStreamStewardship(tmp, cycle, logger)
	path := datacell.StewardEnqueueJSONLPath(tmp)
	b, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read steward enqueue jsonl: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, datacell.MaintenanceOpStreamStewardKind) {
		t.Fatalf("jsonl should record stream_steward_kind op; got %q", s)
	}
	if !strings.Contains(s, cycle) {
		t.Fatalf("jsonl should include cycle id in detail; want substring %q in %q", cycle, s)
	}
	if !strings.Contains(s, string(datacell.ProfileStream)) {
		t.Fatalf("jsonl should record stream profile; got %q", s)
	}
	streamKinds := StreamStorageEnabledKindsList()
	rtKinds := RuntimeDeltaEnabledKindsList(tmp)
	wantLines := len(streamKinds) + len(rtKinds)
	nonEmpty := 0
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	if nonEmpty != wantLines {
		t.Fatalf("want %d steward enqueue lines (stream kinds + runtime-delta kinds), got %d", wantLines, nonEmpty)
	}
}

func TestRunPostRetentionStreamKindStewardship_emptyTree(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	gc, err := RunPostRetentionStreamKindStewardship(tmp, objects.KindAuditEvent)
	if err != nil {
		t.Fatalf("unexpected error on empty registry: %v", err)
	}
	if gc.Kind != objects.KindAuditEvent {
		t.Fatalf("result kind: got %q", gc.Kind)
	}
}

// TestPostRetentionStreamStewardship_removesOrphanSegmentFixture lays down a minimal stream segment
// tree under t.TempDir() only (orphan file, no registry references), then asserts
// PostRetentionStreamStewardship removes that file via the same compact→GC sequence production uses.
// Pre-creates .zqk/state so CompactStreamRegistryForKind succeeds; otherwise stewardship skips GC per kind.
func TestPostRetentionStreamStewardship_removesOrphanSegmentFixture(t *testing.T) {
	root := t.TempDir()

	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	kind := objects.KindAuditAggregationMetric
	segDir := setupTestSegmentDir(t, root, kind)

	past := time.Now().UTC().AddDate(0, 0, -90).Format("2006-01-02")
	orphanFile := writeSegmentFile(t, segDir, past+"_stream.json")

	nBefore := countRegularFilesInDir(t, segDir)
	if nBefore != 1 {
		t.Fatalf("want 1 segment file before stewardship, got %d", nBefore)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PostRetentionStreamStewardship(root, "fixture-orphan-gc", logger)

	if _, err := fileutil.Stat(orphanFile); !fileutil.IsNotExist(err) {
		t.Fatalf("orphan segment should be removed; stat err=%v path=%s", err, orphanFile)
	}
	nAfter := countRegularFilesInDir(t, segDir)
	if nAfter != 0 {
		t.Fatalf("segment dir should be empty after orphan GC; got %d files", nAfter)
	}
}

// TestPostRetentionStreamStewardship_softDeletedRegistryCompactionAndGC_fixture mirrors the
// registry+segment contract from TestGCOrphanedStreamSegments_SoftDeletedIDsStillOrphaned but
// drives it only through PostRetentionStreamStewardship (compact then GC per kind). Two segment
// files: one referenced by a live registry entry (kept), one pointed to only by a soft-deleted ID
// (removed after compact drops it from the live set). Isolated t.TempDir() only.
func TestPostRetentionStreamStewardship_softDeletedRegistryCompactionAndGC_fixture(t *testing.T) {
	root := t.TempDir()

	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	kind := objects.KindAuditAggregationMetric
	segDir := setupTestSegmentDir(t, root, kind)

	now := time.Now().UTC()
	deadDate := now.AddDate(0, 0, -120).Format("2006-01-02")
	liveDate := now.AddDate(0, 0, -30).Format("2006-01-02")

	deadFile := writeSegmentFile(t, segDir, deadDate+"_stream.json")
	liveFile := writeSegmentFile(t, segDir, liveDate+"_stream.json")

	deadID := "AAM-deleted-steward-001"
	liveID := "AAM-live-steward-001"
	if err := AppendStreamLocationToRegistry(root, kind, deadID, FormatStreamLocation(deadFile, 0)); err != nil {
		t.Fatalf("AppendStreamLocationToRegistry dead: %v", err)
	}
	if err := AppendStreamLocationToRegistry(root, kind, liveID, FormatStreamLocation(liveFile, 0)); err != nil {
		t.Fatalf("AppendStreamLocationToRegistry live: %v", err)
	}
	if err := AddStreamDeletedID(root, kind, deadID); err != nil {
		t.Fatalf("AddStreamDeletedID: %v", err)
	}

	if n := countRegularFilesInDir(t, segDir); n != 2 {
		t.Fatalf("want 2 segment files before stewardship, got %d", n)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PostRetentionStreamStewardship(root, "fixture-soft-delete-gc", logger)

	if _, err := fileutil.Stat(deadFile); !fileutil.IsNotExist(err) {
		t.Fatalf("soft-deleted segment file should be removed after compact+GC; stat err=%v path=%s", err, deadFile)
	}
	if _, err := fileutil.Stat(liveFile); err != nil {
		t.Fatalf("live segment file should remain: %v", err)
	}
	if n := countRegularFilesInDir(t, segDir); n != 1 {
		t.Fatalf("want 1 segment file after stewardship, got %d", n)
	}
}

func countRegularFilesInDir(t *testing.T, dir string) int {
	t.Helper()
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

// TestPostRetentionStreamStewardship_runtimeDeltaOverlayGC_fixture registers runtime_delta for
// scheduler_job, writes a ghost overlay (no stream/CAS object), and asserts PostRetentionStreamStewardship
// removes it via GCRuntimeDeltaCurrentForKind. Uses MustEnsureProcessSpecsLayout so Exists() can infer
// kind from SCH- ID; temp dir only.
func TestPostRetentionStreamStewardship_runtimeDeltaOverlayGC_fixture(t *testing.T) {
	root := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, root)
	WriteRuntimeDeltaKindsConfigForTest(t, root)

	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	kind := objects.KindSchedulerJob
	overlayDir := filepath.Join(stateDir, RuntimeDeltaCurrentDirNameForTest, kind)
	if err := fileutil.MkdirAll(overlayDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir runtime_delta_current: %v", err)
	}
	ghostID := "SCH-GHOST-STEWARD-RT-001"
	p := filepath.Join(overlayDir, ghostID+".yaml")
	if err := fileutil.WriteFile(p, []byte("id: "+ghostID+"\nkind: scheduler_job\nstatus: active\n"), paths.FilePerm600); err != nil {
		t.Fatalf("write ghost overlay: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PostRetentionStreamStewardship(root, "fixture-runtime-delta-overlay-gc", logger)

	if _, err := fileutil.Stat(p); !fileutil.IsNotExist(err) {
		t.Fatalf("ghost runtime_delta overlay should be removed; stat err=%v path=%s", err, p)
	}
}

// TestPostRetentionStreamStewardship_runtimeDeltaBackfill_fixture creates a stream-backed scheduler_job
// (runtime_delta fields present, no overlay yet), runs PostRetentionStreamStewardship, and asserts
// BackfillRuntimeDeltaCurrentForKind wrote runtime_delta_current/<id>.yaml. Temp project only.
func TestPostRetentionStreamStewardship_runtimeDeltaBackfill_fixture(t *testing.T) {
	root := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, root)
	if err := fileutil.MkdirAll(filepath.Join(root, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	WriteRuntimeDeltaKindsConfigForTest(t, root)

	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	fos, err := NewFileObjectStorageForTest(root)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(root, fos)
		_ = RunProjectTestTeardown(opts)
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	jobID := "SCH-STEWARD-BACKFILL-001"
	job := map[string]any{
		objects.FieldKeyID: jobID, objects.FieldKeyKind: objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "Steward backfill",
		objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeyJobType: "cache_prewarm", objects.FieldKeyTriggerType: "timer",
		objects.FieldKeyScheduleExpression: "*/5 * * * *", objects.FieldKeyCategory: "maintenance",
		objects.FieldKeyExecutionMode: "reusable", objects.FieldKeyEnabled: true,
		objects.FieldKeyCreatedAt: "2030-01-01T00:00:00Z", objects.FieldKeyCreatedBy: "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject: "zqk", objects.FieldKeyOriginSystem: "zqk",
	}
	ctx := pkgctx.NewSystemContext()
	if err := fos.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create scheduler_job: %v", err)
	}

	kind := objects.KindSchedulerJob
	p := filepath.Join(stateDir, RuntimeDeltaCurrentDirNameForTest, kind, jobID+".yaml")
	if _, err := fileutil.Stat(p); err == nil {
		t.Fatal("overlay should not exist before stewardship backfill")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PostRetentionStreamStewardship(root, "fixture-runtime-delta-backfill", logger)

	if _, err := fileutil.Stat(p); err != nil {
		t.Fatalf("runtime_delta overlay should exist after backfill; stat err=%v path=%s", err, p)
	}
}
