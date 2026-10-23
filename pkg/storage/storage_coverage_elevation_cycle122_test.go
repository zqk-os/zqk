package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// setupCycle122IsolatedEnv establishes clean directory bounds for test runs.
func setupCycle122IsolatedEnv(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	return tmpDir
}

func TestCycle122_WALFacade_Basics(t *testing.T) {
	tmpDir := setupCycle122IsolatedEnv(t)

	walObj, err := AcquireObjectWAL(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, walObj)

	refCount := objectWALRefCountForTest(tmpDir)
	require.GreaterOrEqual(t, refCount, 1)

	err = ReleaseObjectWAL(tmpDir, walObj)
	require.NoError(t, err)

	seq, err := ReadAppliedSeq(tmpDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, seq, int64(0))

	err = WriteAppliedSeq(tmpDir, 100)
	require.NoError(t, err)

	seq, err = ReadAppliedSeq(tmpDir)
	require.NoError(t, err)
	require.Equal(t, int64(100), seq)

	err = CompactWAL(tmpDir)
	require.NoError(t, err)

	lastSeq, err := ReadLastSeqFromTail(tmpDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, lastSeq, int64(0))

	err = ReplayWAL(tmpDir, 0, func(rec *WALRecord) error {
		return nil
	})
	require.NoError(t, err)

	replayed, lastS, err := ReplayWALChunk(tmpDir, 0, 10, func(rec *WALRecord) error {
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 0, replayed)
	require.GreaterOrEqual(t, lastS, int64(0))
}

func TestCycle122_WALFacade_LeaseAndParsing(t *testing.T) {
	tmpDir := setupCycle122IsolatedEnv(t)

	owner1 := "test-worker-1"
	owner2 := "test-worker-2"
	claimed := tryClaimWriteBehindOwner(tmpDir, owner1)
	require.True(t, claimed)

	claimed2 := tryClaimWriteBehindOwner(tmpDir, owner2)
	require.False(t, claimed2)

	logWriteBehindOwnerSkipped(tmpDir)
	releaseWriteBehindOwner(tmpDir, owner1)

	claimed3 := tryClaimWriteBehindOwner(tmpDir, owner2)
	require.True(t, claimed3)
	releaseWriteBehindOwner(tmpDir, owner2)

	records, err := parseWALLine([]byte(`{"seq":10,"op":"create"}`))
	if err != nil {
		require.Error(t, err)
	} else {
		require.NotEmpty(t, records)
	}

	maxSeq := getMaxSeqFromLine([]byte(`{"seq":15}`))
	require.GreaterOrEqual(t, maxSeq, int64(0))

	err = WaitForWALProcessingEventDriven(tmpDir, 10*time.Millisecond)
	if err != nil {
		require.Error(t, err)
	}
}

func TestCycle122_StreamRegistryPersistent_FastParsers(t *testing.T) {
	setupCycle122IsolatedEnv(t)

	line := []byte(`{"id":"TST-001","loc":"/path/to/stream::123","status":"active"}`)
	val := extractJSONStringFieldFast(line, []byte(`"id":"`))
	require.Equal(t, "TST-001", val)

	valMissing := extractJSONStringFieldFast(line, []byte(`"nonexistent":"`))
	require.Empty(t, valMissing)

	segLine := []byte(`{"id":"SEG-999","data":"hello"}`)
	id := parseSegmentLineIDFast(segLine)
	require.Equal(t, "SEG-999", id)

	regLine := []byte(`{"id":"OBJ-123","loc":"/streams/2026-10-22_stream.json::456"}`)
	id2, loc := parseStreamRegistryLineFast(regLine)
	require.Equal(t, "OBJ-123", id2)
	require.Equal(t, "/streams/2026-10-22_stream.json::456", loc)
}

func TestCycle122_StreamRegistryPersistent_EmptyHandling(t *testing.T) {
	err := AppendStreamLocationToRegistry("", "", "", "")
	require.NoError(t, err)

	err = AddStreamDeletedID("", "", "")
	require.NoError(t, err)

	err = BatchAddStreamDeletedIDs("", "", nil)
	require.NoError(t, err)

	loc := getStreamLocationFromPersistentRegistry("", "", "")
	require.Empty(t, loc)

	ids := ListStreamIDsFromPersistentRegistry("", "")
	require.Empty(t, ids)

	InvalidateStreamRegistryCacheForKind("", "")
}

func TestCycle122_StreamRegistryPersistent_Lifecycle(t *testing.T) {
	tmpDir := setupCycle122IsolatedEnv(t)
	kind := "audit_aggregation_metric"

	err := AppendStreamLocationToRegistry(tmpDir, kind, "AAM-001", "/file::0")
	require.NoError(t, err)
	err = AppendStreamLocationToRegistry(tmpDir, kind, "AAM-002", "/file::100")
	require.NoError(t, err)
	err = AppendStreamLocationToRegistry(tmpDir, kind, "AAM-003", "/file::200")
	require.NoError(t, err)

	sig := GetModSigForKind(tmpDir, kind)
	require.True(t, sig.exists)

	loc := getStreamLocationFromPersistentRegistry(tmpDir, kind, "AAM-001")
	require.Equal(t, "/file::0", loc)

	liveSet := LiveStreamIDSet(tmpDir, kind)
	require.True(t, liveSet["AAM-001"])
	require.True(t, liveSet["AAM-002"])
	require.True(t, liveSet["AAM-003"])

	oldest := OldestStreamIDsFromPersistentRegistry(tmpDir, kind, 2)
	require.Len(t, oldest, 2)

	err = BatchAddStreamDeletedIDs(tmpDir, kind, []string{"AAM-002"})
	require.NoError(t, err)

	liveSetAfter := LiveStreamIDSet(tmpDir, kind)
	require.True(t, liveSetAfter["AAM-001"])
	require.False(t, liveSetAfter["AAM-002"])

	InvalidateStreamRegistryCacheForKind(tmpDir, kind)
}

func TestCycle122_StreamSegmentGC(t *testing.T) {
	tmpDir := setupCycle122IsolatedEnv(t)

	res := GCOrphanedStreamSegmentsForKind("", "")
	require.Equal(t, 0, res.FilesRemoved)

	results, errKinds := GCOrphanedStreamSegmentsForAllKinds(tmpDir)
	require.NotNil(t, results)
	require.Equal(t, 0, errKinds)

	kind := "audit_aggregation_metric"
	segDir := setupTestSegmentDir(t, tmpDir, kind)
	todayBase := zqktime.NowLayoutUTC(zqktime.LayoutDate)
	deadPIDName := todayBase + "_pid99999999_stream.json"
	deadPIDFile := writeSegmentFile(t, segDir, deadPIDName)

	res = GCOrphanedStreamSegmentsForKind(tmpDir, kind)
	require.Equal(t, 1, res.FilesRemoved)

	_, statErr := fileutil.Stat(deadPIDFile)
	require.True(t, fileutil.IsNotExist(statErr))
}

func TestCycle122_ChangeJournalCompaction_RangesAndStats(t *testing.T) {
	setupCycle122IsolatedEnv(t)

	windows, entries := GetChangeJournalCompactionStats()
	require.GreaterOrEqual(t, windows, int64(0))
	require.GreaterOrEqual(t, entries, int64(0))

	entriesRaw := []map[string]any{
		{
			objects.FieldKeyID:           "CHA-1",
			objects.FieldKeyObjectRef:    "GOAL-001",
			objects.FieldKeyChangeType:   "create",
			objects.FieldKeyCreatedAt:    "2026-10-22T10:00:00Z",
			objects.FieldKeyCreatedBy:    "ACC-001",
			objects.FieldKeyChangedPaths: []any{"title", "description"},
		},
		{
			objects.FieldKeyID:           "CHA-2",
			objects.FieldKeyObjectRef:    "GOAL-001",
			objects.FieldKeyChangeType:   "update",
			objects.FieldKeyChangedPaths: []string{"status"},
		},
	}
	maps := EntryMapsFromEntries(entriesRaw)
	require.Len(t, maps, 2)
	require.Equal(t, "CHA-1", maps[0][objects.FieldKeyID])

	require.Nil(t, CompressEntryIDs(nil))

	entriesToCompress := []map[string]any{
		{objects.FieldKeyID: "CHA-1"},
		{objects.FieldKeyID: "CHA-2"},
		{objects.FieldKeyID: "CHA-3"},
		{objects.FieldKeyID: "CHA-5"},
		{objects.FieldKeyID: "OTHER-1"},
	}
	compressed := CompressEntryIDs(entriesToCompress)
	require.Contains(t, compressed, "CHA-1..CHA-3")
	require.Contains(t, compressed, "CHA-5")

	expanded := ExpandChangeJournalIDRanges([]string{"CHA-1..CHA-3", "CHA-5", "single", "CHA-bad..CHA-w"})
	require.Contains(t, expanded, "CHA-1")
	require.Contains(t, expanded, "CHA-2")
	require.Contains(t, expanded, "CHA-3")
	require.Contains(t, expanded, "CHA-5")
}

func TestCycle122_ChangeJournalCompaction_WindowAndArtifact(t *testing.T) {
	tmpDir := setupCycle122IsolatedEnv(t)
	wStart := time.Now().Add(-1 * time.Hour)
	wEnd := time.Now()

	emptyRes, err := CompactChangeJournalWindow(nil, tmpDir, wStart, wEnd, nil)
	require.NoError(t, err)
	require.Equal(t, 0, emptyRes.EntryCount)

	entries := []map[string]any{
		{
			objects.FieldKeyID:           "CHA-10",
			objects.FieldKeyObjectRef:    "REQ-001",
			objects.FieldKeyChangeType:   "create",
			objects.FieldKeyCreatedAt:    "2026-10-22T12:00:00Z",
			objects.FieldKeyCreatedBy:    "ACC-001",
			objects.FieldKeyChangedPaths: []string{"title"},
		},
		{
			objects.FieldKeyID:           "CHA-11",
			objects.FieldKeyObjectRef:    "REQ-001",
			objects.FieldKeyChangeType:   "update",
			objects.FieldKeyCreatedAt:    "2026-10-22T12:05:00Z",
			objects.FieldKeyCreatedBy:    "ACC-001",
			objects.FieldKeyChangedPaths: []string{"description"},
		},
	}
	res, err := CompactChangeJournalWindow(entries, tmpDir, wStart, wEnd, nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 2, res.EntryCount)
	require.FileExists(t, res.ArtifactPath)

	art, err := ReadCompactedChangeJournalArtifact(res.ArtifactPath)
	require.NoError(t, err)
	require.NotNil(t, art)
	require.Equal(t, CompactedChangeJournalFormatVersion, art.Header.FormatVersion)

	expandedObjs, err := ExpandCompactedChangeJournalArtifact(art)
	require.NoError(t, err)
	require.Len(t, expandedObjs, 2)

	nonexistentPath := filepath.Join(tmpDir, "nonexistent.yaml")
	_, err = ReadCompactedChangeJournalArtifact(nonexistentPath)
	require.Error(t, err)

	corruptPath := filepath.Join(tmpDir, "corrupt.yaml")
	err = fileutil.WriteFile(corruptPath, []byte("bad: [yaml"), paths.FilePerm600)
	require.NoError(t, err)
	_, err = ReadCompactedChangeJournalArtifact(corruptPath)
	require.Error(t, err)
}

type cycle122StorageMock struct {
	MockObjectStorage
	listResult *QueryResult
	listErr    error
}

func (c *cycle122StorageMock) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	if c.listResult != nil {
		return c.listResult, nil
	}
	return &QueryResult{Objects: []map[string]any{}}, nil
}

