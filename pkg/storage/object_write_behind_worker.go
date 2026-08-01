// Package storage: background worker that drains the object write buffer and
// persists create/update/delete to CAS, index, hash registry, and audit.

package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// shutdownDrainDeadline is the max time to drain the buffer when Stop() is called.
// Kept short so shutdown completes quickly; remaining WAL entries are replayed on next start.
const shutdownDrainDeadline = 5 * time.Second

// Startup replay: load WAL in chunks and process between chunks to bound memory.
// Increased for large WAL backlogs (e.g. checkpoint at 20k with sequence near 300k).
const startupReplayChunkSize = 8000

// maxReplayBacklog prevents replaying more into the buffer when already above this (avoids unbounded memory).
// Lowered from 60k to reduce peak RSS when replay dominates (sample 2026-03-04: 16.5 GB with JSON/YAML in WAL replay).
const maxReplayBacklog = 25000

// Backlog logging: log when buffer exceeds this so we can monitor for perpetual backlog.
const backlogWarnThreshold = 5000

// WAL compaction: run when buffer is empty and WAL file is large enough.
// Shorter interval so we catch empty-buffer windows more often (large backlog keeps buffer busy).
const compactInterval = 30 * time.Second

// minWALSizeToCompactBytes is the WAL file size at which tryCompactWAL triggers during normal
// operation. Lowered to 100 KB (was 1 MB) so that even compact single-record WAL lines trigger
// compaction while the system is running — prevents replay cost from growing with accumulated runs.
// Post-startup compaction (CompactWAL called directly after replay) bypasses this threshold
// entirely, guaranteeing each process run begins with an empty WAL.
const minWALSizeToCompactBytes = 100 * 1024 // 100 KB

// ObjectWriteBehindWorker drains the write buffer and applies each op to storage.
// Single goroutine per project root to preserve ordering.
type ObjectWriteBehindWorker struct {
	buf         *ObjectWriteBuffer
	wal         *ObjectWAL
	projectRoot string
	storage     *FileObjectStorage
	wakeCh      chan struct{}
	stopCh      chan struct{}
	done        chan struct{}
	mu          sync.Mutex
	running     bool
	// backlogCheckInterval is how often to check for backlog when idle
	backlogCheckInterval time.Duration
	// lastCompactTime is when we last ran WAL compaction (for compactInterval)
	lastCompactTime time.Time
}

// NewObjectWriteBehindWorker creates a worker that will apply ops from buf using storage.
// Call Start() to begin processing; call Stop() to drain and shut down.
func NewObjectWriteBehindWorker(buf *ObjectWriteBuffer, wal *ObjectWAL, projectRoot string, storage *FileObjectStorage) *ObjectWriteBehindWorker {
	return &ObjectWriteBehindWorker{
		buf:                  buf,
		wal:                  wal,
		projectRoot:          projectRoot,
		storage:              storage,
		wakeCh:               make(chan struct{}, 1),
		stopCh:               make(chan struct{}),
		done:                 make(chan struct{}),
		backlogCheckInterval: 250 * time.Millisecond, // Check for backlog when idle (faster catch-up for large WAL backlogs)
	}
}

// Start starts the worker. Idempotent.
// WAL replay runs in a background goroutine so Start() returns immediately; the scheduler
// can start and run jobs without waiting for replay. Replay runs in chunks (replay N ->
// process until empty -> repeat); when replay finishes (or ctx is cancelled), the main
// run() loop is started so the worker continues to process new ops.
// ctx is optional; when set, replay stops when ctx is cancelled but the worker still starts.
func (w *ObjectWriteBehindWorker) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var alreadyRunning bool
	var err_swallow_56 = concurrency.RunInLock(&w.mu, func() error {
		alreadyRunning = w.running
		if !alreadyRunning {
			w.running = true
		}
		return nil
	})
	if err_swallow_56 != nil {
		logging.LogSwallowedError(

			// Cap buffer so Enqueue blocks when backlog exceeds maxReplayBacklog (back-pressure on Create path)
			err_swallow_56)
	}
	if alreadyRunning {
		return nil
	}

	w.buf.SetMaxBacklog(maxReplayBacklog)

	// Run replay in background so caller (e.g. scheduler start) is not blocked.
	// When replay is done (or ctx cancelled), the replay goroutine starts run().
	bud := goroutinelabels.DefaultBudget()
	replayBuilder := goroutinelabels.NewGoroutine(ConstStreamObjectWriteBehindReplay, fmt.Sprintf(ConstStreamWalReplayForStr, w.projectRoot))
	if bud != nil {
		replayBuilder = replayBuilder.WithBudget(bud)
	}
	replayBuilder.StartSimple(func() {
		w.runReplayThenRun(ctx)
	})
	return nil
}

