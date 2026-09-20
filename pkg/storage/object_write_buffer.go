// Package storage: in-memory write buffer for Option B write-behind.
// Holds pending create/update/delete ops until the background worker persists them.
// Read/List merge this buffer with on-disk state.

package storage

import (
	"context"
	"encoding/base64"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// objectWriteBufferLockLogger returns the instrumented lock logger used for ObjectWriteBuffer
// critical sections (same profile as other storage mutex instrumentation).
func objectWriteBufferLockLogger() concurrency.LockLogger {
	return logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
}

// PendingOp represents one buffered write (create, update, or delete).
type PendingOp struct {
	Op   string // "create", "update", "delete"
	Kind string
	ID   string
	Seq  int64
	Data []byte // YAML bytes for create/update; nil for delete
}

// ObjectWriteBuffer holds pending ops in FIFO order and by (kind, id) for lookup.
// Thread-safe. Optional back-pressure: SetMaxBacklog(n) to block Enqueue when len >= n.
type ObjectWriteBuffer struct {
	mu     sync.Mutex
	queue  []*PendingOp
	byKey  map[string]*PendingOp // key = kind + "\x00" + id
	maxLen int                   // if > 0, Enqueue blocks until len < maxLen
	cond   *sync.Cond
}

// NewObjectWriteBuffer returns a new empty write buffer.
func NewObjectWriteBuffer() *ObjectWriteBuffer {
	b := &ObjectWriteBuffer{
		queue:  nil,
		byKey:  make(map[string]*PendingOp),
		maxLen: 0,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// SetMaxBacklog sets the maximum pending ops; Enqueue blocks when len >= n until the worker drains.
// Pass 0 to disable back-pressure. Call once after construction (e.g. from write-behind worker).
func (b *ObjectWriteBuffer) SetMaxBacklog(n int) {
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferSetMaxBacklog, objectWriteBufferLockLogger(), func() error {
		b.maxLen = n
		if b.cond != nil && n > 0 {
			b.cond.Broadcast()
		}
		return nil
	})
}

func key(kind, id string) string { return kind + "\x00" + id }

// Enqueue appends a pending op and indexes it by (kind, id).
// For create/update, data is the YAML bytes; for delete, data is nil.
// If SetMaxBacklog(n) was set and len(queue) >= n, blocks until the worker removes an op.
func (b *ObjectWriteBuffer) Enqueue(op, kind, id string, seq int64, data []byte) {
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferEnqueue, objectWriteBufferLockLogger(), func() error {
		for b.maxLen > 0 && len(b.queue) >= b.maxLen {
			b.cond.Wait()
		}
		entry := &PendingOp{Op: op, Kind: kind, ID: id, Seq: seq, Data: data}
		b.queue = append(b.queue, entry)
		b.byKey[key(kind, id)] = entry
		return nil
	})
}

// Peek returns the next op to apply without removing it, or nil if empty.
func (b *ObjectWriteBuffer) Peek() *PendingOp {
	var out *PendingOp
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferPeek, objectWriteBufferLockLogger(), func() error {
		if len(b.queue) == 0 {
			return nil
		}
		out = b.queue[0]
		return nil
	})
	return out
}

// RemoveFront removes the front op (after worker applied it). Call with the same op as Peek.
func (b *ObjectWriteBuffer) RemoveFront(op *PendingOp) {
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferRemoveFront, objectWriteBufferLockLogger(), func() error {
		if len(b.queue) == 0 || b.queue[0] != op {
			return nil
		}
		k := key(op.Kind, op.ID)
		if b.byKey[k] == op {
			delete(b.byKey, k)
		}
		b.queue = b.queue[1:]
		if b.cond != nil {
			b.cond.Signal()
		}
		return nil
	})
}

// GetPending returns the latest pending op for (kind, id), or nil.
// Used by Read: if create/update return data; if delete return not-found.
func (b *ObjectWriteBuffer) GetPending(kind, id string) *PendingOp {
	var out *PendingOp
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferGetPending, objectWriteBufferLockLogger(), func() error {
		out = b.byKey[key(kind, id)]
		return nil
	})
	return out
}

// ClearPendingForKey removes all queued and indexed pending ops for (kind, id).
// Used after a synchronous CLI delete that already persisted to disk so Read/List do not
// merge stale create/update from the buffer (the WAL may still replay; worker skips no-ops).
func (b *ObjectWriteBuffer) ClearPendingForKey(kind, id string) {
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferClearPendingForKey, objectWriteBufferLockLogger(), func() error {
		delete(b.byKey, key(kind, id))
		if len(b.queue) == 0 {
			return nil
		}
		out := b.queue[:0]
		for _, op := range b.queue {
			if op.Kind == kind && op.ID == id {
				continue
			}
			out = append(out, op)
		}
		b.queue = out
		// Rebuild byKey from remaining queue (last op per key wins, matching Enqueue).
		b.byKey = make(map[string]*PendingOp)
		for _, op := range b.queue {
			b.byKey[key(op.Kind, op.ID)] = op
		}
		if b.cond != nil {
			b.cond.Broadcast()
		}
		return nil
	})
}

// ListPendingIDs returns all IDs whose latest pending op for the kind is create or update
// (used to merge with index for List). Caller should exclude PendingDeletesForKind when merging.
func (b *ObjectWriteBuffer) ListPendingIDs(kind string) []string {
	var ids []string
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferListPendingIds, objectWriteBufferLockLogger(), func() error {
		for k, op := range b.byKey {
			if op.Kind == kind && (op.Op == "create" || op.Op == "update") {
				_ = k
				ids = append(ids, op.ID)
			}
		}
		return nil
	})
	return ids
}

