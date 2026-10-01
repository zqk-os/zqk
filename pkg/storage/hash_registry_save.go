// Extracted from pkg/storage/hash_registry.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// startSaveWorker starts the background worker that processes save operations in batches
// Implements on-demand pattern: processes batches until idle timeout, then shuts down
// This ensures all saves are serialized and prevents race conditions
// Batches are processed when either:
//   - Batch size reaches saveBatchSize
//   - saveBatchTimeout (100ms) elapses
//
// Only the latest snapshot in each batch is saved (since each request contains the full registry snapshot)
func (hr *HashRegistry) startSaveWorker() {
	defer func() {
		hr.workerRunning.Store(0)

	}()

	select {
	case <-hr.ctx.Done():

		return
	default:
	}

	hr.emitBatchEvent("start", 0, 0, 0, nil)

	batch := make([]*saveRequest, 0, saveBatchSize)
	batchTimer := time.NewTimer(saveBatchTimeout)
	defer batchTimer.Stop()

	idleTimer := time.NewTimer(hashRegistryIdleTimeout)
	defer idleTimer.Stop()

	lastWorkTime := time.Now()

	for {
		select {
		case <-hr.ctx.Done():

			batchTimer.Stop()
			idleTimer.Stop()

			if len(batch) > 0 {
				hr.processBatch(batch)
			}

			hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegCtxCancelled))
			hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
			return

		case req, ok := <-hr.saveQueue:
			if !ok {

				batchTimer.Stop()
				idleTimer.Stop()

				if len(batch) > 0 {
					hr.processBatch(batch)
				}

				hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegShutdownInit))
				hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
				return
			}

			lastWorkTime = time.Now()
			if !batchTimer.Stop() {

				select {
				case <-batchTimer.C:
				default:
				}
			}
			batchTimer.Reset(saveBatchTimeout)
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(hashRegistryIdleTimeout)

			batch = append(batch, req)
			if len(batch) >= saveBatchSize {

				hr.processBatch(batch)
				batch = batch[:0]
				if !batchTimer.Stop() {
					select {
					case <-batchTimer.C:
					default:
					}
				}
				batchTimer.Reset(saveBatchTimeout)
			}

		case <-batchTimer.C:

			if hr.ctx.Err() != nil {

				batchTimer.Stop()
				idleTimer.Stop()

				if len(batch) > 0 {
					hr.processBatch(batch)
				}

				hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegCtxCancelled))
				hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
				return
			}
			if len(batch) > 0 {
				lastWorkTime = time.Now()
				hr.processBatch(batch)
				batch = batch[:0]
			}

		case <-idleTimer.C:

			if hr.ctx.Err() != nil {

				batchTimer.Stop()
				idleTimer.Stop()

				if len(batch) > 0 {
					hr.processBatch(batch)
				}

				hr.drainQueueAndCloseChannels(errfmt.Errorf(DescHashRegCtxCancelled))
				hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
				return
			}
			queueSize := len(hr.saveQueue)
			if queueSize == 0 && len(batch) == 0 {
				idleDuration := time.Since(lastWorkTime)
				if idleDuration >= hashRegistryIdleTimeout {

					hr.workerRunning.Store(0)
					if len(hr.saveQueue) > 0 {

						if hr.workerRunning.CompareAndSwap(0, 1) {

							idleTimer.Reset(hashRegistryIdleTimeout)
							continue
						}
					}
					hr.emitBatchEvent("shutdown", 0, 0, 0, nil)
					return
				}

				idleTimer.Reset(hashRegistryIdleTimeout - idleDuration)
			} else {

				lastWorkTime = time.Now()
				idleTimer.Reset(hashRegistryIdleTimeout)
			}
		}
	}
}

// drainQueueAndCloseChannels drains the save queue and closes all done channels
// This ensures Save() calls don't wait forever when worker exits
// Note: Queue may be closed, so we use non-blocking reads
func (hr *HashRegistry) drainQueueAndCloseChannels(err error) {

	for {
		select {
		case req, ok := <-hr.saveQueue:
			if !ok {

				return
			}

			select {
			case req.done <- err:
			default:
			}
			close(req.done)
		default:

			return
		}
	}
}

// processBatch processes a batch of save requests.
// Only the latest snapshot is persisted; we nil out other requests' .data so the GC can reclaim
// those maps immediately (reduces peak memory when batch holds many full registry copies).
func (hr *HashRegistry) processBatch(batch []*saveRequest) {
	if len(batch) == 0 {
		return
	}

	latestReq := batch[len(batch)-1]
	batchSize := len(batch)
	hashCount := len(latestReq.data)

	for i := 0; i < len(batch)-1; i++ {
		batch[i].data = nil
	}

	hr.emitBatchEvent("start", batchSize, hashCount, 0, nil)

	startTime := time.Now()
	err := hr.processSave(latestReq.data)
	duration := time.Since(startTime)

	status := "complete"
	if err != nil {
		status = "error"
	}
	hr.emitBatchEvent(status, batchSize, hashCount, duration, err)

	for _, req := range batch {

		select {
		case req.done <- err:
		default:
		}

		close(req.done)
	}
}

