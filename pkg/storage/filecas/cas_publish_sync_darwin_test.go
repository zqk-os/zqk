//go:build darwin

package filecas

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestQueueOrSync_FallsBackWhenQueueCannotAccept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas-object.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-1\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	if err := queueOrSync(make(chan *fileutil.File), f); err != nil {
		t.Fatalf("queueOrSync fallback: %v", err)
	}
	if _, err := f.Stat(); err == nil {
		t.Fatal("fallback left duplicated file descriptor open")
	}
}

func TestQueueOrSync_QueuesWhenCapacityIsAvailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas-object.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-1\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	queue := make(chan *fileutil.File, 1)

	if err := queueOrSync(queue, f); err != nil {
		t.Fatalf("queueOrSync enqueue: %v", err)
	}
	select {
	case queued := <-queue:
		if queued != f {
			t.Fatal("queueOrSync enqueued a different file descriptor")
		}
		if err := syncAndClose(queued); err != nil {
			t.Fatalf("sync queued descriptor: %v", err)
		}
	default:
		t.Fatal("queueOrSync did not enqueue with available capacity")
	}
}

func TestQueueOrSync_PropagatesFallbackSyncError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cas-object.yaml")
	if err := fileutil.WriteFile(path, []byte("id: OBJ-1\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}

	if err := queueOrSync(make(chan *fileutil.File), f); err == nil {
		t.Fatal("queueOrSync hid fallback sync failure")
	}
}
