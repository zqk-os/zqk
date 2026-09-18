// Extracted from hash_registry.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func (hr *HashRegistry) wakeWorkerIfNeeded() {
	// Check if context is already cancelled before starting worker
	if hr.ctx.Err() != nil {
		// Context cancelled - don't start worker
		return
	}

	// Try to set workerRunning from 0 to 1 (atomic compare-and-swap)
	if hr.workerRunning.CompareAndSwap(0, 1) {
		// Double-check context after acquiring lock (might have been cancelled)
		if hr.ctx.Err() != nil {
			// Context cancelled - reset flag and don't start worker
			hr.workerRunning.Store(0)
			return
		}

		// Successfully acquired lock - start worker
		wgID := fmt.Sprintf(DescHashRegWorker, hr.kind)
		wg := hr.wgManager.CreateGroupForGoroutine(wgID, DescHashRegSaveWorker)
		goroutinelabels.NewGoroutine(DescHashRegSaveWorker, DescProcessBatchHashReg).
			WithWaitGroup(wg).
			StartSimple(hr.startSaveWorker)
	}
	// If worker is already running, do nothing (worker will process the new item)
}

// startSaveWorker starts the background worker that processes save operations in batches
// Implements on-demand pattern: processes batches until idle timeout, then shuts down
// This ensures all saves are serialized and prevents race conditions
// Batches are processed when either:
//   - Batch size reaches saveBatchSize
//   - saveBatchTimeout (100ms) elapses
//
// Only the latest snapshot in each batch is saved (since each request contains the full registry snapshot)

// Note: wg.Done() is called by goroutinelabels.WithWaitGroup() on goroutine exit

// Check context immediately before starting work

// Context already cancelled - exit immediately

// Emit worker start event via coordinator

// Use timer for idle detection (reset when work arrives)

// Context cancelled - stop timers immediately and exit

// Process any pending batch

// Drain any remaining requests and close their done channels

// Channel closed - shutdown initiated (event-driven)
// Stop timers immediately

// Process any pending batch

// Drain any remaining requests and close their done channels

// New work arrived - reset timers

// Non-blocking drain: the channel may already be empty if the case <-batchTimer.C
// branch consumed the value without resetting the timer (e.g. empty batch path).
// A blocking <-batchTimer.C here would deadlock the worker.

// Batch is full - process immediately

// Reset batch

// Batch timeout - process current batch
// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting

// Context cancelled - stop timers immediately and exit

// Process any pending batch

// Drain any remaining requests and close their done channels

// Reset batch

// DO NOT reset batchTimer here if batch is empty!
// Resetting it causes 5ms polling loops which thrash the CPU.
// The non-blocking drain in the req arrival path handles the expired state safely.

// Idle timeout - check if we should shut down (on-demand pattern)
// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting

// Context cancelled - stop timers immediately and exit

// Process any pending batch

// Drain any remaining requests and close their done channels

// Been idle long enough - shut down worker
// Prevent race condition: Save() might enqueue a request right after len(hr.saveQueue) check

// Work arrived between our check and marking worker as stopped

// Reclaimed worker status - stay alive and process

// Not idle long enough - reset timer

// There's work - reset idle timer

// drainQueueAndCloseChannels drains the save queue and closes all done channels
// This ensures Save() calls don't wait forever when worker exits
// Note: Queue may be closed, so we use non-blocking reads

// Drain queue using non-blocking reads (queue may be closed)

// Queue closed - can't drain more

// Send error and close channel (non-blocking)

// Queue empty or closed - done draining

// processBatch processes a batch of save requests.
// Only the latest snapshot is persisted; we nil out other requests' .data so the GC can reclaim
// those maps immediately (reduces peak memory when batch holds many full registry copies).

// Process only the latest snapshot (since each request contains full registry)

// Release other requests' data so GC can reclaim; we only need latestReq.data for processSave

// Emit batch start event via coordinator

// Process save

// Emit batch completion event via coordinator

// Signal completion to all requests in the batch
// All requests in the batch get the same result since we only save the latest

// Send error (buffered channel, non-blocking)

// Always close channel so Save() doesn't wait forever

// Load loads the hash registry from disk
// OPTIMIZATION: Skip re-read when the file's mtime matches the mtime from the last successful load.
// Do not compare file mtime to lastLoadTime (wall clock): coarse mtimes can appear "before"
// lastLoadTime even when the file changed, causing stale in-memory hashes.
// CRITICAL FIX: Release lock before doing blocking file I/O to prevent deadlocks