// runReplayThenRun runs the chunked WAL replay, then starts the main run() loop.
// Called from a goroutine so Start() returns immediately and the scheduler can start.
func (w *ObjectWriteBehindWorker) runReplayThenRun(ctx context.Context) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	// Signal storage that we are in replay so saveHashRegistry uses SaveAsync() and stays fast.
	w.storage.SetReplayPhase(true)
	defer w.storage.SetReplayPhase(false)

	startReplay := time.Now()
	var totalReplayed int
	for {
		select {
		case <-ctx.Done():
			StorageLog(logger).Debug(LogEventStorageWriteBehindWalReplayCancelled).
				ProjectRoot(w.projectRoot).
				Int(ConstStreamReplayedSoFar, totalReplayed).
				Log()
			goto startRun
		default:
		}
		appliedSeq, _err_83621405 := ReadAppliedSeq(w.projectRoot)
		if _err_83621405 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamSwallowedErrorValN, _err_83621405).Log()
		}
		replayed, _, err := ReplayWALChunk(w.projectRoot, appliedSeq, startupReplayChunkSize, w.buf.EnqueueFromWALRecord)
		if err != nil {
			StorageLog(logger).Error(LogEventStorageWriteBehindWalReplayFailed, err).
				ProjectRoot(w.projectRoot).
				Log()
			break
		}
		totalReplayed += replayed
		if replayed == 0 {
			break
		}
		w.processOneOrMore(ctx, secCtx, logger)
		select {
		case <-ctx.Done():
			goto startRun
		default:
		}
	}
	if totalReplayed > 0 {
		StorageLog(logger).Info(LogEventStorageWriteBehindWalStartupReplayCompleted).
			Int(ConstStreamTotalReplayed, totalReplayed).
			ProjectRoot(w.projectRoot).
			String("duration", time.Since(startReplay).Round(time.Millisecond).String()).
			Log()
	}

	// Post-startup compaction: compact WAL unconditionally after startup replay.
	// Replay has applied all known WAL entries (applied_seq = max_seq), so compaction
	// clears the file entirely. Without this, each worker replays the full WAL file
	// on every 250ms backlog tick even when all entries are already applied, wasting
	// proportionally more CPU with every accumulated run. Bypasses tryCompactWAL's
	// size/time guards — they exist for the running case, not the post-replay case.
	if err := w.storage.TryCompactWAL(); err != nil {
		StorageLog(logger).Warn(LogEventStorageWriteBehindWalPostStartupCompactionFailed).
			WithError(err).
			ProjectRoot(w.projectRoot).
			Log()
	} else {
		StorageLog(logger).Debug(LogEventStorageWriteBehindWalPostStartupCompactionCompleted).
			ProjectRoot(w.projectRoot).
			Log()
	}

startRun:
	// Main loop: wait for wake/stop, drain buffer, handle backlog and compaction.
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine(ConstStreamObjectWriteBehindWorker, fmt.Sprintf(ConstStreamWriteBehindWorkerForStr, w.projectRoot))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(w.run)
}

// Notify wakes the worker to process pending ops (non-blocking).
func (w *ObjectWriteBehindWorker) Notify() {
	select {
	case w.wakeCh <- struct{}{}:
	default:
	}
}