func TestCycle122_ChangeJournalAggregationService(t *testing.T) {
	setupCycle122IsolatedEnv(t)

	s1 := NewChangeJournalAggregationService(nil)
	require.Equal(t, DefaultBatchSize, s1.batchSize)

	s2 := NewChangeJournalAggregationServiceWithBatchSize(nil, -10)
	require.Equal(t, DefaultBatchSize, s2.batchSize)

	s3 := NewChangeJournalAggregationServiceWithBatchSize(nil, 50)
	require.Equal(t, 50, s3.batchSize)

	runs, entries := s3.GetAggregationStats()
	require.Equal(t, int64(0), runs)
	require.Equal(t, int64(0), entries)

	mock := &cycle122StorageMock{
		listResult: &QueryResult{Objects: []map[string]any{}},
	}
	service := NewChangeJournalAggregationServiceWithBatchSize(mock, 10)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	res, err := service.AggregateChangeJournalEntries(ctx, secCtx, storageCtx, time.Now().Add(-1*time.Hour), time.Now())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 0, res.EntryCount)
	require.Equal(t, 0, res.MetricsCreated)

	runs, entriesAgg := service.GetAggregationStats()
	require.Equal(t, int64(1), runs)
	require.Equal(t, int64(0), entriesAgg)
}

