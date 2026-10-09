package filecas

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_BlobCache(t *testing.T) {
	ClearCASBlobCache()

	hash := "1111111111111111111111111111111111111111111111111111111111111111"
	data := []byte("test blob data")

	// store and lookup via unexported helpers
	storeCASBlob(hash, data)
	got, ok := lookupCASBlob(hash)
	if !ok || string(got) != string(data) {
		t.Errorf("lookupCASBlob failed: got %v, ok %v", got, ok)
	}

	// EvictCASBlob
	if !EvictCASBlob(hash) {
		t.Errorf("expected EvictCASBlob to return true")
	}
	if EvictCASBlob("invalid-short-hash") {
		t.Errorf("expected false for invalid hash")
	}

	// ClearCASBlobCache
	storeCASBlob(hash, data)
	ClearCASBlobCache()
	if _, ok := lookupCASBlob(hash); ok {
		t.Errorf("expected miss after ClearCASBlobCache")
	}

	// Edge cases
	StoreCASBlob("not-valid-hash", data)
	StoreCASBlob(hash, nil)

	// Direct BlobCache tests
	bc := NewBlobCache(2)
	if bc.Len() != 0 {
		t.Errorf("expected 0 len")
	}
	bc.Put("k1", []byte("v1"))
	bc.Put("k2", []byte("v2"))
	bc.Put("k3", []byte("v3")) // evicts k1
	if _, ok := bc.Get("k1"); ok {
		t.Errorf("expected k1 to be evicted")
	}
	if v, ok := bc.Peek("k2"); !ok || string(v) != "v2" {
		t.Errorf("Peek k2 failed: %s, %v", string(v), ok)
	}
	bc.Put("k2", []byte("v2-updated"))
	if !bc.Evict("k2") {
		t.Errorf("Evict k2 failed")
	}
	bc.Clear()
	if bc.Len() != 0 {
		t.Errorf("expected len 0 after clear")
	}
	// nil receiver edge cases
	var nilBC *BlobCache
	nilBC.Put("a", []byte("b"))
	_, _ = nilBC.Get("a")
	_, _ = nilBC.Peek("a")
	_ = nilBC.Evict("a")
	nilBC.Clear()
	if nilBC.Len() != 0 {
		t.Errorf("nilBC len failed")
	}
	NewBlobCache(0) // tests min 1
}

func TestExtended_CASIndexMergeAndPostwrite(t *testing.T) {
	kindDir := t.TempDir()

	// pickCASIndexHash: neither exists
	chosen := pickCASIndexHash(kindDir, "aaaa", "bbbb")
	if chosen != "aaaa" {
		t.Errorf("expected diskHash when neither exists, got %s", chosen)
	}

	// Create disk blob
	diskHash := "2222222222222222222222222222222222222222222222222222222222222222"
	memHash := "3333333333333333333333333333333333333333333333333333333333333333"
	_ = fileutil.WriteStandardFile(filepath.Join(kindDir, diskHash+".yaml"), []byte("disk"))

	// Disk exists, mem does not
	if pickCASIndexHash(kindDir, diskHash, memHash) != diskHash {
		t.Errorf("expected diskHash")
	}

	// Mem exists, disk does not
	_ = fileutil.RemoveFile(filepath.Join(kindDir, diskHash+".yaml"))
	_ = fileutil.WriteStandardFile(filepath.Join(kindDir, memHash+".yaml"), []byte("mem"))
	if pickCASIndexHash(kindDir, diskHash, memHash) != memHash {
		t.Errorf("expected memHash")
	}

	// Both exist
	_ = fileutil.WriteStandardFile(filepath.Join(kindDir, diskHash+".yaml"), []byte("disk"))
	if pickCASIndexHash(kindDir, diskHash, memHash) != diskHash {
		t.Errorf("expected diskHash when both exist")
	}

	// MergeCASIndexMaps
	diskMap := map[string]string{"ID-1": diskHash, "ID-2": diskHash}
	memMap := map[string]string{"ID-2": memHash, "ID-3": memHash, "": "ignored"}
	merged := MergeCASIndexMaps(kindDir, diskMap, memMap)
	if len(merged) < 3 || merged["ID-3"] != memHash {
		t.Errorf("MergeCASIndexMaps unexpected: %v", merged)
	}

	// hashPrefixForLog
	if hashPrefixForLog("") != "(missing)" {
		t.Errorf("expected (missing)")
	}
	if hashPrefixForLog("short") != "short" {
		t.Errorf("expected short")
	}
	if len(hashPrefixForLog("12345678901234567890")) != 16 {
		t.Errorf("expected 16 chars")
	}

	// cleanupStaleCASIndexTempFiles
	cleanupStaleCASIndexTempFiles("", "", "")
	tempDir := t.TempDir()
	indexBase := "index"
	cleanupStaleCASIndexTempFiles(tempDir, indexBase, "")
	staleTmp := filepath.Join(tempDir, indexBase+".tmp-12345")
	_ = fileutil.WriteStandardFile(staleTmp, []byte("stale"))
	cleanupStaleCASIndexTempFiles(tempDir, indexBase, staleTmp) // skip justWroteTmp
	cleanupStaleCASIndexTempFiles(tempDir, indexBase, "")
}