// PendingDeletesForKind returns IDs whose latest pending op for the kind is delete.
func (b *ObjectWriteBuffer) PendingDeletesForKind(kind string) map[string]bool {
	m := make(map[string]bool)
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferPendingDeletesForKind, objectWriteBufferLockLogger(), func() error {
		for _, op := range b.byKey {
			if op.Kind == kind && op.Op == "delete" {
				m[op.ID] = true
			}
		}
		return nil
	})
	return m
}

// Len returns the number of pending ops (for drain and metrics).
func (b *ObjectWriteBuffer) Len() int {
	var n int
	_ = concurrency.RunInLockOrLog(&b.mu, locknames.LockNameObjectWriteBufferLen, objectWriteBufferLockLogger(), func() error {
		n = len(b.queue)
		return nil
	})
	return n
}

// WaitUntilEmpty blocks until the in-memory write buffer is drained (queue length becomes 0).
// This is an event-driven barrier (sync.Cond) so callers do not need polling/sleep loops.
//
// Note: this waits only for the buffer to become empty. It does NOT flush the listing/CAS
// index queues; callers that need full read-your-writes should also flush the index.
func (b *ObjectWriteBuffer) WaitUntilEmpty(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if b.cond == nil {
		// No condition available; fall back to a best-effort loop bounded by ctx.
		deadline := time.Time{}
		if dl, ok := ctx.Deadline(); ok {
			deadline = dl
		}
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if b.Len() == 0 {
				return nil
			}
			if !deadline.IsZero() && time.Now().After(deadline) {
				return errfmt.Errorf(ConstStreamTimeoutWaitingForWriteBufferToDrain)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
	}

	// Ensure we wake cond.Wait() on ctx cancellation.
	stop := make(chan struct{})
	goroutinelabels.NewGoroutine("storage", ConstStreamObjectWriteBufferWaitUntilEmptyInterruptor).StartSimple(func() {
		select {
		case <-ctx.Done():
			b.mu.Lock()
			if b.cond != nil {
				b.cond.Broadcast()
			}
			b.mu.Unlock()
		case <-stop:
		}
	})
	defer close(stop)

	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.queue) > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b.cond.Wait()
	}
	return nil
}

// EnqueueFromWALRecord enqueues from a replayed WAL record (ReplayWAL callback).
func (b *ObjectWriteBuffer) EnqueueFromWALRecord(rec *WALRecord) error {
	data, err := rec.DecodeRecordData()
	if err != nil {
		return errfmt.Newf(ConstStreamDecodeWalRecordData).Wrap(err)
	}
	b.Enqueue(rec.Op, rec.Kind, rec.ID, rec.Seq, data)
	return nil
}

// AppendToWALAndBuffer appends a record to the WAL and enqueues to the buffer.
// For stream-only kinds (StreamStorageEnabledForKind), the WAL record omits payload (minimal record:
// op, kind, id, seq) to shrink WAL bytes; full data is still enqueued for the worker to persist.
// Caller must ensure wal is not nil. Sync is optional (set syncWAL true for durability before return).
func AppendToWALAndBuffer(wal *ObjectWAL, buf *ObjectWriteBuffer, op, kind, id string, data []byte, syncWAL bool) error {
	rec := &WALRecord{Op: op, Kind: kind, ID: id}
	if len(data) > 0 && !StreamStorageEnabledForKind(kind) {
		rec.DataB64 = base64.StdEncoding.EncodeToString(data)
	}
	if err := wal.Append(rec); err != nil {
		return err
	}
	if syncWAL {
		if err := wal.Sync(); err != nil {
			return err
		}
	}
	seq := rec.Seq
	buf.Enqueue(op, kind, id, seq, data)
	return nil
}

// AppendBatchToWALAndBuffer appends multiple records in one WAL batch and enqueues each to the buffer.
// For stream-only kinds, payload is omitted from the WAL record (minimal WAL). Decoded data is
// enqueued for the worker; replay of minimal records will see nil data and skip apply for stream kinds.
// recs are updated with Seq by wal.AppendBatch. Caller must ensure wal and buf are not nil.
func AppendBatchToWALAndBuffer(wal *ObjectWAL, buf *ObjectWriteBuffer, recs []*WALRecord, syncWAL bool) error {
	if len(recs) == 0 {
		return nil
	}
	// Decode payloads before we may clear DataB64 for stream-only kinds
	decoded := make([][]byte, len(recs))
	for i, rec := range recs {
		if rec.DataB64 != emptyValue {
			var err error
			decoded[i], err = base64.StdEncoding.DecodeString(rec.DataB64)
			if err != nil {
				return errfmt.Errorf(ConstStreamDecodeRecordDataForStrStrErr, rec.Kind, rec.ID, err)
			}
		}
		if StreamStorageEnabledForKind(rec.Kind) {
			rec.DataB64 = "" // minimal WAL record
		}
	}
	if err := wal.AppendBatch(recs); err != nil {
		return err
	}
	if syncWAL {
		if err := wal.Sync(); err != nil {
			return err
		}
	}
	for i, rec := range recs {
		buf.Enqueue(rec.Op, rec.Kind, rec.ID, rec.Seq, decoded[i])
	}
	return nil
}
