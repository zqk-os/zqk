package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DurabilityFlushContext returns a timeout context for EnsureCLIObjectMutationVisible* calls.
// It is rooted at [context.Background] so a canceled CLI or HTTP request context cannot abort
// the write-behind drain / CAS flush after Create/Update has already succeeded.
func DurabilityFlushContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second) // Background: request-or-shutdown derived
}

// EnsureCLIObjectMutationVisible waits until the write-behind buffer is empty (so WAL-applied
// creates/updates/deletes have been written through to CAS/files) and flushes the per-project CAS
// index queue for the given kinds. Use after CLI object create/update/delete so a subsequent
// zqk process (e.g. `zqk object get`) sees the object immediately.
func (f *FileObjectStorage) EnsureCLIObjectMutationVisible(ctx context.Context, flushKinds []string) error {
	if f.projectRoot == emptyValue {
		return nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()

	hadBufferedWork := f.writeBuf != nil && f.writeBuf.Len() > 0

	drainUntilEmpty := func() error {
		if f.writeBuf == nil {
			return nil
		}
		if f.writeBuf.Len() == 0 {
			return nil
		}
		// Wake the worker once; subsequent wake-ups are not required because
		// WaitUntilEmpty is event-driven via the buffer's sync.Cond.
		if f.writeBehindWorker != nil {
			f.writeBehindWorker.Notify()
		}
		if err := f.writeBuf.WaitUntilEmpty(ctx); err != nil {
			bufLen := f.writeBuf.Len()
			return errfmt.Errorf(ConstStreamTimeoutWaitingForWriteBehindDrainBufferLenIntErr, bufLen, err)
		}
		return nil
	}

	// First drain: apply all WAL-buffered user ops (metrics, etc.).
	if err := drainUntilEmpty(); err != nil {
		return err
	}
	// Audit events deferred during write-behind apply are queued in memory; materialize them
	// (BulkCreate audit_event may enqueue more WAL work).
	FlushPendingAuditEvents(ctx, f.projectRoot, f, secCtx)
	// Second drain: apply audit_event creates from the flush above.
	hadBufferedWork = hadBufferedWork || (f.writeBuf != nil && f.writeBuf.Len() > 0)
	if err := drainUntilEmpty(); err != nil {
		return err
	}

	// Buffer is empty, but worker might still be applying the very last item.
	// Skip cross-process WAL checkpoint wait when this process had nothing buffered
	// (e.g. CLI create with WithSkipWriteBehind already flushed CAS/index synchronously).
	if hadBufferedWork {
		if err := WaitForWALProcessingEventDriven(f.projectRoot, 15*time.Second); err != nil {
			return err
		}
	}

	q := caspkg.GetListingIndexWriteQueueForProjectRoot(f.projectRoot)
	if len(flushKinds) == 0 {
		// Defense: callers that pass nil (historical promote/demote bug) still need CAS
		// index durable across process boundaries. TRACK: [REDACTED-ID]
		if err := q.FlushAll(15 * time.Second); err != nil {
			return errfmt.Errorf(ConstStreamFailedToFlushAllCasIndexesVal, err)
		}
		return nil
	}
	for _, k := range flushKinds {
		if k == emptyValue {
			continue
		}
		if err := q.FlushKind(k, 15*time.Second); err != nil {
			return errfmt.Errorf(ConstStreamFlushCasIndexForKindQuoteErr, k, err)
		}
		// Stream-backed kinds do not use the CAS index for reads, but another process (or a fresh
		// in-memory snapshot) must see on-disk stream registry updates immediately after drain.
		if StreamStorageEnabledForKind(k) {
			InvalidateStreamRegistryCacheForKind(f.projectRoot, k)
		}
	}
	return nil
}

// EnsureCLIObjectMutationVisibleForProvider runs EnsureCLIObjectMutationVisible for *FileObjectStorage;
// otherwise falls back to WAL checkpoint stabilization and per-root CAS flush (best effort).
func EnsureCLIObjectMutationVisibleForProvider(ctx context.Context, provider ObjectStorageProvider, projectRoot string, flushKinds []string) error {
	if projectRoot == emptyValue {
		return nil
	}

	// Try to get underlying providers based on the flushKinds.
	providersToFlush := []ObjectStorageProvider{provider}

	for i := 0; i < 10; i++ {
		var nextProviders []ObjectStorageProvider
		changed := false
		for _, p := range providersToFlush {
			switch v := p.(type) {
			case *BatchingObjectStorage:
				nextProviders = append(nextProviders, v.ObjectStorageProvider)
				changed = true
				continue
			case *HybridObjectStorage:
				// Typically FileObjectStorage is secondary, but could be primary. We flush both if they match.
				nextProviders = append(nextProviders, v.GetPrimary(), v.GetSecondary())
				changed = true
				continue
			case *RoutingObjectStorage:
				factory := v.GetStorageFactory()
				for _, k := range flushKinds {
					if k != emptyValue {
						sp := factory.GetStorageForKind(k)
						nextProviders = append(nextProviders, sp)
					}
				}
				changed = true
				continue
			}

			nextProviders = append(nextProviders, p)
		}

		providersToFlush = nextProviders
		if !changed {
			break
		}
	}

	flushedFileStorage := false
	for _, p := range providersToFlush {
		if fs, ok := p.(*FileObjectStorage); ok && fs != nil {
			if err := fs.EnsureCLIObjectMutationVisible(ctx, flushKinds); err != nil {
				return err
			}
			flushedFileStorage = true
		}
	}
	if flushedFileStorage {
		return nil
	}

	// Cross-process / non-file backends: wait for WAL checkpoint progress without relying on
	// local in-memory write buffer state. If the scheduler daemon is not running, there is
	// no WAL processing worker to wait for, so we bypass to prevent hanging.
	if !isSchedulerRunning(projectRoot) {
		return nil
	}

	waitTimeout := 15 * time.Second
	if ctx != nil {
		if dl, ok := ctx.Deadline(); ok {
			if remaining := time.Until(dl); remaining > 0 && remaining < waitTimeout {
				waitTimeout = remaining
			}
		}
	}
	if err := WaitForWALProcessingEventDriven(projectRoot, waitTimeout); err != nil {
		return err
	}
	for _, k := range flushKinds {
		if k == emptyValue {
			continue
		}
		if err := FlushListingIndexForProjectRoot(projectRoot, k); err != nil {
			return errfmt.Errorf(ConstStreamFlushCasIndexForKindQuoteErr, k, err)
		}
	}
	if err := postFlushDarwinCASVisibilityIfNeeded(ctx, projectRoot, flushKinds); err != nil {
		return err
	}
	return nil
}

func isSchedulerRunning(projectRoot string) bool {
	pidBytes, err := fileutil.ReadFile(filepath.Join(projectRoot, ".zqk", "scheduler", "scheduler.pid"))
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(pidBytes))
	if parts := strings.Split(pidStr, ":"); len(parts) > 0 {
		pidStr = parts[0]
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