func TestExtended_StreamSegmentMembrane(t *testing.T) {
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "segment_test")

	payload := []byte("stream segment test payload that should be partitioned and compressed")
	objectID := "SEG-001"

	// PutStreamSegmentChunk
	h, err := cas.PutStreamSegmentChunk(objectID, payload)
	if err != nil || h == "" {
		t.Fatalf("PutStreamSegmentChunk failed: %v", err)
	}

	// GetStreamSegmentChunk
	data, err := cas.GetStreamSegmentChunk(objectID)
	if err != nil || string(data) != string(payload) {
		t.Fatalf("GetStreamSegmentChunk failed: %v, got %s", err, string(data))
	}

	// RehydrateChunk
	data2, err := cas.RehydrateChunk(h)
	if err != nil || string(data2) != string(payload) {
		t.Fatalf("RehydrateChunk failed: %v", err)
	}

	// PutStreamSegment multi-chunk / larger
	hashes, err := cas.PutStreamSegment("SEG-002", payload)
	if err != nil || len(hashes) == 0 {
		t.Fatalf("PutStreamSegment failed: %v", err)
	}

	membrane := cas.StreamSegmentMembrane()
	if !membrane.HasChunk(h) {
		t.Errorf("expected membrane.HasChunk to be true for %s", h)
	}
	if membrane.ChunksDir() == "" {
		t.Errorf("ChunksDir should not be empty")
	}
	cnt, err := membrane.CountLiveChunkBlobs()
	if err != nil || cnt == 0 {
		t.Errorf("CountLiveChunkBlobs failed: %v, cnt=%d", err, cnt)
	}
}

