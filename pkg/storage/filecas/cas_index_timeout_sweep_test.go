package filecas

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// hangIndexQueue accepts index updates but never completes them, forcing Create/Update
// to hit casIndexUpdateWaitTimeout after the hash YAML is already on disk.
type hangIndexQueue struct{}

func hangingDone() <-chan error {
	return make(chan error)
}

func (hangIndexQueue) EnqueueUpdateWithOperationCallbackAndCreatedAt(string, string, string, string, string, *ContentAddressableStorage, concurrency.OperationCallback) (<-chan error, error) {
	return hangingDone(), nil
}
func (hangIndexQueue) EnqueueUpdateWithOperationCallback(string, string, string, string, *ContentAddressableStorage, concurrency.OperationCallback) (<-chan error, error) {
	return hangingDone(), nil
}
func (hangIndexQueue) EnqueueUpdateWithCallback(string, string, string, *ContentAddressableStorage) (<-chan error, error) {
	return hangingDone(), nil
}
func (hangIndexQueue) EnqueueInternal(string, string, string, string, string, *ContentAddressableStorage, concurrency.OperationCallback, bool, bool) (<-chan error, error) {
	return hangingDone(), nil
}
func (hangIndexQueue) EnqueueRemove(string, string, *ContentAddressableStorage) (<-chan error, error) {
	return hangingDone(), nil
}
func (hangIndexQueue) EnqueueRemoveNoWait(string, string, *ContentAddressableStorage) error {
	return nil
}
func (hangIndexQueue) FlushKind(string, time.Duration) error { return nil }

func TestUpdate_IndexWaitTimeout_SweepsSupersededBlob(t *testing.T) {
	prevTimeout := casIndexUpdateWaitTimeout
	casIndexUpdateWaitTimeout = 25 * time.Millisecond
	prevMetrics := GetMetrics
	GetMetrics = func() StorageMetrics { return noopCASMetrics{} }
	prevPending := NoteObjectIDCachePending
	NoteObjectIDCachePending = func(string, string, string, string, string, string) {}
	prevVis := GetCASPendingVisibilityCache
	GetCASPendingVisibilityCache = func(string) PendingVisibilityCache { return noopPendingCache{} }
	prevHV := IsHighVolumeKindForCache
	IsHighVolumeKindForCache = func(string) bool { return false }
	t.Cleanup(func() {
		casIndexUpdateWaitTimeout = prevTimeout
		GetMetrics = prevMetrics
		NoteObjectIDCachePending = prevPending
		GetCASPendingVisibilityCache = prevVis
		IsHighVolumeKindForCache = prevHV
	})

	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, objects.KindBacklogItem, hangIndexQueue{})
	cas.SetPostSyncCallback(func(string, string, string, string) error { return nil })
	objectID := "BLI-timeout-sweep-001"
	data1 := []byte("id: " + objectID + "\nkind: " + objects.KindBacklogItem + "\ntitle: First\nstatus: planned\n")
	if err := cas.Create(objectID, data1); err == nil {
		t.Fatal("Create should time out when the index queue never completes")
	}
	data2 := []byte("id: " + objectID + "\nkind: " + objects.KindBacklogItem + "\ntitle: Second restamp\nstatus: planned\n")
	if err := cas.Update(objectID, data2); err == nil {
		t.Fatal("Update should time out when the index queue never completes")
	}
	files, err := filepath.Glob(filepath.Join(kindDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("timed-out Update must still leave exactly one blob, found %d: %v", len(files), files)
	}
	if !fileutil.Exists(files[0]) {
		t.Fatal("keeper blob missing")
	}
}

type noopPendingCache struct{}

func (noopPendingCache) LookupPending(string) (PendingVisibilityEntry, bool) {
	return PendingVisibilityEntry{}, false
}
func (noopPendingCache) PublishPending(string, string, string, string) error { return nil }
func (noopPendingCache) EvictPending(string) error                           { return nil }
func (noopPendingCache) SnapshotPending() []PendingVisibilityEntry           { return nil }

type noopCASMetrics struct{}

func (noopCASMetrics) RecordCreate(time.Duration, bool)            {}
func (noopCASMetrics) RecordRead(time.Duration, bool)              {}
func (noopCASMetrics) RecordUpdate(time.Duration, bool)            {}
func (noopCASMetrics) RecordDelete(time.Duration, bool)            {}
func (noopCASMetrics) RecordIndexFileLock(bool, time.Duration)     {}
func (noopCASMetrics) RecordIndexSave(time.Duration, error, int)   {}
func (noopCASMetrics) RecordSetMapping(time.Duration, error, bool) {}
func (noopCASMetrics) RecordIndexReload()                          {}
func (noopCASMetrics) RecordRemoveMapping(time.Duration, error)    {}
