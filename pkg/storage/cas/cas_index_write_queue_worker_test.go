package cas

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/storage/filecas"

	"time"
)

func TestCASIndexWriteQueueWorker_ShutdownPropagatesFinalBatchError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A directory cannot be replaced by the index file's atomic rename, forcing
	// processBatch to fail without relying on platform-specific permissions.
	indexPath := t.TempDir()
	idx := &filecas.IDIndex{
		Version:  "1",
		Kind:     "criteria",
		Mappings: map[string]string{},
		FilePath: indexPath,
	}
	cas := filecas.NewContentAddressableStorageWithIndex(idx)
	done := make(chan error, 1)
	queue := make(chan *indexUpdateRequest, 1)
	queue <- &indexUpdateRequest{
		objectID:  "CRIT-SHUTDOWN-ERROR",
		hash:      "expected-hash",
		done:      done,
		startedAt: time.Now(),
	}
	close(queue)

	iq := &indexQueue{
		kind:      "criteria",
		queue:     queue,
		cas:       cas,
		batchSize: 10,
		timeout:   time.Hour,
		ctx:       ctx,
	}
	iq.startWorker()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected final batch persistence error, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for final batch completion")
	}
}
