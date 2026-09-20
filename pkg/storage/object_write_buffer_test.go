// Unit tests for object write buffer. Validate enqueue, peek, remove, get pending, list IDs.
// Write-behind is disabled in most tests; these tests exercise the buffer in isolation.

package storage

import (
	"testing"
)

func TestObjectWriteBuffer_EnqueuePeekRemove(t *testing.T) {
	buf := NewObjectWriteBuffer()
	buf.Enqueue("create", "backlog_item", "bli-001", 1, []byte("id: bli-001"))
	if buf.Len() != 1 {
		t.Fatalf("Len: got %d, want 1", buf.Len())
	}
	op := buf.Peek()
	if op == nil || op.Op != "create" || op.ID != "bli-001" {
		t.Fatalf("Peek: got %+v", op)
	}
	buf.RemoveFront(op)
	if buf.Len() != 0 || buf.Peek() != nil {
		t.Errorf("after RemoveFront: Len=%d Peek=%v", buf.Len(), buf.Peek())
	}
}

func TestObjectWriteBuffer_GetPending(t *testing.T) {
	buf := NewObjectWriteBuffer()
	buf.Enqueue("create", "backlog_item", "bli-001", 1, []byte("data1"))
	p := buf.GetPending("backlog_item", "bli-001")
	if p == nil || p.Op != "create" || string(p.Data) != "data1" {
		t.Fatalf("GetPending: got %+v", p)
	}
	if buf.GetPending("other_kind", "bli-001") != nil {
		t.Error("GetPending wrong kind should be nil")
	}
	buf.Enqueue("delete", "backlog_item", "bli-001", 2, nil)
	p2 := buf.GetPending("backlog_item", "bli-001")
	if p2 == nil || p2.Op != "delete" {
		t.Fatalf("GetPending after delete: got %+v", p2)
	}
}

func TestObjectWriteBuffer_ListPendingIDs(t *testing.T) {
	buf := NewObjectWriteBuffer()
	buf.Enqueue("create", "backlog_item", "bli-001", 1, nil)
	buf.Enqueue("create", "backlog_item", "bli-002", 2, nil)
	buf.Enqueue("delete", "backlog_item", "bli-001", 3, nil)
	ids := buf.ListPendingIDs("backlog_item")
	if len(ids) != 1 {
		t.Fatalf("ListPendingIDs: got %v, want 1 id (bli-002; bli-001 latest is delete)", ids)
	}
	if ids[0] != "bli-002" {
		t.Errorf("ListPendingIDs: got %v", ids)
	}
}

func TestObjectWriteBuffer_PendingDeletesForKind(t *testing.T) {
	buf := NewObjectWriteBuffer()
	buf.Enqueue("create", "backlog_item", "bli-001", 1, nil)
	buf.Enqueue("delete", "backlog_item", "bli-001", 2, nil)
	deletes := buf.PendingDeletesForKind("backlog_item")
	if !deletes["bli-001"] {
		t.Errorf("PendingDeletesForKind: want bli-001 in deletes, got %v", deletes)
	}
	buf.Enqueue("create", "backlog_item", "bli-001", 3, nil)
	deletes2 := buf.PendingDeletesForKind("backlog_item")
	if deletes2["bli-001"] {
		t.Error("after create again, bli-001 should not be in PendingDeletes")
	}
}

func TestAppendToWALAndBuffer(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	defer wal.Close()
	buf := NewObjectWriteBuffer()

	err = AppendToWALAndBuffer(wal, buf, "create", "backlog_item", "bli-001", []byte("yaml: data"), true)
	if err != nil {
		t.Fatalf("AppendToWALAndBuffer: %v", err)
	}
	if buf.Len() != 1 {
		t.Fatalf("buffer Len: got %d, want 1", buf.Len())
	}
	p := buf.GetPending("backlog_item", "bli-001")
	if p == nil || string(p.Data) != "yaml: data" {
		t.Fatalf("GetPending after AppendToWALAndBuffer: %+v", p)
	}
}

func TestAppendDeleteToWAL_ReturnsClosedWALErrorWithoutBuffering(t *testing.T) {
	wal, err := NewObjectWAL(t.TempDir())
	if err != nil {
		t.Fatalf("NewObjectWAL: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	buf := NewObjectWriteBuffer()
	storage := &FileObjectStorage{wal: wal, writeBuf: buf}

	if err := storage.appendDeleteToWAL("backlog_item", "BLI-1"); err == nil {
		t.Fatal("appendDeleteToWAL returned nil for a closed WAL")
	}
	if buf.Len() != 0 {
		t.Fatalf("failed WAL append enqueued %d operation(s)", buf.Len())
	}
}

func TestObjectWriteBuffer_EnqueueFromWALRecord(t *testing.T) {
	buf := NewObjectWriteBuffer()
	rec := &WALRecord{Op: "create", Kind: "backlog_item", ID: "bli-001", Seq: 1, DataB64: "eWRhdGE="}
	if err := buf.EnqueueFromWALRecord(rec); err != nil {
		t.Fatalf("EnqueueFromWALRecord: %v", err)
	}
	p := buf.GetPending("backlog_item", "bli-001")
	if p == nil || string(p.Data) != "ydata" {
		t.Fatalf("EnqueueFromWALRecord: got %+v", p)
	}
}