// processSave performs the actual file I/O to save the hash registry
// This is called by the background worker, ensuring all saves are serialized.
// Uses json.Marshal (not MarshalIndent) for speed and smaller I/O on large registries.
func (hr *HashRegistry) processSave(hashes map[string]string) error {
	if StreamStorageEnabledForKind(hr.kind) {
		return nil
	}
	if len(hashes) == 0 {
		if _, statErr := fileutil.Stat(filepath.Dir(hr.filePath)); statErr != nil && fileutil.IsNotExist(statErr) {
			return nil
		}
	}
	registry := struct {
		Hashes map[string]string `json:"hashes"`
	}{
		Hashes: hashes,
	}
	data, err := json.Marshal(registry)
	if err != nil {
		return errfmt.Newf(ErrMsgMarshalHashReg).Wrap(err)
	}
	data = append(data, '\n')

	if err := fileutil.MkdirAll(filepath.Dir(hr.filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateHashRegDir).Wrap(err)
	}

	tmpFile := hr.filePath + ".tmp"
	file, err := fileutil.OpenFile(tmpFile, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_TRUNC, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenTempFile).Wrap(err)
	}

	writeErr := func() error {
		defer file.Close()

		if _, err := file.Write(data); err != nil {
			return errfmt.Newf(ErrMsgWriteHashRegData).Wrap(err)
		}

		return nil
	}()

	if writeErr != nil {

		if rmErr := fileutil.Remove(tmpFile); rmErr != nil && !fileutil.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return writeErr
	}

	dirPath := filepath.Dir(hr.filePath)
	if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		if rmErr := fileutil.Remove(tmpFile); rmErr != nil && !fileutil.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return errfmt.Newf(ErrMsgEnsureDirBeforeRename).Wrap(err)
	}

	if dir, err := fileutil.Open(dirPath); err == nil {

		_ = dir.Close()
	}

	if _, err := fileutil.Stat(dirPath); err != nil {
		if rmErr := fileutil.Remove(tmpFile); rmErr != nil && !fileutil.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return errfmt.Newf(ErrMsgDirNotAccessible).Wrap(err)
	}

	retryConfig := &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	renameErr := ExecuteSimpleRetry(hr.ctx, retryConfig, func() error {

		if mkdirErr := fileutil.MkdirAll(dirPath, paths.DirPerm755); mkdirErr != nil {
			return errfmt.Newf(ErrMsgEnsureDirHashReg).Wrap(mkdirErr)
		}

		if dir, dirErr := fileutil.Open(dirPath); dirErr == nil {
			_ = dir.Close()
		}

		if _, statErr := fileutil.Stat(dirPath); statErr != nil {
			return errfmt.Newf(ErrMsgDirNotAccessible).Wrap(statErr)
		}

		if _, statErr := fileutil.Stat(tmpFile); statErr != nil {
			return errfmt.Newf(ErrMsgTempFileNotExist).Wrap(statErr)
		}

		if renameErr := fileutil.Rename(tmpFile, hr.filePath); renameErr != nil {

			if !fileutil.IsNotExist(renameErr) {

				if rmErr := fileutil.Remove(tmpFile); rmErr != nil && !fileutil.IsNotExist(rmErr) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
				}
				return errfmt.Newf(ErrMsgRenameTempFile).Wrap(renameErr)
			}
			return errfmt.Newf(ErrMsgRenameNotFoundRetry).Wrap(renameErr)
		}

		return nil
	}, nil)

	if renameErr != nil {
		if rmErr := fileutil.Remove(tmpFile); rmErr != nil && !fileutil.IsNotExist(rmErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgRemoveTempFile, rmErr).Log()
		}
		return errfmt.Newf(ConstMiscFailedToSaveHashRegistryAfterRetriesW).Wrap(renameErr)
	}

	parentDir := filepath.Dir(hr.filePath)
	now := time.Now()
	if err := fileutil.Chtimes(parentDir, now, now); err != nil {

	}
	if dir, err := fileutil.Open(parentDir); err == nil {
		_ = dir.Close()
	}

	if fileInfo, statErr := fileutil.Stat(hr.filePath); statErr == nil {
		if err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryUpdateMtime, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			hr.fileMtime = fileInfo.ModTime()
			hr.lastLoadTime = time.Now()
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
		}

	}

	return nil
}

// emitBatchEvent emits a batch processing event via coordinator
func (hr *HashRegistry) emitBatchEvent(
	status string,
	batchSize, hashCount int,
	duration time.Duration,
	err error,
) {
	callback := getHashRegistryEventCallback()
	if callback == nil {

		return
	}

	projectRoot := hr.GetProjectRoot()
	storage := hr.GetStorage()
	kind := hr.kind

	if projectRoot == emptyValue || storage == nil {

		return
	}

	ctx := hr.ctx

	goroutinelabels.NewGoroutine(DescHashRegCoordEvent, fmt.Sprintf(DescEmitBatchProcessEvent, kind)).
		StartSimple(func() {
			callback(
				ctx,
				projectRoot,
				storage,
				kind,
				batchSize,
				hashCount,
				duration,
				status,
				err,
			)
		})
}