type cycle122RoutingMock struct {
	NoopObjectStorage
	name       string
	lastCalled string
}

func (m *cycle122RoutingMock) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.lastCalled = "Create"
	return nil
}
func (m *cycle122RoutingMock) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.lastCalled = "Read"
	return map[string]any{objects.FieldKeyID: id, objects.FieldKeyKind: "goal"}, nil
}
func (m *cycle122RoutingMock) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.lastCalled = "Update"
	return nil
}
func (m *cycle122RoutingMock) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.lastCalled = "Delete"
	return nil
}
func (m *cycle122RoutingMock) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	m.lastCalled = "List"
	return &QueryResult{Objects: []map[string]any{}}, nil
}
func (m *cycle122RoutingMock) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	m.lastCalled = "Query"
	return &QueryResult{Objects: []map[string]any{}}, nil
}
func (m *cycle122RoutingMock) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	m.lastCalled = "Search"
	return &SearchResult{}, nil
}
func (m *cycle122RoutingMock) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	m.lastCalled = "BeginTransaction"
	return nil, nil
}
func (m *cycle122RoutingMock) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	m.lastCalled = "BulkCreate"
	return &BulkResult{SuccessCount: len(objects), TotalCount: len(objects)}, nil
}
func (m *cycle122RoutingMock) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	m.lastCalled = "BulkUpdate"
	return &BulkResult{SuccessCount: len(updates), TotalCount: len(updates)}, nil
}
func (m *cycle122RoutingMock) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	m.lastCalled = "BulkGet"
	return &BulkResult{SuccessCount: len(ids), TotalCount: len(ids), Results: make([]map[string]any, len(ids))}, nil
}
func (m *cycle122RoutingMock) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	m.lastCalled = "BulkDelete"
	return &BulkResult{SuccessCount: len(ids), TotalCount: len(ids)}, nil
}
func (m *cycle122RoutingMock) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	m.lastCalled = "Exists"
	return true, nil
}
func (m *cycle122RoutingMock) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	m.lastCalled = "Count"
	return 1, nil
}
func (m *cycle122RoutingMock) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	m.lastCalled = "Aggregate"
	return &AggregateResult{}, nil
}
func (m *cycle122RoutingMock) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	m.lastCalled = "GetRelated"
	return []map[string]any{}, nil
}
func (m *cycle122RoutingMock) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	m.lastCalled = "GetPath"
	return []map[string]any{}, nil
}
func (m *cycle122RoutingMock) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	m.lastCalled = "GetNeighbors"
	return []map[string]any{}, nil
}
func (m *cycle122RoutingMock) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	m.lastCalled = "Move"
	return nil
}
func (m *cycle122RoutingMock) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	m.lastCalled = "Rename"
	return nil
}
func (m *cycle122RoutingMock) Shutdown(ctx context.Context) error {
	m.lastCalled = "Shutdown"
	return nil
}