// run is the main loop: wait for wake or stop, then drain buffer.
// When there's a backlog, processes continuously until caught up.
func (w *ObjectWriteBehindWorker) run() {
	defer close(w.done)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext() // Use system context for write-behind worker

	backlogTicker := time.NewTicker(w.backlogCheckInterval)
	defer backlogTicker.Stop()

	// Periodic backlog metric: log buffer length when non-zero so we can monitor for perpetual backlog
	metricInterval := 60 * time.Second
	metricTicker := time.NewTicker(metricInterval)
	defer metricTicker.Stop()

	// WAL compaction: when buffer is empty and WAL is large, compact to keep file size bounded
	compactTicker := time.NewTicker(compactInterval)
	defer compactTicker.Stop()

	for {
		select {
		case <-w.stopCh:
			StorageLog(logger).Info("ObjectWriteBehindWorker_Transition").String("state", "stop_received").ProjectRoot(w.projectRoot).Log()
			w.drain(ctx, secCtx, logger)
			return
		case <-w.wakeCh:
			StorageLog(logger).Info("ObjectWriteBehindWorker_Transition").String("state", "wake_received").ProjectRoot(w.projectRoot).Log()
			// Process until buffer is empty
			w.processOneOrMore(ctx, secCtx, logger)
			// Opportunistic compaction: buffer is empty now, compact before refilling from backlog
			w.tryCompactWAL(logger)
			// After processing, check if there's more backlog in WAL
			w.processBacklogIfNeeded(ctx, secCtx, logger)
		case <-backlogTicker.C:
			StorageLog(logger).Info("ObjectWriteBehindWorker_Transition").String("state", "backlog_ticker").ProjectRoot(w.projectRoot).Log()
			// When buffer is empty, try compaction before refilling (catch empty windows without new creates)
			if w.buf.Len() == 0 {
				w.tryCompactWAL(logger)
			} else {
				// Retry a stuck head op (e.g. non-retryable apply returned without advancing seq) or
				// continue draining if wake coalesced while work remained.
				w.processOneOrMore(ctx, secCtx, logger)
			}
			// Periodic check: if buffer is empty but WAL has backlog, replay more
			w.processBacklogIfNeeded(ctx, secCtx, logger)
		case <-metricTicker.C:
			if n := w.buf.Len(); n > 0 {
				appliedSeq, _err_83625569 := ReadAppliedSeq(w.projectRoot)
				if _err_83625569 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamSwallowedErrorValN, _err_83625569).Log()
				}
				StorageLog(logger).Info(LogEventStorageWriteBehindBacklogStatus).
					Int("buffer_len", n).
					Int("applied_seq", int(appliedSeq)).
					ProjectRoot(w.projectRoot).
					Log()
			}
		case <-compactTicker.C:
			StorageLog(logger).Info("ObjectWriteBehindWorker_Transition").String("state", "compact_ticker").ProjectRoot(w.projectRoot).Log()
			w.tryCompactWAL(logger)
		}
	}
}

// tryCompactWAL runs WAL compaction when buffer is empty and WAL file size exceeds threshold.
func (w *ObjectWriteBehindWorker) tryCompactWAL(logger logging.Logger) {
	if w.buf.Len() > 0 {
		return
	}
	var lastCompact time.Time
	var err_swallow_57 = concurrency.RunInLock(&w.mu, func() error {
		lastCompact = w.lastCompactTime
		return nil
	})
	if err_swallow_57 != nil {
		logging.LogSwallowedError(err_swallow_57)
	}
	if time.Since(lastCompact) < compactInterval {
		return
	}
	walPath := GetWALPath(w.projectRoot)
	info, err := os.Stat(walPath)
	if err != nil || info == nil || info.Size() < minWALSizeToCompactBytes {
		return
	}
	if err := w.storage.TryCompactWAL(); err != nil {
		StorageLog(logger).Warn(LogEventStorageWriteBehindWalCompactionFailed).
			WithError(err).
			ProjectRoot(w.projectRoot).
			Log()
		return
	}
	var err_swallow_58 = concurrency.RunInLock(&w.mu, func() error {
		w.lastCompactTime = time.Now()
		return nil
	})
	if err_swallow_58 != nil {
		logging.LogSwallowedError(err_swallow_58)
	}
	StorageLog(logger).Info(LogEventStorageWriteBehindWalCompactionCompleted).
		ProjectRoot(w.projectRoot).
		String(ConstStreamPreviousSizeMb, fmt.Sprintf("%d", info.Size()/(1024*1024))).
		Log()
}

