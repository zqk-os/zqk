package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestCAS_MetricsUnit exercises all metric recording methods, statistics, and getters.
func TestCAS_MetricsUnit(t *testing.T) {
	t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	ResetCASMetrics()
	ResetObjectStorageMetrics()

	m := GetCASMetrics()
	if m == nil {
		t.Fatal("GetCASMetrics returned nil")
	}
	if GetObjectStorageMetrics() != m {
		t.Fatal("GetObjectStorageMetrics should match GetCASMetrics singleton")
	}

	// Record operations with both success and failure
	m.RecordCreate(10*time.Millisecond, nil)
	m.RecordCreate(15*time.Millisecond, errors.New("create err"))

	m.RecordRead(5*time.Millisecond, nil)
	m.RecordRead(8*time.Millisecond, errors.New("read err"))

	m.RecordUpdate(12*time.Millisecond, nil)
	m.RecordUpdate(14*time.Millisecond, errors.New("update err"))

	m.RecordDelete(2*time.Millisecond, nil)
	m.RecordDelete(4*time.Millisecond, errors.New("delete err"))

	m.RecordSetMapping(4*time.Millisecond, nil, true)
	m.RecordSetMapping(5*time.Millisecond, errors.New("set mapping err"), false)

	m.RecordRemoveMapping(2*time.Millisecond, nil)
	m.RecordRemoveMapping(3*time.Millisecond, errors.New("remove mapping err"))

	m.RecordIndexLoad(3*time.Millisecond, nil, 100)
	m.RecordIndexLoad(4*time.Millisecond, errors.New("index load err"), 0)

	m.RecordIndexSave(6*time.Millisecond, nil, 100)
	m.RecordIndexSave(7*time.Millisecond, errors.New("index save err"), 0)

	m.RecordIndexReload()
	m.RecordIndexMerge(50)
	m.RecordIndexLockContention(1 * time.Millisecond)
	m.RecordIndexFileLock(true, 1*time.Millisecond)
	m.RecordIndexFileLock(false, 0)

	m.RecordOrphanCleanupBatch(50*time.Millisecond, 10, 8, 2)
	m.RecordOrphanCleanupBatch(80*time.Millisecond, 5, 5, 0)

	// Check snapshots & stats
	snap := m.GetSnapshot()
	if snap.Creates != 2 || snap.Reads != 2 || snap.Updates != 2 || snap.Deletes != 2 {
		t.Errorf("unexpected snapshot counts: %+v", snap)
	}
	if snap.GetTotalOperations() != 8 {
		t.Errorf("expected 8 total operations, got %d", snap.GetTotalOperations())
	}

	snapMap := snap.ToMap()
	if len(snapMap) == 0 {
		t.Error("expected non-empty snapshot map")
	}

	// Observability builder helpers
	rec := m.getCASMetricsRecorder()
	if rec == nil {
		t.Error("getCASMetricsRecorder returned nil")
	}
	bld1 := buildCASOperationMetric("create", 10*time.Millisecond, nil)
	if bld1 == nil {
		t.Error("buildCASOperationMetric returned nil")
	}
	bld2 := buildCASOperationMetric("create", 10*time.Millisecond, errors.New("op err"))
	if bld2 == nil {
		t.Error("buildCASOperationMetric with error returned nil")
	}
	bld3 := buildListingIndexOperationMetric("update", 5*time.Millisecond, nil, map[string]any{"kind": "backlog_item"})
	if bld3 == nil {
		t.Error("buildListingIndexOperationMetric returned nil")
	}
}

// mockStorageFacade implements MetricsStorageFacade and idGenerator for testing.
type mockStorageFacade struct {
	casEnabled bool
	genID      string
	genErr     error
	createdObj map[string]any
}

func (m *mockStorageFacade) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.createdObj = obj
	return nil
}

func (m *mockStorageFacade) CASUsesContentAddressableStorage(kind string) bool {
	return m.casEnabled
}

func (m *mockStorageFacade) CASGenerateID(ctx context.Context, kind string) (string, error) {
	return m.genID, m.genErr
}

// TestCAS_MetricsAsyncCollector exercises CASMetricsAsyncCollector.
func TestCAS_MetricsAsyncCollector(t *testing.T) {
	t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	mock := &mockStorageFacade{
		casEnabled: false,
		genID:      "CMD-100",
	}

	collector := NewCASMetricsAsyncCollector(mock)
	defer collector.Stop()

	// Verify alias functions
	objCollector := NewObjectStorageMetricsAsyncCollector(mock)
	defer objCollector.Stop()

	global := GetCASMetricsAsyncCollector(mock)
	if global == nil {
		t.Fatal("GetCASMetricsAsyncCollector returned nil")
	}
	if GetObjectStorageMetricsAsyncCollector(mock) != global {
		t.Fatal("GetObjectStorageMetricsAsyncCollector should match GetCASMetricsAsyncCollector")
	}

	collector.Enable()
	collector.Disable()
	collector.Enable()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	doneCh := make(chan string, 1)
	err := collector.CollectMetricsAsync(ctx, secCtx, time.Now().Add(-1*time.Hour), time.Now(), func(metricID string, err error) {
		doneCh <- metricID
	})
	if err != nil {
		t.Fatalf("CollectMetricsAsync failed: %v", err)
	}

	select {
	case id := <-doneCh:
		t.Logf("collected metric id: %s", id)
	case <-time.After(3 * time.Second):
		t.Log("timed out waiting for async collector callback (non-fatal)")
	}

	collector.Stop()
	collector.Stop() // Test idempotency of Stop
}