func TestCycle122_RoutingStorage_Delegations(t *testing.T) {
	setupCycle122IsolatedEnv(t)
	mockDefault := &cycle122RoutingMock{name: "default"}
	mockGoal := &cycle122RoutingMock{name: "goal"}
	factory := &StorageFactory{
		defaultStorage: mockDefault,
		providers: map[string]ObjectStorageProvider{
			"goal": mockGoal,
		},
	}
	router := NewRoutingObjectStorage(factory)
	router.idValidator.InjectPatternForTest("goal", "^GOAL-\\d{3}$", "GOAL-")

	require.Equal(t, factory, router.GetStorageFactory())
	require.Nil(t, router.GetPool())

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	runRoutingCRUDChecks(t, router, ctx, secCtx, storageCtx)
	runRoutingBulkChecks(t, router, ctx, secCtx)
	runRoutingGraphAndLifecycleChecks(t, router, ctx, secCtx, storageCtx)
}

func runRoutingCRUDChecks(t *testing.T, router *RoutingObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext) {
	t.Helper()
	err := router.Create(ctx, secCtx, map[string]any{objects.FieldKeyKind: "goal", "title": "test"})
	require.NoError(t, err)

	readObj, err := router.Read(ctx, secCtx, "GOAL-001")
	require.NoError(t, err)
	require.NotNil(t, readObj)

	err = router.Update(ctx, secCtx, "GOAL-001", map[string]any{objects.FieldKeyKind: "goal", "title": "updated"})
	require.NoError(t, err)

	err = router.Delete(ctx, secCtx, "GOAL-001", false)
	require.NoError(t, err)

	res, err := router.List(ctx, secCtx, storageCtx, ListFilter{Kind: "goal"})
	require.NoError(t, err)
	require.NotNil(t, res)

	qRes, err := router.Query(ctx, secCtx, storageCtx, Query{Kind: "goal"})
	require.NoError(t, err)
	require.NotNil(t, qRes)

	sRes, err := router.Search(ctx, secCtx, storageCtx, SearchQuery{Query: "test"})
	require.NoError(t, err)
	require.NotNil(t, sRes)

	tx, err := router.BeginTransaction(ctx)
	require.NoError(t, err)
	require.Nil(t, tx)
}