func (w *ObjectWriteBehindWorker) processOneOrMore(ctx context.Context, secCtx *pkgctx.SecurityContext, logger logging.Logger) {
	// Process in batches; larger batch size for faster catch-up when WAL backlog is high
	const batchSize = 2500
	processed := 0

	for {
		op := w.buf.Peek()
		if op == nil {
			return
		}
		if err := w.apply(ctx, op, secCtx); err != nil {
			// Only drop-and-advance for explicitly retryable cases (WAL retains the op for replay).
			// For other failures, keep the op at the head and do not advance applied_seq so we never
			// claim durability that did not happen (silent data loss). Backlog ticker will retry apply.
			if isRetryableWriteBehindApplyDrop(err) {
				StorageLog(logger).Warn(LogEventStorageWriteBehindApplyRetryableDropped).
					WithError(err).
					String("op", op.Op).
					Kind(op.Kind).
					ObjectID(op.ID).
					Log()
				w.buf.RemoveFront(op)
				var err_swallow_59 = WriteAppliedSeq(w.projectRoot, op.Seq)
				if err_swallow_59 != nil {
					logging.LogSwallowedError(err_swallow_59)
				}
				continue
			}
			StorageLog(logger).Error(LogEventStorageWriteBehindApplyNonRetryableHeld, err).
				String("op", op.Op).
				Kind(op.Kind).
				ObjectID(op.ID).
				Log()
			return
		}
		w.buf.RemoveFront(op)
		if err := WriteAppliedSeq(w.projectRoot, op.Seq); err != nil {
			StorageLog(logger).Warn(LogEventStorageWriteBehindWriteCheckpointFailed).
				WithError(err).
				Int("seq", int(op.Seq)).
				Log()
		}
		processed++
		if processed >= batchSize {
			processed = 0
			time.Sleep(250 * time.Microsecond) // Brief yield to avoid monopolizing CPU
		}
	}
}

// processBacklogIfNeeded checks if there's a backlog in the WAL and replays more entries into the buffer.
// Only replays when buffer is below maxReplayBacklog to avoid unbounded memory.
func (w *ObjectWriteBehindWorker) processBacklogIfNeeded(ctx context.Context, secCtx *pkgctx.SecurityContext, logger logging.Logger) {
	bufLen := w.buf.Len()
	// Don't replay more if buffer is already above cap (avoids perpetual backlog blowing memory)
	if bufLen >= maxReplayBacklog {
		return
	}
	// Prefer draining buffer first before replaying more (when buffer has work)
	if bufLen > 0 {
		return
	}

	appliedSeq, err := ReadAppliedSeq(w.projectRoot)
	if err != nil {
		return
	}

	// Replay one chunk from WAL into buffer. Smaller batch to limit allocation spike per cycle (WAL replay does JSON decode + YAML marshal per record).
	const batchSize = 4000
	replayed, _, replayErr := ReplayWALChunk(w.projectRoot, appliedSeq, batchSize, w.buf.EnqueueFromWALRecord)
	if replayErr != nil {
		StorageLog(logger).Warn(LogEventStorageWriteBehindBacklogReplayFailed).WithError(replayErr).Log()
		return
	}

	if replayed > 0 {
		w.processOneOrMore(ctx, secCtx, logger)
		// Log if buffer is still high after processing (possible perpetual backlog)
		if n := w.buf.Len(); n >= backlogWarnThreshold {
			StorageLog(logger).Warn(LogEventStorageWriteBehindBufferAboveThreshold).
				Int("buffer_len", n).
				Int("threshold", backlogWarnThreshold).
				ProjectRoot(w.projectRoot).
				Log()
		}
	}
}