func TestExtended_CRUDMethods(t *testing.T) {
	prevMetrics := GetMetrics
	GetMetrics = func() StorageMetrics { return noopStorageMetrics{} }
	prevPending := NoteObjectIDCachePending
	NoteObjectIDCachePending = func(string, string, string, string, string, string) {}
	prevVis := GetCASPendingVisibilityCache
	GetCASPendingVisibilityCache = func(string) PendingVisibilityCache { return noopPendingCache{} }
	prevHV := IsHighVolumeKindForCache
	IsHighVolumeKindForCache = func(string) bool { return false }
	t.Cleanup(func() {
		GetMetrics = prevMetrics
		NoteObjectIDCachePending = prevPending
		GetCASPendingVisibilityCache = prevVis
		IsHighVolumeKindForCache = prevHV
	})

	kindDir := t.TempDir()
	queue := immediateMockIndexQueue{}
	cas := NewContentAddressableStorage(kindDir, "task", queue)
	cas.SetPostSyncCallback(func(string, string, string, string) error { return nil })

	// SetSkipIndexUpdateWait
	SetSkipIndexUpdateWait(true)
	if !GetSkipIndexUpdateWait() {
		t.Errorf("expected skipIndexUpdateWait true")
	}
	SetSkipIndexUpdateWait(false)
	if GetSkipIndexUpdateWait() {
		t.Errorf("expected skipIndexUpdateWait false")
	}

	// parseCreatedAtFromObjectData
	rawYAML := []byte("id: TASK-1\ncreated_at: \"2026-09-23T12:00:00Z\"\n")
	ca := parseCreatedAtFromObjectData(rawYAML)
	if ca != "2026-09-23T12:00:00Z" {
		t.Errorf("parseCreatedAtFromObjectData failed: %s", ca)
	}

	// Create and Read
	err := cas.Create("TASK-1", rawYAML, "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	readData, err := cas.Read("TASK-1")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(readData) == 0 {
		t.Errorf("Read returned empty data")
	}

	// Read missing ID
	_, err = cas.Read("TASK-MISSING")
	if err == nil {
		t.Errorf("expected error reading missing ID")
	}

	// ListIDs & GetAllMappings
	ids, err := cas.ListIDs()
	if err != nil || len(ids) == 0 {
		t.Errorf("ListIDs failed: %v, ids: %v", err, ids)
	}
	mappings, err := cas.GetAllMappings()
	if err != nil || len(mappings) == 0 {
		t.Errorf("GetAllMappings failed: %v", err)
	}

	// GetFilePathForID
	path, err := cas.GetFilePathForID("TASK-1")
	if err != nil || path == "" {
		t.Errorf("GetFilePathForID failed: %v, path: %s", err, path)
	}

	// Delete
	err = cas.Delete("TASK-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Delete missing ID
	err = cas.Delete("TASK-1")
	if err == nil {
		t.Errorf("expected error deleting non-existent ID")
	}

	// BatchDelete
	// Create two items
	_ = cas.Create("TASK-B1", []byte("id: TASK-B1\n"), "")
	_ = cas.Create("TASK-B2", []byte("id: TASK-B2\n"), "")
	err = cas.BatchDelete([]string{"TASK-B1", "TASK-B2"})
	if err != nil {
		t.Errorf("BatchDelete failed: %v", err)
	}
	// BatchDelete empty
	if err := cas.BatchDelete(nil); err != nil {
		t.Errorf("BatchDelete nil failed: %v", err)
	}

	// Bucketed Create, Read, and Update (bucket migration)
	bucketDir1 := filepath.Join(kindDir, "2026-09")
	bucketDir2 := filepath.Join(kindDir, "2026-10")
	_ = fileutil.MkdirAll(bucketDir1, 0755)
	_ = fileutil.MkdirAll(bucketDir2, 0755)

	bucketData := []byte("id: TASK-BUCKET\nstatus: active\n")
	err = cas.Create("TASK-BUCKET", bucketData, bucketDir1)
	if err != nil {
		t.Errorf("Create in bucket failed: %v", err)
	}

	readBucket, err := cas.Read("TASK-BUCKET")
	if err != nil || len(readBucket) == 0 {
		t.Errorf("Read from bucket failed: %v", err)
	}

	// Update existing object in bucket, migrating to bucketDir2
	bucketDataV2 := []byte("id: TASK-BUCKET\nstatus: completed\n")
	err = cas.Update("TASK-BUCKET", bucketDataV2, bucketDir2)
	if err != nil {
		t.Errorf("Update migrating bucket failed: %v", err)
	}

	// Update non-existent object directly with targetBucketDir
	newObjInBucket := []byte("id: TASK-NEW-BUCKET\nstatus: new\n")
	err = cas.Update("TASK-NEW-BUCKET", newObjInBucket, bucketDir1)
	if err != nil {
		t.Errorf("Update new object in bucket failed: %v", err)
	}

	// Update with empty content
	if err := cas.Update("TASK-BUCKET", nil); err == nil {
		t.Errorf("expected error updating with empty content")
	}

	// findHashFile directly
	h := CalculateSHA256Hash(bucketDataV2)
	foundPath, foundDir := cas.findHashFile(h, "2026-10")
	if foundPath == "" || foundDir == "" {
		t.Errorf("findHashFile with bucket key failed")
	}
	// findHashFile scanning subdirs
	foundPath2, _ := cas.findHashFile(h, "")
	if foundPath2 == "" {
		t.Errorf("findHashFile scan failed")
	}
}

func TestExtended_CallbacksAndIndexWrappers(t *testing.T) {
	// Exercise getSafeMetrics & getSafeLockStrategy fallbacks
	prevM := GetMetrics
	GetMetrics = nil
	sm := getSafeMetrics()
	if sm == nil {
		t.Errorf("getSafeMetrics returned nil")
	}
	sm.RecordCreate(time.Millisecond, true)
	sm.RecordRead(time.Millisecond, true)
	sm.RecordUpdate(time.Millisecond, true)
	sm.RecordDelete(time.Millisecond, true)
	sm.RecordIndexFileLock(true, time.Millisecond)
	sm.RecordIndexSave(time.Millisecond, nil, 1)
	sm.RecordSetMapping(time.Millisecond, nil, true)
	sm.RecordIndexReload()
	sm.RecordRemoveMapping(time.Millisecond, nil)

	prevLockStrat := NewAutoCleanupStrategy
	NewAutoCleanupStrategy = nil
	ls := getSafeLockStrategy()
	if ls == nil {
		t.Errorf("getSafeLockStrategy returned nil")
	}
	lh, _ := ls.AcquireLock("dummy", time.Second)
	_ = lh.Release()

	prevVis := GetCASPendingVisibilityCache
	GetCASPendingVisibilityCache = func(string) PendingVisibilityCache { return noopPendingCache{} }
	GetMetrics = func() StorageMetrics { return noopStorageMetrics{} }
	prevHV := IsHighVolumeKindForCache
	IsHighVolumeKindForCache = func(string) bool { return false }
	prevPending := NoteObjectIDCachePending
	NoteObjectIDCachePending = func(string, string, string, string, string, string) {}
	prevHandler := GetCacheOperationHandler
	GetCacheOperationHandler = func() func(*pkgctx.CacheContext) error {
		return nil
	}
	t.Cleanup(func() {
		GetCASPendingVisibilityCache = prevVis
		GetMetrics = prevM
		IsHighVolumeKindForCache = prevHV
		NoteObjectIDCachePending = prevPending
		GetCacheOperationHandler = prevHandler
		NewAutoCleanupStrategy = prevLockStrat
	})

	kindDir := t.TempDir()
	queue := immediateMockIndexQueue{}
	cas := NewContentAddressableStorage(kindDir, "task", queue)
	cas.SetPostSyncCallback(func(string, string, string, string) error { return nil })

	// Callbacks
	cas.SetOrphanCleanupCallback(func(oldHashFilePath string) error {
		return nil
	})
	cas.SetPostSyncCallback(func(objectID, kind, hash, filePath string) error {
		return nil
	})
	cas.SetRequiredPostSyncCallback(func(objectID, kind, hash, filePath string) error {
		return nil
	})

	// GetWriteQueue
	wq := cas.GetWriteQueue()
	if wq == nil {
		t.Errorf("GetWriteQueue returned nil")
	}

	// NewContentAddressableStorageWithIndex
	idx := cas.GetIndex()
	casWithIdx := NewContentAddressableStorageWithIndex(idx)
	if casWithIdx == nil {
		t.Errorf("NewContentAddressableStorageWithIndex returned nil")
	}

	// EnsureCASIndexMatchesContentHash
	payload := []byte("id: TST-ENSURE\nkind: task\n")
	_ = cas.EnsureCASIndexMatchesContentHash("TST-ENSURE", payload, "")
	_ = cas.EnsureCASIndexMatchesContentHash("", nil, "")

	// dropGhostIndexMapping
	cas.dropGhostIndexMapping("")
	cas.dropGhostIndexMapping("GHOST-1")

	// removeHashFileByScan
	cas.removeHashFileByScan("non-existent-hash")

	// getCASReadDirPool
	pool := getCASReadDirPool()
	if pool == nil {
		t.Errorf("getCASReadDirPool returned nil")
	}

	// Accessors
	if cas.GetKindDir() != kindDir {
		t.Errorf("GetKindDir failed")
	}
	if cas.GetKind() != "task" {
		t.Errorf("GetKind failed")
	}
	cas.RemoveIndexMappingInMemory("task-non-existent")

	// SetOperationCallback
	cas.SetOperationCallback(&concurrency.NoOpOperationCallback{})

	// ErasePending methods on IDIndex and CAS
	if cas.IsErasePending("ERASE-1") {
		t.Errorf("expected not erase pending initially")
	}
	cas.SetErasePending("ERASE-1", "test_event")
	if !cas.IsErasePending("ERASE-1") {
		t.Errorf("expected erase pending after set")
	}
	evt, ok := cas.GetErasePendingEventID("ERASE-1")
	if !ok || evt != "test_event" {
		t.Errorf("GetErasePendingEventID failed: got %s, %v", evt, ok)
	}
	snap := cas.SnapshotErasePending()
	if len(snap) != 1 || snap["ERASE-1"] != "test_event" {
		t.Errorf("SnapshotErasePending failed: %v", snap)
	}
	cas.ClearErasePending("ERASE-1")
	if cas.IsErasePending("ERASE-1") {
		t.Errorf("expected erase pending cleared")
	}

	// OldestIDs (max-heap implementation)
	idx.CreatedAt = map[string]string{
		"ID-NEW":  "2026-09-23T12:00:00Z",
		"ID-MID1": "2026-09-20T12:00:00Z",
		"ID-MID2": "2026-09-18T12:00:00Z",
		"ID-MID3": "2026-09-15T12:00:00Z",
		"ID-OLD":  "2026-09-10T12:00:00Z",
		"ID-BAD":  "invalid",
	}
	oldest := idx.OldestIDs(2)
	if len(oldest) != 2 {
		t.Errorf("OldestIDs failed: got %v", oldest)
	}
	if idx.OldestIDs(0) != nil {
		t.Errorf("expected nil for 0 limit")
	}

	// Index Save and RemoveMapping
	if err := idx.Save(); err != nil {
		t.Errorf("idx.Save failed: %v", err)
	}
	if err := idx.RemoveMapping("ID-OLD"); err != nil {
		t.Errorf("idx.RemoveMapping failed: %v", err)
	}
	_ = idx.LoadLocked()

	// UpdateWithIDChange
	dataOld := []byte("id: OLD-ID\nkind: task\n")
	_ = cas.Create("OLD-ID", dataOld)
	dataNew := []byte("id: NEW-ID\nkind: task\n")
	err := cas.UpdateWithIDChange("OLD-ID", "NEW-ID", dataNew)
	if err != nil {
		t.Errorf("UpdateWithIDChange failed: %v", err)
	}
	// empty content
	if err := cas.UpdateWithIDChange("OLD-ID", "NEW-ID", nil); err == nil {
		t.Errorf("expected error for empty content")
	}
	// HealIndexMappingAsync
	cas.HealIndexMappingAsync("HEAL-1", "0000000000000000000000000000000000000000000000000000000000000000")
	cas.HealIndexMappingAsync("", "")

	// SnapshotBucketKeys
	idx.BucketKeys = map[string]string{"HEAL-1": "bucket1"}
	bkSnap := idx.SnapshotBucketKeys()
	if bkSnap["HEAL-1"] != "bucket1" {
		t.Errorf("SnapshotBucketKeys failed: %v", bkSnap)
	}

	// ContentAddressableStorageOrIndexMissing
	if !ContentAddressableStorageOrIndexMissing(errfmt.Errorf("err"), cas) {
		t.Errorf("expected true when err is non-nil")
	}
	if !ContentAddressableStorageOrIndexMissing(nil, nil) {
		t.Errorf("expected true when cas is nil")
	}
	if !ContentAddressableStorageOrIndexMissing(nil, &ContentAddressableStorage{}) {
		t.Errorf("expected true when index is nil")
	}
	if ContentAddressableStorageOrIndexMissing(nil, cas) {
		t.Errorf("expected false when no err and cas is valid")
	}

	// noopStorageMetrics
	metrics := noopStorageMetrics{}
	metrics.RecordCreate(time.Second, true)
	metrics.RecordRead(time.Second, true)
	metrics.RecordUpdate(time.Second, true)
	metrics.RecordDelete(time.Second, true)
	metrics.RecordIndexFileLock(true, time.Second)
	metrics.RecordIndexSave(time.Second, nil, 1)
	metrics.RecordSetMapping(time.Second, nil, true)
	metrics.RecordIndexReload()
	metrics.RecordRemoveMapping(time.Second, nil)

	// GetFilePathForID branches
	// 1. In bucket
	bkDir := filepath.Join(kindDir, "2026-09")
	_ = fileutil.MkdirAll(bkDir, 0755)
	cas.index.SetMapping("FILE-BK", "hash123", "2026-09")
	bFile := filepath.Join(bkDir, "hash123.yaml")
	_ = fileutil.WriteStandardFile(bFile, []byte("data"))
	fp, err := cas.GetFilePathForID("FILE-BK")
	if err != nil || fp != bFile {
		t.Errorf("GetFilePathForID bucket failed: %s, %v", fp, err)
	}

	// 2. In base kindDir
	hex64 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cas.index.SetMapping("FILE-BASE", hex64)
	baseF := filepath.Join(kindDir, hex64+".yaml")
	_ = fileutil.WriteStandardFile(baseF, []byte("id: FILE-BASE\n"))
	fp, err = cas.GetFilePathForID("FILE-BASE")
	if err != nil || fp != baseF {
		t.Errorf("GetFilePathForID base failed: %s, %v", fp, err)
	}

	// 3. Fallback date bucket in subdir without bucket key in index
	cas.index.SetMapping("FILE-SUB", "hash789")
	dateDir := filepath.Join(kindDir, "2026-11-01")
	_ = fileutil.MkdirAll(dateDir, 0755)
	subF := filepath.Join(dateDir, "hash789.yaml")
	_ = fileutil.WriteStandardFile(subF, []byte("data"))
	fp, err = cas.GetFilePathForID("FILE-SUB")
	if err != nil || fp != subF {
		t.Errorf("GetFilePathForID fallback sub failed: %s, %v", fp, err)
	}

	// 4. Ghost mapping (hash file does not exist)
	cas.index.SetMapping("FILE-GHOST", "hashGhost")
	_, err = cas.GetFilePathForID("FILE-GHOST")
	if err == nil {
		t.Errorf("expected error for ghost mapping")
	}

	// listLiveCASBlobsForObjectID
	if listLiveCASBlobsForObjectID("", "") != nil {
		t.Errorf("expected nil for empty args")
	}
	blobs := listLiveCASBlobsForObjectID("FILE-BASE", kindDir)
	if len(blobs) == 0 {
		t.Errorf("expected to find live blob for FILE-BASE")
	}

	// RemoveOrphanCASHashFileSync
	orphanFile := filepath.Join(kindDir, "orphan.yaml")
	_ = fileutil.WriteStandardFile(orphanFile, []byte("data"))
	if err := RemoveOrphanCASHashFileSync(orphanFile); err != nil {
		t.Errorf("RemoveOrphanCASHashFileSync failed: %v", err)
	}
	// non-existent file
	if err := RemoveOrphanCASHashFileSync(filepath.Join(kindDir, "non-existent.yaml")); err != nil {
		t.Errorf("expected no error for non-existent file: %v", err)
	}

	// WriteFileWithSync
	syncTarget := filepath.Join(kindDir, "sync-test.yaml")
	syncData := []byte("sync data")
	if err := cas.WriteFileWithSync(syncTarget, syncData); err != nil {
		t.Errorf("WriteFileWithSync failed: %v", err)
	}
	// fast-path: file already exists with identical content
	if err := cas.WriteFileWithSync(syncTarget, syncData); err != nil {
		t.Errorf("WriteFileWithSync fast-path matching failed: %v", err)
	}
	// fast-path: file already exists with different content (hash mismatch)
	if err := cas.WriteFileWithSync(syncTarget, []byte("different")); err == nil {
		t.Errorf("expected hash mismatch error on existing file")
	}

	// removeHashFileByScan in subdir
	scanSubDir := filepath.Join(kindDir, "scan-sub")
	_ = fileutil.MkdirAll(scanSubDir, 0755)
	scanHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	scanFilePath := filepath.Join(scanSubDir, scanHash+".yaml")
	_ = fileutil.WriteStandardFile(scanFilePath, []byte("sub-data"))
	cas.removeHashFileByScan(scanHash)
	if fileutil.Exists(scanFilePath) {
		t.Errorf("expected removeHashFileByScan to remove file in subdir")
	}
}

type immediateMockIndexQueue struct{}

func immediateDone() <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	return ch
}