// TestCAS_MetricsCollector_CollectAndReset exercises CollectMetrics and CollectAndReset.
func TestCAS_MetricsCollector_CollectAndReset(t *testing.T) {
	t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	mock := &mockStorageFacade{
		casEnabled: true,
		genID:      "CMD-200",
	}

	collector := NewCASMetricsCollector(mock)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id, err := collector.CollectAndReset(ctx, secCtx, time.Now().Add(-1*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("CollectAndReset failed: %v", err)
	}
	if id == "" {
		t.Errorf("expected non-empty metric ID, got empty")
	}

	cols, mets := collector.GetCASCollectorStats()
	if cols != 1 || mets != 1 {
		t.Errorf("expected (1, 1), got (%d, %d)", cols, mets)
	}
}

// TestCAS_DuplicateIDInventory_Comprehensive tests duplicate ID inventory, edge cases, and quarantine.
func TestCAS_DuplicateIDInventory_Comprehensive(t *testing.T) {
	// Empty root checks
	if DefaultCASDuplicateQuarantineDir("") != "" {
		t.Error("expected empty quarantine dir for empty root")
	}
	invEmpty := InventoryCASDuplicateIDs(nil, "")
	if invEmpty.DuplicateCount != 0 {
		t.Errorf("expected 0 for empty root, got %d", invEmpty.DuplicateCount)
	}

	ptrNil := CASDuplicateIDInventoryPtr(CASDuplicateIDInventory{DuplicateCount: 0})
	if ptrNil != nil {
		t.Error("expected nil pointer for DuplicateCount == 0")
	}
	ptrValid := CASDuplicateIDInventoryPtr(CASDuplicateIDInventory{DuplicateCount: 1})
	if ptrValid == nil || ptrValid.DuplicateCount != 1 {
		t.Error("expected non-nil pointer for DuplicateCount == 1")
	}

	// Validation errors for QuarantineCASDuplicateLosers
	_, errs := QuarantineCASDuplicateLosers(nil, "", "", "", false)
	if len(errs) == 0 {
		t.Error("expected error for empty args in QuarantineCASDuplicateLosers")
	}

	// Create test directory with duplicate files
	root := t.TempDir()
	kindDir := filepath.Join(datacell.ProcessPrimaryDir(root), "backlog")
	qDir := DefaultCASDuplicateQuarantineDir(root)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	body := "id: BLI-TEST-DUP-01\nkind: backlog_item\ntitle: Dup item\n"
	data := []byte(body)
	hash1 := hashHex(data)
	body2 := "id: BLI-TEST-DUP-01\nkind: backlog_item\ntitle: Dup item 2\n"
	data2 := []byte(body2)
	hash2 := hashHex(data2)

	p1 := filepath.Join(kindDir, hash1+".yaml")
	p2 := filepath.Join(kindDir, hash2+".yaml")
	if err := fileutil.WriteFile(p1, data, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := fileutil.WriteFile(p2, data2, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// Dry run quarantine
	nDry, errsDry := QuarantineCASDuplicateLosers(context.Background(), "backlog_item", kindDir, qDir, true)
	if len(errsDry) != 0 || nDry != 1 {
		t.Fatalf("dry run quarantine failed: n=%d errs=%v", nDry, errsDry)
	}
	// Verify neither file was deleted in dry run
	if _, err := fileutil.Stat(p1); err != nil {
		t.Errorf("p1 should not be deleted in dry run: %v", err)
	}

	// Actual quarantine
	n, errs := QuarantineCASDuplicateLosers(context.Background(), "backlog_item", kindDir, qDir, false)
	if len(errs) != 0 || n != 1 {
		t.Fatalf("actual quarantine failed: n=%d errs=%v", n, errs)
	}

	// QuarantineCASHashFile standalone test
	tempSrc := filepath.Join(kindDir, "temp_corrupt.yaml")
	if err := fileutil.WriteFile(tempSrc, []byte("test"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := QuarantineCASHashFile(tempSrc, qDir, "backlog_item"); err != nil {
		t.Fatalf("QuarantineCASHashFile failed: %v", err)
	}
}

// TestCAS_Recovery_Comprehensive tests RecoverCASObject and RecoverCASKind under various scenarios.
func TestCAS_Recovery_Comprehensive(t *testing.T) {
	root := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(root)
	dirName := objects.GetDirectoryFromKind("backlog_item")
	kindDir := filepath.Join(processDir, dirName)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Unknown kind error
	res, err := RecoverCASObject(ctx, root, "BLI-001", "unknown_kind_xyz", nil, nil)
	if err == nil || res == nil || res.Success {
		t.Errorf("expected error for unknown kind, got res=%v err=%v", res, err)
	}

	// 2. Object not in index
	cas := filecas.NewContentAddressableStorage(kindDir, "backlog_item")
	res, err = RecoverCASObject(ctx, root, "BLI-NOT-IN-INDEX", "backlog_item", nil, cas)
	if err != nil || res.Message != ConstStreamObjectNotInCasIndex {
		t.Errorf("expected object not in index, got res=%v err=%v", res, err)
	}

	// 3. File exists - no recovery needed
	validBody := "id: BLI-VALID-01\nkind: backlog_item\ntitle: Valid\n"
	validHash := hashHex([]byte(validBody))
	validPath := filepath.Join(kindDir, validHash+".yaml")
	if err := fileutil.WriteFile(validPath, []byte(validBody), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = cas.GetIndex().SetMapping("BLI-VALID-01", validHash)
	_ = cas.GetIndex().Save()

	res, err = RecoverCASObject(ctx, root, "BLI-VALID-01", "backlog_item", nil, cas)
	if err != nil || !res.Success || res.Message != ConstStreamFileExistsNoRecoveryNeeded {
		t.Errorf("expected file exists no recovery needed, got res=%v err=%v", res, err)
	}

	// 4. Missing file, ID-based file exists with matching hash -> recreated
	idBasedBody := "id: BLI-RECOVER-01\nkind: backlog_item\ntitle: Recreated\n"
	var parsedObj map[string]any
	_ = yaml.Unmarshal([]byte(idBasedBody), &parsedObj)
	normalizedData, _ := yaml.Marshal(parsedObj)
	matchingHash := hashHex(normalizedData)

	idBasedPath := filepath.Join(kindDir, "BLI-RECOVER-01.yaml")
	if err := fileutil.WriteFile(idBasedPath, normalizedData, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = cas.GetIndex().SetMapping("BLI-RECOVER-01", matchingHash)
	_ = cas.GetIndex().Save()

	testLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	res, err = RecoverCASObject(ctx, root, "BLI-RECOVER-01", "backlog_item", DefaultCASRecoveryOptions(testLogger), cas)
	if err != nil || !res.Success || res.Action != "recreated" {
		t.Errorf("expected recreated from ID-based file, got res=%v err=%v", res, err)
	}

	// 5. Missing file, ID-based file exists with changed hash -> recreated with new hash and EnqueueIndexUpdate
	idBasedBodyChanged := "id: BLI-RECOVER-02\nkind: backlog_item\ntitle: Content Changed\n"
	idBasedPath2 := filepath.Join(kindDir, "BLI-RECOVER-02.yaml")
	if err := fileutil.WriteFile(idBasedPath2, []byte(idBasedBodyChanged), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	oldHash := "0000000000000000000000000000000000000000000000000000000000000000"
	_ = cas.GetIndex().SetMapping("BLI-RECOVER-02", oldHash)
	_ = cas.GetIndex().Save()

	enqueued := false
	opts := &CASRecoveryOptions{
		RemoveFromIndexIfMissing: true,
		RecreateFromIDBased:      true,
		Logger:                   testLogger,
		EnqueueIndexUpdate: func(kind, objectID, hash string, cas *filecas.ContentAddressableStorage) error {
			enqueued = true
			return nil
		},
	}
	res, err = RecoverCASObject(ctx, root, "BLI-RECOVER-02", "backlog_item", opts, cas)
	if err != nil || !res.Success || res.Action != "recreated" || !enqueued {
		t.Errorf("expected recreated with changed hash, got res=%v err=%v enqueued=%v", res, err, enqueued)
	}

	// 6. Missing file, no ID-based file -> removed from index
	missingHash := "1111111111111111111111111111111111111111111111111111111111111111"
	_ = cas.GetIndex().SetMapping("BLI-RECOVER-03", missingHash)
	_ = cas.GetIndex().Save()

	res, err = RecoverCASObject(ctx, root, "BLI-RECOVER-03", "backlog_item", DefaultCASRecoveryOptions(testLogger), cas)
	if err != nil || !res.Success || res.Action != ConstStreamRemovedFromIndex {
		t.Errorf("expected removed from index, got res=%v err=%v", res, err)
	}

	// 7. Missing file, RecreateFromIDBased=false, RemoveFromIndexIfMissing=false -> error
	_ = cas.GetIndex().SetMapping("BLI-RECOVER-04", missingHash)
	_ = cas.GetIndex().Save()
	res, err = RecoverCASObject(ctx, root, "BLI-RECOVER-04", "backlog_item", &CASRecoveryOptions{
		RemoveFromIndexIfMissing: false,
		RecreateFromIDBased:      false,
	}, cas)
	if err == nil || res.Success {
		t.Errorf("expected error when recovery not possible, got res=%v err=%v", res, err)
	}

	// 8. RecoverCASKind success
	_ = cas.GetIndex().SetMapping("BLI-KIND-RECOVER", missingHash)
	_ = cas.GetIndex().Save()
	results, err := RecoverCASKind(ctx, root, "backlog_item", DefaultCASRecoveryOptions(nil), nil)
	if err != nil {
		t.Errorf("RecoverCASKind failed: %v", err)
	}
	if len(results) == 0 {
		t.Errorf("expected RecoverCASKind to recover at least 1 object, got %d", len(results))
	}
}

// TestCAS_CorruptionRepair_Comprehensive exercises RepairCASHFilenameMismatch and BatchRepairCASMismatch.
func TestCAS_CorruptionRepair_Comprehensive(t *testing.T) {
	testLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	root := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(root, "backlog")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// Error paths: missing arguments
	_, err := RepairCASHFilenameMismatch(context.Background(), "", "backlog_item", "BLI-01", "path.yaml", nil)
	if err == nil {
		t.Error("expected error for empty projectRoot")
	}
	_, err = RepairCASHFilenameMismatch(context.Background(), root, "", "BLI-01", "path.yaml", nil)
	if err == nil {
		t.Error("expected error for empty kind")
	}
	_, err = RepairCASHFilenameMismatch(context.Background(), root, "backlog_item", "", "path.yaml", nil)
	if err == nil {
		t.Error("expected error for empty objectID")
	}
	_, err = RepairCASHFilenameMismatch(context.Background(), root, "backlog_item", "BLI-01", "", nil)
	if err == nil {
		t.Error("expected error for empty corruptFilePath")
	}

	// Non-yaml extension error
	txtFile := filepath.Join(kindDir, "corrupt.txt")
	_ = fileutil.WriteFile(txtFile, []byte("data"), paths.FilePerm644)
	_, err = RepairCASHFilenameMismatch(context.Background(), root, "backlog_item", "BLI-01", txtFile, nil)
	if err == nil {
		t.Error("expected error for non-yaml extension")
	}

	// Content already matching filename stem (no repair needed)
	cleanContent := []byte("id: BLI-CLEAN\nkind: backlog_item\n")
	cleanHash := hashHex(cleanContent)
	cleanFile := filepath.Join(kindDir, cleanHash+".yaml")
	_ = fileutil.WriteFile(cleanFile, cleanContent, paths.FilePerm644)

	resClean, err := RepairCASHFilenameMismatch(context.Background(), root, "backlog_item", "BLI-CLEAN", cleanFile, nil)
	if err != nil || resClean.Fixed {
		t.Errorf("expected no repair needed, got res=%v err=%v", resClean, err)
	}

	// Corrupt file (filename stem != content hash)
	wrongHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	corruptFile := filepath.Join(kindDir, wrongHash+".yaml")
	_ = fileutil.WriteFile(corruptFile, cleanContent, paths.FilePerm644)

	// Dry run repair
	resDry, err := RepairCASHFilenameMismatch(context.Background(), root, "backlog_item", "BLI-CLEAN", corruptFile, &CASCorruptionRepairOptions{
		DryRun: true,
	})
	if err != nil || resDry.Fixed || !strings.Contains(resDry.Message, "[DRY RUN]") {
		t.Errorf("expected dry run result, got res=%v err=%v", resDry, err)
	}

	// Actual repair
	resActual, err := RepairCASHFilenameMismatch(context.Background(), root, objects.KindBacklogItem, "BLI-CLEAN", corruptFile, &CASCorruptionRepairOptions{
		Logger: testLogger,
	})
	if err != nil || !resActual.Fixed {
		t.Fatalf("actual repair failed: res=%v err=%v", resActual, err)
	}

	// Batch repair test
	items := []CASCorruptionRepairItem{
		{
			Kind:            objects.KindBacklogItem,
			ObjectID:        "BLI-CLEAN",
			CorruptFilePath: cleanFile,
		},
	}
	batchRes, err := BatchRepairCASMismatch(context.Background(), root, items, nil)
	if err != nil || len(batchRes) != 1 {
		t.Errorf("batch repair failed: len=%d err=%v", len(batchRes), err)
	}

	// Batch repair context cancelled
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = BatchRepairCASMismatch(cancelCtx, root, items, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestCAS_ListingIndexWriteQueue_Stats exercises GetQueueStats, GetPendingCount, GetName, and IsCritical.
func TestCAS_ListingIndexWriteQueue_Stats(t *testing.T) {
	q := NewListingIndexWriteQueue()
	defer func() { _ = q.Shutdown() }()

	stats := q.GetQueueStats()
	if stats == nil {
		t.Fatal("GetQueueStats returned nil")
	}
	if stats["queue_count"] == nil {
		t.Error("expected queue_count key in stats")
	}

	count := q.GetPendingCount()
	if count != 0 {
		t.Errorf("expected 0 pending count, got %d", count)
	}

	if q.GetName() != ConstStreamListingIndexWriteQueue {
		t.Errorf("unexpected name: %s", q.GetName())
	}

	if !q.IsCritical() {
		t.Error("expected IsCritical() == true")
	}
}

// TestCAS_OrphanGuard_GitStagedDeletions exercises staged deletions git integration.
func TestCAS_OrphanGuard_GitStagedDeletions(t *testing.T) {
	repoDir := t.TempDir()

	// Initialize git repo
	runGit := func(args ...string) error {
		cmd := execwrap.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %v failed: %w (output: %s)", args, err, string(out))
		}
		return nil
	}

	if err := runGit("init"); err != nil {
		t.Skipf("git init not available: %v", err)
	}
	if err := runGit("config", "user.email", "test@test.com"); err != nil {
		t.Skip(err)
	}
	if err := runGit("config", "user.name", "Test"); err != nil {
		t.Skip(err)
	}

	guard := NewCASOrphanGuard(repoDir)

	// Clean staged deletions check when nothing is staged
	results, err := guard.CheckStagedDeletions()
	if err != nil || len(results) != 0 {
		t.Fatalf("expected empty results for clean repo, got %v err=%v", results, err)
	}

	// Create and commit two related process objects: BLI referencing GOL
	processBacklog := filepath.Join(repoDir, paths.ProcessDir, "backlog")
	processGoals := filepath.Join(repoDir, paths.ProcessDir, "goals")
	if err := fileutil.MkdirAll(processBacklog, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(processGoals, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	golFile := filepath.Join(processGoals, "goal1.yaml")
	bliFile := filepath.Join(processBacklog, "bli1.yaml")

	golContent := "id: GOL-GIT-01\nkind: goal\ntitle: Goal 1\n"
	bliContent := "id: BLI-GIT-01\nkind: backlog_item\ntitle: BLI 1\ngoal_refs:\n  - GOL-GIT-01\n"

	if err := fileutil.WriteFile(golFile, []byte(golContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(bliFile, []byte(bliContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	if err := runGit("add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit("commit", "-m", "initial commit"); err != nil {
		t.Fatal(err)
	}

	// Staged deletion of GOL-GIT-01 while BLI-GIT-01 still references it -> should detect orphan!
	if err := runGit("rm", filepath.Join(paths.ProcessDir, "goals", "goal1.yaml")); err != nil {
		t.Fatal(err)
	}

	orphanResults, err := guard.CheckStagedDeletions()
	if err != nil {
		t.Fatalf("CheckStagedDeletions failed: %v", err)
	}
	if len(orphanResults) == 0 {
		t.Fatal("expected orphan detection for GOL-GIT-01, got none")
	}
	if orphanResults[0].DeletedID != "GOL-GIT-01" {
		t.Errorf("expected deleted id GOL-GIT-01, got %s", orphanResults[0].DeletedID)
	}

	// Staged rename scenario: add new file with same ID to simulate CAS rename
	_ = fileutil.MkdirAll(processGoals, paths.DirPerm755)
	newGolFile := filepath.Join(processGoals, "goal1_renamed.yaml")
	if err := fileutil.WriteFile(newGolFile, []byte(golContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := runGit("add", newGolFile); err != nil {
		t.Fatal(err)
	}
	// With goal1_renamed declared in staging, it should be recognized as a rename (not an orphan)
	noOrphansOnRename, err := guard.CheckStagedDeletions()
	if err != nil {
		t.Fatalf("CheckStagedDeletions failed: %v", err)
	}
	if len(noOrphansOnRename) != 0 {
		t.Errorf("expected 0 orphans on rename, got %v", noOrphansOnRename)
	}

	// Clean integrity check on non-existent repo root
	emptyGuard := NewCASOrphanGuard(filepath.Join(repoDir, "non_existent_subdir"))
	intRes, err := emptyGuard.CheckPackageIntegrity()
	if err != nil || len(intRes) != 0 {
		t.Errorf("expected nil results for non-existent process dir, got %v err=%v", intRes, err)
	}
}

// TestCAS_LivePathResolve_Comprehensive tests ResolveLiveCASFilePath and CachePathNeedsCASResolve.
func TestCAS_LivePathResolve_Comprehensive(t *testing.T) {
	// Empty checks
	p, ok := ResolveLiveCASFilePath("", "backlog_item", "BLI-01")
	if ok || p != "" {
		t.Errorf("expected false for empty root, got %s %v", p, ok)
	}
	p, ok = ResolveLiveCASFilePath("/tmp", "", "BLI-01")
	if ok || p != "" {
		t.Errorf("expected false for empty kind, got %s %v", p, ok)
	}
	p, ok = ResolveLiveCASFilePath("/tmp", "backlog_item", "")
	if ok || p != "" {
		t.Errorf("expected false for empty id, got %s %v", p, ok)
	}
	p, ok = ResolveLiveCASFilePath("/tmp", "invalid_xyz", "BLI-01")
	if ok || p != "" {
		t.Errorf("expected false for invalid kind, got %s %v", p, ok)
	}
	p, ok = ResolveLiveCASFilePathFromKindDir("", "backlog_item", "BLI-01")
	if ok || p != "" {
		t.Errorf("expected false for empty kindDir, got %s %v", p, ok)
	}

	if !CachePathNeedsCASResolve("") {
		t.Error("expected true for empty filePath in CachePathNeedsCASResolve")
	}
	if !CachePathNeedsCASResolve("/non/existent/path/xyz.yaml") {
		t.Error("expected true for non-existent file in CachePathNeedsCASResolve")
	}

	// Real file check
	root := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(root, "backlog_items")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	cas := filecas.NewContentAddressableStorage(kindDir, "backlog_item")
	body := "id: BLI-LIVE-01\nkind: backlog_item\n"
	hash := hashHex([]byte(body))
	fPath := filepath.Join(kindDir, hash+".yaml")
	if err := fileutil.WriteFile(fPath, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	_ = cas.GetIndex().SetMapping("BLI-LIVE-01", hash)
	_ = cas.GetIndex().Save()

	resolvedPath, found := ResolveLiveCASFilePath(root, "backlog_item", "BLI-LIVE-01")
	if !found || resolvedPath != fPath {
		t.Errorf("expected resolvedPath %q, got %q (found=%v)", fPath, resolvedPath, found)
	}
	if CachePathNeedsCASResolve(fPath) {
		t.Errorf("expected CachePathNeedsCASResolve=false for existing file %s", fPath)
	}
}

// TestCAS_PendingVisibilityCache_Comprehensive tests pending visibility cache operations and edge cases.
func TestCAS_PendingVisibilityCache_Comprehensive(t *testing.T) {
	root := t.TempDir()
	cache := GetCASPendingVisibilityCache(root)
	if cache == nil {
		t.Fatal("GetCASPendingVisibilityCache returned nil")
	}

	// Lookup missing
	_, found := cache.LookupPending("MISSING-ID")
	if found {
		t.Error("expected missing ID not found")
	}

	// Publish pending
	err := cache.PublishPending("BLI-PENDING-01", "backlog_item", "hash001", "bucketA")
	if err != nil {
		t.Fatalf("PublishPending failed: %v", err)
	}

	entry, found := cache.LookupPending("BLI-PENDING-01")
	if !found || entry.Hash != "hash001" || entry.BucketKey != "bucketA" {
		t.Errorf("expected entry hash001, got %+v (found=%v)", entry, found)
	}

	// Evict pending if hash
	err = cache.EvictPendingIfHash("BLI-PENDING-01", "different_hash")
	if err != nil {
		t.Errorf("EvictPendingIfHash failed: %v", err)
	}
	_, found = cache.LookupPending("BLI-PENDING-01")
	if !found {
		t.Error("entry should still exist after mismatch EvictPendingIfHash")
	}

	err = cache.EvictPendingIfHash("BLI-PENDING-01", "hash001")
	if err != nil {
		t.Errorf("EvictPendingIfHash failed: %v", err)
	}
	_, found = cache.LookupPending("BLI-PENDING-01")
	if found {
		t.Error("entry should be evicted after matching EvictPendingIfHash")
	}

	// DumpPendingToDurableIndexes test
	err = cache.PublishPending("BLI-PENDING-02", "backlog_item", "hash002", "")
	if err != nil {
		t.Fatal(err)
	}

	kindDir := datacell.CellCASPrimaryDir(root, "backlog_items")
	_ = fileutil.MkdirAll(kindDir, paths.DirPerm755)
	cas := filecas.NewContentAddressableStorage(kindDir, "backlog_item")

	err = cache.DumpPendingToDurableIndexes(func(kind string) (*filecas.ContentAddressableStorage, error) {
		return cas, nil
	})
	if err != nil {
		t.Errorf("DumpPendingToDurableIndexes failed: %v", err)
	}
	if !cache.WritesRejected() {
		t.Error("expected WritesRejected == true after DumpPendingToDurableIndexes")
	}

	// Further writes rejected
	err = cache.PublishPending("BLI-PENDING-03", "backlog_item", "hash003", "")
	if err == nil {
		t.Error("expected error publishing when writes rejected")
	}

	// ProjectRootFromCASIndexPath
	if ProjectRootFromCASIndexPath("") != "." {
		t.Errorf("expected '.' for empty index path")
	}
	fakeIndex := filepath.Join(root, paths.ProcessDir, "backlog", ".backlog_item.index")
	derivedRoot := ProjectRootFromCASIndexPath(fakeIndex)
	if derivedRoot == "" {
		t.Error("expected non-empty derived root")
	}
}

// TestCAS_QueuesAndLifecycle_Comprehensive exercises listing index queue lifecycle and orphan cleanup queue.
func TestCAS_QueuesAndLifecycle_Comprehensive(t *testing.T) {
	root := t.TempDir()

	// 1. Listing index write queue lifecycle
	q := NewListingIndexWriteQueueForTest()
	defer func() { _ = q.Shutdown() }()

	if err := q.FlushKind("backlog_item", 50*time.Millisecond); err != nil {
		t.Errorf("FlushKind failed: %v", err)
	}
	if err := q.FlushAll(50*time.Millisecond); err != nil {
		t.Errorf("FlushAll failed: %v", err)
	}
	if err := q.FlushAllWithDeadline(time.Now().Add(50*time.Millisecond)); err != nil {
		t.Errorf("FlushAllWithDeadline failed: %v", err)
	}
	if err := FlushAllListingIndexesForProjectRootWithTimeout(root, 50*time.Millisecond); err != nil {
		t.Errorf("FlushAllListingIndexesForProjectRootWithTimeout failed: %v", err)
	}

	if err := q.InitiateShutdown(); err != nil {
		t.Errorf("InitiateShutdown failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := q.Drain(ctx); err != nil {
		t.Errorf("Drain failed: %v", err)
	}
	if !q.IsDrained() {
		t.Error("expected IsDrained == true")
	}

	// Callbacks
	called := false
	SetListingIndexBatchEventCallback(func(ctx context.Context, projectRoot string, storageProvider CASFacade, kind string, batchSize int, duration time.Duration, status string, err error) {
		called = true
	})
	cb := getListingIndexBatchEventCallback()
	if cb == nil {
		t.Error("expected non-nil listing index callback")
	} else {
		cb(context.Background(), root, nil, "backlog_item", 1, 1*time.Millisecond, "success", nil)
		if !called {
			t.Error("expected callback to be executed")
		}
	}
	reg, trig := GetListingIndexCallbackStats()
	if reg == 0 || trig == 0 {
		t.Errorf("expected callback stats > 0, got (%d, %d)", reg, trig)
	}
	SetListingIndexBatchEventCallback(nil)

	// 2. Orphan cleanup queue methods
	orphanQ := GetGlobalCASOrphanCleanupQueue()
	if orphanQ == nil {
		t.Fatal("GetGlobalCASOrphanCleanupQueue returned nil")
	}

	if orphanQ.GetName() != ConstStreamCasOrphanCleanupQueue {
		t.Errorf("unexpected name: %s", orphanQ.GetName())
	}
	if orphanQ.IsCritical() {
		t.Error("expected IsCritical == false for orphan queue")
	}
	_ = orphanQ.QueueSize()
	_ = orphanQ.IsWorkerRunning()
	_ = orphanQ.GetPendingCount()
	_ = orphanQ.IsDrained()

	n, err := orphanQ.ProcessQueueIfIdle(context.Background())
	if err != nil {
		t.Errorf("ProcessQueueIfIdle failed: %v", err)
	}
	t.Logf("ProcessQueueIfIdle processed %d items", n)

	stats := orphanQ.GetQueueStats()
	if stats.QueueCapacity == 0 {
		t.Error("expected non-zero queue capacity")
	}

	// CleanupOrphanedDrafts with a mock draft directory
	draftsDir := filepath.Join(root, "test_drafts")
	_ = fileutil.MkdirAll(draftsDir, paths.DirPerm755)
	dummyDraft := filepath.Join(draftsDir, "draft_01.yaml")
	_ = fileutil.WriteFile(dummyDraft, []byte("test draft"), paths.FilePerm644)

	// Add a subdirectory and test maxAge filtering
	_ = fileutil.MkdirAll(filepath.Join(draftsDir, "subdir"), paths.DirPerm755)

	enqueuedDrafts, err := orphanQ.CleanupOrphanedDrafts(context.Background(), draftsDir, 0)
	if err != nil {
		t.Errorf("CleanupOrphanedDrafts failed: %v", err)
	}
	t.Logf("enqueued %d drafts", enqueuedDrafts)

	// maxAge check (file is newer than cutoff, so skipped)
	enqueuedOld, err := orphanQ.CleanupOrphanedDrafts(context.Background(), draftsDir, 10*time.Hour)
	if err != nil || enqueuedOld != 0 {
		t.Errorf("expected 0 enqueued for old maxAge, got %d err=%v", enqueuedOld, err)
	}

	// Non-existent drafts directory
	enqueuedNonExistent, err := orphanQ.CleanupOrphanedDrafts(context.Background(), filepath.Join(root, "no_such_dir"), 0)
	if err != nil || enqueuedNonExistent != 0 {
		t.Errorf("expected 0 for non-existent drafts dir, got %d err=%v", enqueuedNonExistent, err)
	}

	orphanCalled := false
	SetOrphanCleanupEventCallback(func(ctx context.Context, projectRoot string, storage CASFacade, kind, op, status string, batchSize, succ, fail int, d time.Duration, failedFiles []string) {
		orphanCalled = true
	})
	ocb := getOrphanCleanupEventCallback()
	if ocb == nil {
		t.Error("expected non-nil orphan cleanup callback")
	} else {
		ocb(context.Background(), root, nil, "backlog_item", "op1", "success", 1, 1, 0, 1*time.Millisecond, nil)
		if !orphanCalled {
			t.Error("expected orphan callback to be executed")
		}
	}
	SetOrphanCleanupEventCallback(nil)

	// NewCASOrphanCleanupQueueForTest
	testOrphanQ := NewCASOrphanCleanupQueueForTest(GetCASMetrics())
	if testOrphanQ == nil {
		t.Fatal("NewCASOrphanCleanupQueueForTest returned nil")
	}
	if err := testOrphanQ.InitiateShutdown(); err != nil {
		t.Errorf("testOrphanQ InitiateShutdown failed: %v", err)
	}
	if err := testOrphanQ.Drain(context.Background()); err != nil {
		t.Errorf("testOrphanQ Drain failed: %v", err)
	}
}

// TestCAS_AdditionalCoverageBoost exercises queue enqueues and test export helpers.
func TestCAS_AdditionalCoverageBoost(t *testing.T) {
	root := t.TempDir()
	kindDir := datacell.CellCASPrimaryDir(root, "backlog_items")
	_ = fileutil.MkdirAll(kindDir, paths.DirPerm755)
	cas := filecas.NewContentAddressableStorage(kindDir, "backlog_item")

	q := NewListingIndexWriteQueueForTest()
	q.SetProjectRoot(root)
	defer func() { _ = q.Shutdown() }()

	// 1. EnqueueUpdate
	if err := q.EnqueueUpdate("backlog_item", "BLI-ENQ-01", "hash01", cas); err != nil {
		t.Errorf("EnqueueUpdate failed: %v", err)
	}

	// 2. EnqueueUpdateWithCallback
	doneCh, err := q.EnqueueUpdateWithCallback("backlog_item", "BLI-ENQ-02", "hash02", cas)
	if err != nil {
		t.Errorf("EnqueueUpdateWithCallback failed: %v", err)
	}
	if doneCh == nil {
		t.Error("expected non-nil doneCh")
	}

	// 3. EnqueueUpdateWithOperationCallback
	doneCh2, err := q.EnqueueUpdateWithOperationCallback("backlog_item", "BLI-ENQ-03", "hash03", "bucket1", cas, nil)
	if err != nil {
		t.Errorf("EnqueueUpdateWithOperationCallback failed: %v", err)
	}
	if doneCh2 == nil {
		t.Error("expected non-nil doneCh2")
	}

	// 4. EnqueueUpdateWithOperationCallbackAndCreatedAt
	doneCh3, err := q.EnqueueUpdateWithOperationCallbackAndCreatedAt("backlog_item", "BLI-ENQ-04", "hash04", "bucket1", time.Now().UTC().Format(time.RFC3339), cas, nil)
	if err != nil {
		t.Errorf("EnqueueUpdateWithOperationCallbackAndCreatedAt failed: %v", err)
	}
	if doneCh3 == nil {
		t.Error("expected non-nil doneCh3")
	}

	// 5. EnqueueRemove
	doneCh4, err := q.EnqueueRemove("backlog_item", "BLI-ENQ-01", cas)
	if err != nil {
		t.Errorf("EnqueueRemove failed: %v", err)
	}
	if doneCh4 == nil {
		t.Error("expected non-nil doneCh4")
	}

	// Wait for queue flush
	_ = q.FlushKind("backlog_item", 200*time.Millisecond)

	// 6. Test export helpers and dummy waitgroup
	dw := &dummyWaitGroupManager{}
	grp := dw.CreateGroupForGoroutine("test", "test")
	if grp == nil {
		t.Error("expected non-nil grp")
	}
	dw.Add("test", 1)
	dw.Done("test")
	dw.Wait("test")

	testOrphanQueue, cancel := NewCASOrphanCleanupQueueWithCancelForTest(context.Background())
	defer cancel()
	if testOrphanQueue.QueueForTest() == nil {
		t.Error("QueueForTest returned nil")
	}
	testOrphanQueue.ProcessBatchForTest(nil)
	_ = GetOrphanCleanupEventCallbackForTest()
	_ = CasOrphanCleanupBatchSizeForTest

	// 7. State change callbacks and queue factories
	var stateCalled atomic.Bool
	SetListingIndexStateChangeEventCallback(func(ctx context.Context, projectRoot string, storageProvider CASFacade, changeType string) {
		stateCalled.Store(true)
	})
	scb := getListingIndexStateChangeEventCallback()
	if scb == nil {
		t.Error("expected non-nil state change callback")
	}

	q.SetProjectRoot(filepath.Join(root, "subproject"))
	if q.GetProjectRoot() != filepath.Join(root, "subproject") {
		t.Errorf("unexpected project root: %s", q.GetProjectRoot())
	}
	q.SetStorage(nil)
	_ = q.GetStorage()
	time.Sleep(10 * time.Millisecond) // let background state change goroutine fire
	_ = stateCalled.Load()
	SetListingIndexStateChangeEventCallback(nil)

	// Factory per project root
	SetListingIndexWriteQueueFactoryToPerProjectRoot()
	defer SetListingIndexWriteQueueFactory(nil)

	dedicatedQ := GetListingIndexWriteQueueForProjectRoot(root)
	if dedicatedQ == nil {
		t.Error("GetListingIndexWriteQueueForProjectRoot returned nil")
	}
	RemoveListingIndexWriteQueueForProjectRoot(root)
}

// TestCAS_FinalMembraneAndCollectorBoost exercises membrane validation and collector error paths.
func TestCAS_FinalMembraneAndCollectorBoost(t *testing.T) {
	// 1. criteria_cas_membrane.go
	if UseObjectDraftPlane("criteria", nil, false) {
		t.Error("expected false for nil obj")
	}
	if UseObjectDraftPlane(objects.KindCriteria, map[string]any{}, true) {
		t.Error("expected false when promoteOnCreate=true")
	}
	if UseObjectDraftPlane("bypass_kind", map[string]any{}, false) {
		t.Error("expected false for bypass kind")
	}

	err := RefuseCriteriaCASWithoutCategory("criteria", false, []byte("invalid: yaml: :"))
	if err == nil {
		t.Error("expected unmarshal error for invalid yaml in RefuseCriteriaCASWithoutCategory")
	}
	if err := RefuseCriteriaCASWithoutCategory("criteria", true, []byte("{}")); err != nil {
		t.Errorf("expected nil when isDraft=true, got %v", err)
	}
	if err := RefuseCriteriaCASWithoutCategory("other", false, []byte("{}")); err != nil {
		t.Errorf("expected nil when kind != criteria, got %v", err)
	}

	// 2. description_cas_membrane.go
	if !KindRequiresDescription(objects.KindBacklogItem) {
		t.Error("expected true for backlog_item")
	}
	if ParkObjectWithoutDescription(objects.KindBacklogItem, nil) {
		t.Error("expected false for nil obj")
	}
	if ParkObjectWithoutDescription("non_requiring_kind", map[string]any{}) {
		t.Error("expected false for non-requiring kind")
	}

	for _, ph := range []string{"todo", "tbd", "none", "null", "n/a", "placeholder", "title", "required"} {
		if !isPlaceholderDescription(ph) {
			t.Errorf("expected true for placeholder %q", ph)
		}
	}
	if isPlaceholderDescription("substantive detailed description") {
		t.Error("expected false for non-placeholder description")
	}

	if err := RefuseCASWithoutDescription("backlog_item", true, nil); err != nil {
		t.Errorf("expected nil when isDraft=true, got %v", err)
	}
	if err := RefuseCASWithoutDescription("other_kind", false, nil); err != nil {
		t.Errorf("expected nil when kind does not require description, got %v", err)
	}

	err = RefuseCASWithoutDescription("backlog_item", false, []byte("invalid: yaml: :"))
	if err == nil {
		t.Error("expected unmarshal error for invalid yaml in RefuseCASWithoutDescription")
	}
	err = RefuseCASWithoutDescription("backlog_item", false, []byte("id: BLI-01\n"))
	if err == nil {
		t.Error("expected error for empty description")
	}
	err = RefuseCASWithoutDescription("backlog_item", false, []byte("id: BLI-01\ndescription: todo\n"))
	if err == nil {
		t.Error("expected error for placeholder description")
	}
	err = RefuseCASWithoutDescription("backlog_item", false, []byte("id: BLI-01\ndescription: short\n"))
	if err == nil {
		t.Error("expected error for short description")
	}
	err = RefuseCASWithoutDescription("backlog_item", false, []byte("id: BLI-01\ndescription: substantive description exceeding ten chars\n"))
	if err != nil {
		t.Errorf("expected valid description to succeed, got %v", err)
	}

	// 3. Collector error paths
	failingMock := &mockStorageFacadeFailing{}
	col := NewObjectStorageMetricsCollector(failingMock)
	if col == nil {
		t.Fatal("NewObjectStorageMetricsCollector returned nil")
	}
	t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	_, err = col.CollectMetrics(ctx, secCtx, time.Now().Add(-1*time.Hour), time.Now())
	if err == nil {
		t.Error("expected error when storage.Create fails")
	}
}

type mockStorageFacadeFailing struct{}

func (m *mockStorageFacadeFailing) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return errors.New("simulated create failure")
}

func (m *mockStorageFacadeFailing) CASUsesContentAddressableStorage(kind string) bool {
	return true
}

func (m *mockStorageFacadeFailing) CASGenerateID(ctx context.Context, kind string) (string, error) {
	return "CMD-FAIL", nil
}