func (w *ObjectWriteBehindWorker) apply(ctx context.Context, op *PendingOp, secCtx *pkgctx.SecurityContext) error {
	switch op.Op {
	case "create":
		return w.storage.applyCreateFromBuffer(ctx, op.ID, op.Kind, op.Data, secCtx)
	case "update":
		return w.storage.applyUpdateFromBuffer(ctx, op.ID, op.Kind, op.Data, secCtx)
	case "delete":
		return w.storage.applyDeleteFromBuffer(ctx, op.ID, op.Kind, secCtx)
	default:
		return nil
	}
}

func (w *ObjectWriteBehindWorker) drain(ctx context.Context, secCtx *pkgctx.SecurityContext, logger logging.Logger) {
	deadline := time.Now().Add(shutdownDrainDeadline)
	for w.buf.Len() > 0 && time.Now().Before(deadline) {
		op := w.buf.Peek()
		if op == nil {
			break
		}
		if err := w.apply(ctx, op, secCtx); err != nil {
			if isRetryableWriteBehindApplyDrop(err) {
				w.buf.RemoveFront(op)
				var err_swallow_61 = WriteAppliedSeq(w.projectRoot, op.Seq)
				if err_swallow_61 != nil {
					logging.LogSwallowedError(err_swallow_61)
				}
				continue
			}
			StorageLog(logger).Error(LogEventStorageWriteBehindShutdownDrainApplyFailed, err).
				String("op", op.Op).
				Kind(op.Kind).
				ObjectID(op.ID).
				Log()
			break
		}
		w.buf.RemoveFront(op)
		var err_swallow_60 = WriteAppliedSeq(w.projectRoot, op.Seq)
		if err_swallow_60 != nil {
			logging.LogSwallowedError(err_swallow_60)
		}
	}
	if w.buf.Len() > 0 {
		StorageLog(logger).Warn(LogEventStorageWriteBehindShutdownDrainTimeout).
			Int("remaining", w.buf.Len()).
			ProjectRoot(w.projectRoot).
			Log()
	}
}

// Stop signals the worker to stop and drain, then blocks until done.
func (w *ObjectWriteBehindWorker) Stop() {
	var wasRunning bool
	var err_swallow_62 = concurrency.RunInLock(&w.mu, func() error {
		wasRunning = w.running
		w.running = false
		return nil
	})
	if err_swallow_62 != nil {
		logging.LogSwallowedError(err_swallow_62)
	}
	if !wasRunning {
		return
	}
	close(w.stopCh)
	<-w.done
}

func isRetryableWriteBehindApplyDrop(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ConstStreamSaveQueueIsFull)
}

// InitiateShutdown signals the queue to stop accepting new operations
func (w *ObjectWriteBehindWorker) InitiateShutdown() error {
	w.Stop()
	return nil
}

// Drain processes all pending operations and returns when complete
func (w *ObjectWriteBehindWorker) Drain(ctx context.Context) error {
	// Stop already drained the worker, so we just return nil here since w.Stop() blocked until done
	return nil
}

// IsDrained returns true if the queue has no pending operations
func (w *ObjectWriteBehindWorker) IsDrained() bool {
	return w.buf.Len() == 0
}

// GetPendingCount returns the number of pending operations
func (w *ObjectWriteBehindWorker) GetPendingCount() int64 {
	return int64(w.buf.Len())
}

// GetName returns the queue name for logging
func (w *ObjectWriteBehindWorker) GetName() string {
	return fmt.Sprintf("write-behind-worker-%s", w.projectRoot)
}

// IsCritical returns true if this queue must drain before force shutdown
func (w *ObjectWriteBehindWorker) IsCritical() bool {
	return true
}