func (immediateMockIndexQueue) EnqueueUpdateWithOperationCallbackAndCreatedAt(string, string, string, string, string, *ContentAddressableStorage, concurrency.OperationCallback) (<-chan error, error) {
	return immediateDone(), nil
}
func (immediateMockIndexQueue) EnqueueUpdateWithOperationCallback(string, string, string, string, *ContentAddressableStorage, concurrency.OperationCallback) (<-chan error, error) {
	return immediateDone(), nil
}
func (immediateMockIndexQueue) EnqueueUpdateWithCallback(string, string, string, *ContentAddressableStorage) (<-chan error, error) {
	return immediateDone(), nil
}
func (immediateMockIndexQueue) EnqueueInternal(string, string, string, string, string, *ContentAddressableStorage, concurrency.OperationCallback, bool, bool) (<-chan error, error) {
	return immediateDone(), nil
}
func (immediateMockIndexQueue) EnqueueRemove(string, string, *ContentAddressableStorage) (<-chan error, error) {
	return immediateDone(), nil
}
func (immediateMockIndexQueue) EnqueueRemoveNoWait(string, string, *ContentAddressableStorage) error {
	return nil
}
func (immediateMockIndexQueue) FlushKind(string, time.Duration) error {
	return nil
}

func TestExtended_NoopStorageMetricsAndLock(t *testing.T) {
	var m noopStorageMetrics
	m.RecordCreate(time.Millisecond, true)
	m.RecordRead(time.Millisecond, true)
	m.RecordUpdate(time.Millisecond, true)
	m.RecordDelete(time.Millisecond, true)
	m.RecordIndexFileLock(true, time.Millisecond)
	m.RecordIndexSave(time.Millisecond, nil, 1)
	m.RecordSetMapping(time.Millisecond, nil, true)
	m.RecordIndexReload()
	m.RecordRemoveMapping(time.Millisecond, nil)

	var h noopLockHandle
	if err := h.Release(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	origStrat := NewAutoCleanupStrategy
	defer func() { NewAutoCleanupStrategy = origStrat }()

	NewAutoCleanupStrategy = nil
	s1 := getSafeLockStrategy()
	if s1 == nil {
		t.Fatal("expected non-nil lock strategy")
	}

	NewAutoCleanupStrategy = func() LockStrategy {
		return noopLockStrategy{}
	}
	s2 := getSafeLockStrategy()
	if s2 == nil {
		t.Fatal("expected non-nil lock strategy")
	}
	handle, err := s2.AcquireLock("dummy", time.Second)
	if err != nil || handle == nil {
		t.Fatalf("unexpected AcquireLock result: %v, %v", handle, err)
	}

	origMetrics := GetMetrics
	defer func() { GetMetrics = origMetrics }()
	GetMetrics = nil
	sm := getSafeMetrics()
	if sm == nil {
		t.Fatal("expected non-nil metrics")
	}
}

func TestExtended_IndexSetMappingsAndScanForObjectID(t *testing.T) {
	dir := t.TempDir()
	origMetrics := GetMetrics
	origPending := NoteObjectIDCachePending
	origVis := GetCASPendingVisibilityCache
	origHigh := IsHighVolumeKindForCache
	defer func() {
		GetMetrics = origMetrics
		NoteObjectIDCachePending = origPending
		GetCASPendingVisibilityCache = origVis
		IsHighVolumeKindForCache = origHigh
	}()

	GetMetrics = func() StorageMetrics { return noopStorageMetrics{} }
	NoteObjectIDCachePending = func(string, string, string, string, string, string) {}
	GetCASPendingVisibilityCache = func(string) PendingVisibilityCache { return noopPendingCache{} }
	IsHighVolumeKindForCache = func(string) bool { return false }

	queue := immediateMockIndexQueue{}
	cas := NewContentAddressableStorage(dir, "task", queue)
	cas.SetPostSyncCallback(func(string, string, string, string) error { return nil })

	// Test SetMappings directly
	mappings := map[string]string{
		"obj-scan-1": "1111111111111111111111111111111111111111111111111111111111111111",
		"obj-scan-2": "2222222222222222222222222222222222222222222222222222222222222222",
	}
	bucketKeys := map[string]string{
		"obj-scan-1": "b1",
		"obj-scan-2": "b2",
	}
	if err := cas.index.SetMappings(mappings, bucketKeys); err != nil {
		t.Fatalf("SetMappings failed: %v", err)
	}

	// Verify StringMapsEqual
	if !StringMapsEqual(mappings, mappings) {
		t.Errorf("StringMapsEqual failed for identical maps")
	}
	diffMap := map[string]string{"other": "val"}
	if StringMapsEqual(mappings, diffMap) {
		t.Errorf("StringMapsEqual unexpectedly returned true for different maps")
	}

	// Test scanForObjectID by writing a file on disk that is NOT in index
	scanID := "SCAN-TARGET-ID"
	hexHash := "3333333333333333333333333333333333333333333333333333333333333333"
	yamlContent := "id: " + scanID + "\nkind: task\nname: scan-test\n"
	hashFile := filepath.Join(dir, hexHash+".yaml")
	if err := fileutil.WriteFile(hashFile, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write dummy hash file: %v", err)
	}

	foundHash, err := cas.GetHashForID(scanID)
	if err != nil {
		t.Fatalf("GetHashForID failed to discover scanned object: %v", err)
	}
	if foundHash != hexHash {
		t.Errorf("GetHashForID returned %q, expected %q", foundHash, hexHash)
	}

	// Test negative miss caching
	missingHash, err := cas.GetHashForID("NON-EXISTENT-ID-FOR-MISS")
	if missingHash != "" || err == nil {
		t.Errorf("expected empty hash and error for non-existent id, got hash=%q, err=%v", missingHash, err)
	}

	// Test commitLiveIdentity and invokeCASPostSync
	// 1. Missing arguments
	if err := cas.commitLiveIdentity("", "", ""); err == nil {
		t.Errorf("expected error for commitLiveIdentity with empty params")
	}
	if err := cas.invokeCASPostSync("", "", ""); err == nil {
		t.Errorf("expected error for invokeCASPostSync with empty params")
	}

	// 2. identityCacheRequired with nil postSyncCB
	cas.SetPostSyncCallback(nil)
	cas.identityCacheRequired = true
	if err := cas.invokeCASPostSync("obj-req", "hash-req", "/dummy/path"); err == nil {
		t.Errorf("expected error when identityCacheRequired is true and callback is nil")
	}
	cas.identityCacheRequired = false

	// 3. postSyncCB returning error
	cas.SetPostSyncCallback(func(string, string, string, string) error {
		return errfmt.Errorf("synthetic post sync fail")
	})
	if err := cas.commitLiveIdentity("obj-err", "hash-err", "/dummy/path"); err == nil {
		t.Errorf("expected error when postSyncCB fails")
	}

	// WriteFileWithSync: fast path when file already exists with same content
	existingPath := filepath.Join(dir, "existing-file.txt")
	testData := []byte("hello world sync")
	if err := cas.WriteFileWithSync(existingPath, testData); err != nil {
		t.Fatalf("first WriteFileWithSync failed: %v", err)
	}
	// Calling again triggers fast path (file already exists and matches hash)
	if err := cas.WriteFileWithSync(existingPath, testData); err != nil {
		t.Fatalf("second WriteFileWithSync failed: %v", err)
	}
	// Calling with mismatched data triggers error path
	if err := cas.WriteFileWithSync(existingPath, []byte("mismatched data")); err == nil {
		t.Errorf("expected hash mismatch error for WriteFileWithSync")
	}

	// sweepAfterDurableBlob
	cas.sweepAfterDurableBlob("SWEEP-OBJ", "0000000000000000000000000000000000000000000000000000000000000000")

	// OldestIDs exercising heap.down replacement path
	testIdx := &IDIndex{
		Mu:        sync.RWMutex{},
		CreatedAt: make(map[string]string),
	}
	// Add 5 items with distinct timestamps, requesting limit 2
	testIdx.CreatedAt["T1"] = "2026-09-01T10:00:00Z"
	testIdx.CreatedAt["T2"] = "2026-09-02T10:00:00Z"
	testIdx.CreatedAt["T3"] = "2026-09-03T10:00:00Z"
	testIdx.CreatedAt["T4"] = "2026-09-04T10:00:00Z"
	testIdx.CreatedAt["T0"] = "2026-09-01T08:00:00Z"
	oldestResult := testIdx.OldestIDs(2)
	if len(oldestResult) != 2 {
		t.Fatalf("expected 2 oldest items, got %v", oldestResult)
	}

	// LoadLocked error branches
	corruptIndexFile := filepath.Join(dir, "corrupt-index.json")
	if err := fileutil.WriteFile(corruptIndexFile, []byte("{invalid-json"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}
	corruptIdx := &IDIndex{FilePath: corruptIndexFile}
	if err := corruptIdx.LoadLocked(); err == nil {
		t.Errorf("expected JSON parse error for corrupt index file")
	}

	// Create error cases
	if err := cas.Create("EMPTY-OBJ", []byte("")); err == nil {
		t.Errorf("expected error for empty content create")
	}

	// Read error cases
	if _, err := cas.Read(""); err == nil {
		t.Errorf("expected error for empty objectID read")
	}

	// Nil CAS ErasePending and GetKind branches
	var nilCAS *ContentAddressableStorage
	if nilCAS.IsErasePending("any") {
		t.Errorf("expected false for nil CAS IsErasePending")
	}
	if _, ok := nilCAS.GetErasePendingEventID("any"); ok {
		t.Errorf("expected false for nil CAS GetErasePendingEventID")
	}
	nilCAS.SetErasePending("any", "evt")
	nilCAS.ClearErasePending("any")
	if nilCAS.SnapshotErasePending() != nil {
		t.Errorf("expected nil for nil CAS SnapshotErasePending")
	}
	if nilCAS.GetKind() != "" {
		t.Errorf("expected empty string for nil CAS GetKind")
	}
	nilCAS.RemoveIndexMappingInMemory("any")

	// Nil index CAS ErasePending branches
	casNoIdx := &ContentAddressableStorage{kindDir: dir, kind: "task"}
	if casNoIdx.IsErasePending("any") {
		t.Errorf("expected false for casNoIdx IsErasePending")
	}
	if _, ok := casNoIdx.GetErasePendingEventID("any"); ok {
		t.Errorf("expected false for casNoIdx GetErasePendingEventID")
	}
	casNoIdx.SetErasePending("any", "evt")
	casNoIdx.ClearErasePending("any")
	if casNoIdx.SnapshotErasePending() != nil {
		t.Errorf("expected nil for casNoIdx SnapshotErasePending")
	}

	// compressPayload and decompressPayload direct test
	rawCompress := []byte("payload to compress and decompress")
	compressed, err := compressPayload(rawCompress)
	if err != nil {
		t.Fatalf("compressPayload failed: %v", err)
	}
	decompressed, err := decompressPayload(compressed)
	if err != nil || string(decompressed) != string(rawCompress) {
		t.Fatalf("decompressPayload failed: got %s, err %v", string(decompressed), err)
	}

	// BatchDelete with index miss discovered on disk
	missObjID := "BATCH-MISS-OBJ"
	missHex := "5555555555555555555555555555555555555555555555555555555555555555"
	missYaml := "id: " + missObjID + "\nkind: task\n"
	missPath := filepath.Join(dir, missHex+".yaml")
	if err := fileutil.WriteFile(missPath, []byte(missYaml), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write miss YAML: %v", err)
	}
	// Calling BatchDelete with an ID that is on disk but not in index
	if err := cas.BatchDelete([]string{missObjID}); err != nil {
		t.Fatalf("BatchDelete with scan discovery failed: %v", err)
	}
	// BatchDelete with empty slice
	if err := cas.BatchDelete(nil); err != nil {
		t.Fatalf("BatchDelete nil slice failed: %v", err)
	}

	// Delete non-existent object ID
	if err := cas.Delete("NON-EXISTENT-ID-FOR-DELETE"); err == nil {
		t.Errorf("expected error deleting non-existent object ID")
	}

	// GetKindDir
	if cas.GetKindDir() != dir {
		t.Errorf("GetKindDir returned %q, expected %q", cas.GetKindDir(), dir)
	}

	// UpdateWithIDChange same ID
	sameData := []byte("id: SAME-ID\nkind: task\n")
	_ = cas.Create("SAME-ID", sameData)
	if err := cas.UpdateWithIDChange("SAME-ID", "SAME-ID", sameData); err != nil {
		t.Errorf("UpdateWithIDChange with same ID failed: %v", err)
	}

	// UpdateWithIDChange non-existent old ID
	if err := cas.UpdateWithIDChange("NO-SUCH-OLD-ID", "NEW-ID", sameData); err == nil {
		t.Errorf("expected error for non-existent old ID")
	}
}
