package cas_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"time"

	"github.com/stretchr/testify/require"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestCASMutexLockHierarchyStress tests concurrent CAS reads, updates, and erase-tombstone
// operations to guarantee that the cas.mu -> cas.GetIndex().mu lock hierarchy never deadlocks
// under concurrent worker pressure (F-REL-CAS-INDEX-LOCK-RETRY-001, F-CNC-ASYNC-MUTEX-LOCK-ORDER-001).
func TestCASMutexLockHierarchyStress(t *testing.T) {
	tempDir, err := fileutil.MkdirTemp("", "cas-lock-hierarchy-*")
	require.NoError(t, err)
	defer fileutil.RemoveAll(tempDir)

	kindDir := filepath.Join(tempDir, "criteria")
	require.NoError(t, fileutil.MkdirAll(kindDir, 0755))

	cas := filecas.NewContentAddressableStorage(kindDir, "criteria")

	const numWorkers = 8
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for worker := 0; worker < numWorkers; worker++ {
		w := worker
		goroutinelabels.NewGoroutine("storage.cas_lock_order_worker", "testing concurrent CAS operations").
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					id := fmt.Sprintf("CRIT-WORKER-%d-%d", w, i%10)
					eventID := fmt.Sprintf("EVT-LOCK-TEST-%d-%d", w, i)

					// Exercise tombstone state
					cas.SetErasePending(id, eventID)
					_ = cas.IsErasePending(id)
					_ = cas.SnapshotErasePending()
					cas.ClearErasePending(id)

					// Exercise in-memory mapping operations
					_ = cas.GetIndex()
				}
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("storage.cas_lock_order_wait", "waiting for workers to finish").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})

	select {
	case <-waitDone:
	case <-time.After(10 * time.Second):
		t.Fatal("TestCASMutexLockHierarchyStress deadlocked or timed out waiting for worker goroutines")
	}
}