// SaveAsync enqueues a save request and returns immediately without waiting for completion.
// Use during WAL replay so apply path stays fast; the background worker persists when it can.
// Caller must not rely on durability before the next synchronous Save() or process exit.

// Enqueued; worker will process and close req.done (caller does not wait)

// Queue full - drop this request; next Save() or batch will persist latest state

// Save saves the hash registry to disk
// Uses a thread-safe queue to serialize all save operations, preventing race conditions
// from concurrent saves. The method enqueues a save request and waits for completion.
// Wakes worker if needed (on-demand pattern)
// Returns ErrShutdownInProgress if shutdown has been initiated

// CRITICAL: Check context FIRST (fast path, prevents race condition)
// This ensures we never start workers after shutdown is initiated

// Defensive check: also verify coordinator shutdown status (skip for test registries)

// Double-check context before starting worker (prevent TOCTOU race)

// Acquire read lock, copy data, release lock BEFORE enqueuing

// Create save request

// Enqueue save request (non-blocking due to buffered channel)

// Request enqueued successfully
// Wakes worker if needed (on-demand pattern). MUST happen after enqueue.

// Context cancelled while enqueueing - don't wait for completion

// Channel is full (shouldn't happen with buffer size 100, but handle gracefully)

// Wait for completion with a bounded timeout. file.Sync() is no longer called inside processSave
// so the worker completes quickly (write + atomic rename). 60s is a safety net for queue overhead.

// Timeout waiting for worker to process the request. This should be rare now that
// file.Sync() (F_FULLFSYNC on macOS) has been removed from processSave; the worker
// should complete each batch in milliseconds. Log diagnostics to aid investigation.

// SaveWithContext is like Save but stops waiting when ctx is cancelled (e.g. init timeout).
// Use this during storage init so the daemon does not hang indefinitely on HashRegistry I/O.
// If ctx is nil, behaves as Save() (no timeout).

// Same checks as Save()

// Wait for completion; respect both registry shutdown and caller context

// processSave performs the actual file I/O to save the hash registry
// This is called by the background worker, ensuring all saves are serialized.
// Uses json.Marshal (not MarshalIndent) for speed and smaller I/O on large registries.

// Ensure directory exists before writing

// Use atomic file write pattern (write to temp file, then rename)

// file.Sync() (F_FULLFSYNC on macOS) is intentionally omitted here.
// It blocked for 10+ minutes when Time Machine or Spotlight was active on the
// .zqk/process directory, causing object-creation rollbacks for the hash registry
// even though the CAS write had already succeeded.
// The hash registry is a derived/rebuildable cache (see `system check --auto-fix`);
// hardware-flush durability is not required. The atomic write-to-tmp then rename
// pattern provides sufficient crash-safety within normal OS operation.

// Clean up temp file on error

// Ensure directory still exists before rename (defensive check for race conditions)
// This can happen if the directory was deleted between creation and rename

// Sync parent directory to ensure it's visible to os.Rename on macOS
// On macOS, directory metadata caching can cause os.Rename to fail if directory
// was just created, even though MkdirAll succeeded

//nolint:errcheck // Best effort - directory sync is not critical

// Verify directory exists and is accessible before rename
// This helps catch cases where directory was deleted between MkdirAll and rename

// Atomic rename: on most filesystems, rename is atomic
// Retry with exponential backoff if directory doesn't exist (race condition)
// This handles cases where the directory is deleted between checks

// Always ensure directory exists before rename (even on retries)
// This is critical for CAS-based files in date-based directories that might not exist yet

// Sync directory after creation/recreation to ensure it's visible

// Verify directory exists and is accessible before rename

// Verify temp file exists before rename (defensive check)
// The temp file should exist since we just wrote it, but verify to catch issues early

// Attempt rename

// Only retry on "no such file or directory" errors

// Non-recoverable error - don't retry

// Use default error checker

// Touch and sync parent dir (best-effort; helps cache-staleness detection).

// Best effort

// Update file mtime after save (for Load() optimization)
// This ensures subsequent Load() calls know the file was just updated

// emitBatchEvent emits a batch processing event via coordinator

// Coordinator not available - skip

// Lock-free reads using atomic.Value

// Can't emit without project root and storage

// Emit via coordinator (async, non-blocking)

// GetHash returns the hash for a given filename, or empty string if not found