func runRoutingBulkChecks(t *testing.T, router *RoutingObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	b1, err := router.BulkCreate(ctx, secCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, b1)

	b2, err := router.BulkCreate(ctx, secCtx, []map[string]any{{objects.FieldKeyKind: "goal"}})
	require.NoError(t, err)
	require.Equal(t, 1, b2.SuccessCount)

	u1, err := router.BulkUpdate(ctx, secCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, u1)

	u2, err := router.BulkUpdate(ctx, secCtx, []BulkUpdateItem{{ID: "GOAL-001", Updates: map[string]any{objects.FieldKeyKind: "goal"}}})
	require.NoError(t, err)
	require.Equal(t, 1, u2.SuccessCount)

	g1, err := router.BulkGet(ctx, secCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, g1)

	g2, err := router.BulkGet(ctx, secCtx, []string{"GOAL-001"})
	require.NoError(t, err)
	require.Len(t, g2.Results, 1)

	d1, err := router.BulkDelete(ctx, secCtx, nil, false)
	require.NoError(t, err)
	require.NotNil(t, d1)

	d2, err := router.BulkDelete(ctx, secCtx, []string{"GOAL-001"}, false)
	require.NoError(t, err)
	require.Equal(t, 1, d2.SuccessCount)
}

func runRoutingGraphAndLifecycleChecks(t *testing.T, router *RoutingObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext) {
	t.Helper()
	exists, err := router.Exists(ctx, secCtx, "GOAL-001")
	require.NoError(t, err)
	require.True(t, exists)

	cnt, err := router.Count(ctx, secCtx, ListFilter{Kind: "goal"})
	require.NoError(t, err)
	require.Equal(t, 1, cnt)

	agg, err := router.Aggregate(ctx, secCtx, storageCtx, ListFilter{Kind: "goal"}, nil)
	require.NoError(t, err)
	require.NotNil(t, agg)

	rel, err := router.GetRelated(ctx, secCtx, "GOAL-001", "parent", 1)
	require.NoError(t, err)
	require.NotNil(t, rel)

	path, err := router.GetPath(ctx, secCtx, "GOAL-001", "GOAL-002")
	require.NoError(t, err)
	require.NotNil(t, path)

	nbrs, err := router.GetNeighbors(ctx, secCtx, "GOAL-001", "out")
	require.NoError(t, err)
	require.NotNil(t, nbrs)

	err = router.Move(ctx, secCtx, "GOAL-001", "goal", false)
	require.NoError(t, err)

	err = router.Rename(ctx, secCtx, "GOAL-001", "GOAL-002", false)
	require.NoError(t, err)

	err = router.Shutdown(ctx)
	require.NoError(t, err)
}

func TestCycle122_SnapshotManager_Coordination(t *testing.T) {
	tmpDir := setupCycle122IsolatedEnv(t)
	var nilMeta *SnapshotMetadata
	r0, c0 := nilMeta.CountHashSources()
	require.Equal(t, 0, r0)
	require.Equal(t, 0, c0)

	meta := &SnapshotMetadata{
		HashSources: map[string]string{
			"obj1": "registry",
			"obj2": "calculated",
			"obj3": "registry",
		},
	}
	r1, c1 := meta.CountHashSources()
	require.Equal(t, 2, r1)
	require.Equal(t, 1, c1)

	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(10)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, tmpDir)
	ctx := context.Background()

	err := manager.EndSnapshot(ctx)
	require.Error(t, err)

	_, err = manager.CaptureMetadata(ctx, []string{"OBJ-1"})
	require.Error(t, err)

	ts, err := manager.BeginSnapshot()
	require.NoError(t, err)
	require.True(t, manager.IsActive())
	require.False(t, ts.IsZero())
	require.Equal(t, ts, manager.GetSnapshotTimestamp())

	_, err = manager.BeginSnapshot()
	require.Error(t, err)

	mockStorage.objects["GOAL-001"] = map[string]any{
		objects.FieldKeyID:   "GOAL-001",
		objects.FieldKeyKind: "goal",
	}
	capMeta, err := manager.CaptureMetadata(ctx, []string{"GOAL-001"})
	require.NoError(t, err)
	require.NotNil(t, capMeta)

	err = manager.EndSnapshot(ctx)
	require.NoError(t, err)
	require.False(t, manager.IsActive())
}

func TestCycle122_BatchProcessor_Scenarios(t *testing.T) {
	setupCycle122IsolatedEnv(t)

	bpDefault := NewBatchProcessor(0)
	require.Equal(t, DefaultBatchSize, bpDefault.batchSize)

	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := bpDefault.ProcessInBatches(ctxCancelled, nil, nil, nil)
	require.ErrorIs(t, err, context.Canceled)

	bp := NewBatchProcessor(2)
	errExpected := errors.New("query failure")
	_, _, err = bp.ProcessInBatches(context.Background(), func(c context.Context, o, l int) ([]map[string]any, error) {
		return nil, errExpected
	}, nil, nil)
	require.Error(t, err)

	errProc := errors.New("process failure")
	_, _, err = bp.ProcessInBatches(context.Background(), func(c context.Context, o, l int) ([]map[string]any, error) {
		return []map[string]any{{"id": "1"}}, nil
	}, func(b []map[string]any) (any, []string, error) {
		return nil, nil, errProc
	}, nil)
	require.Error(t, err)

	errMerge := errors.New("merge failure")
	callCount := 0
	_, _, err = bp.ProcessInBatches(context.Background(), func(c context.Context, o, l int) ([]map[string]any, error) {
		callCount++
		if callCount > 2 {
			return nil, nil
		}
		return []map[string]any{{"id": "1"}, {"id": "2"}}, nil
	}, func(b []map[string]any) (any, []string, error) {
		return 1, []string{"1"}, nil
	}, func(f, s any) (any, error) {
		return nil, errMerge
	})
	require.Error(t, err)

	emptyRes, emptyIDs, err := bp.ProcessInBatches(context.Background(), func(c context.Context, o, l int) ([]map[string]any, error) {
		return nil, nil
	}, nil, nil)
	require.NoError(t, err)
	require.Nil(t, emptyRes)
	require.Empty(t, emptyIDs)

	mockStorage := &cycle122StorageMock{
		listResult: &QueryResult{Objects: []map[string]any{{"id": "GOAL-001"}}},
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	builder := NewBatchQueryBuilder(mockStorage, secCtx, storageCtx, "goal")
	builder.WithFilters(map[string]any{"status": "originated"}).WithSort("title", false)
	qFn := builder.BuildQueryFunc(context.Background())
	objs, err := qFn(context.Background(), 0, 10)
	require.NoError(t, err)
	require.Len(t, objs, 1)
}
