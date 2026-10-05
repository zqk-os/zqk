// Extracted from pkg/storage/hash_registry.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Load loads the hash registry from disk
// OPTIMIZATION: Skip re-read when the file's mtime matches the mtime from the last successful load.
// Do not compare file mtime to lastLoadTime (wall clock): coarse mtimes can appear "before"
// lastLoadTime even when the file changed, causing stale in-memory hashes.
// CRITICAL FIX: Release lock before doing blocking file I/O to prevent deadlocks
func (hr *HashRegistry) Load() error {
	var fileInfo fileutil.FileInfo
	var fileMtime time.Time
	var shouldReload bool
	err := concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryLoadCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var statErr error
		fileInfo, statErr = fileutil.Stat(hr.filePath)
		if fileutil.IsNotExist(statErr) {
			hr.hashes = make(map[string]string)
			hr.lastLoadTime = time.Now()
			hr.fileMtime = time.Time{}
			return nil
		}
		if statErr != nil {
			return errfmt.Newf(ErrMsgStatHashReg).Wrap(statErr)
		}
		fileMtime = fileInfo.ModTime()
		if !hr.fileMtime.IsZero() && fileMtime.Equal(hr.fileMtime) {
			return nil
		}
		shouldReload = true
		return nil
	})
	if err != nil {
		return err
	}
	if !shouldReload {
		return nil
	}

	data, err := fileutil.ReadFile(hr.filePath)
	if err != nil {
		return errfmt.Newf(ErrMsgReadHashRegistry).Wrap(err)
	}
	var registry struct {
		Hashes map[string]string `json:"hashes"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return errfmt.Newf(ErrMsgParseHashReg).Wrap(err)
	}

	err = concurrency.RunInLockWithLogger(&hr.mu, locknames.LockNameHashRegistryLoadUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !hr.fileMtime.IsZero() && fileMtime.Equal(hr.fileMtime) {
			return nil
		}
		hr.hashes = registry.Hashes
		if hr.hashes == nil {
			hr.hashes = make(map[string]string)
		}
		hr.lastLoadTime = time.Now()
		hr.fileMtime = fileMtime
		return nil
	})
	return err
}

// SaveAsync enqueues a save request and returns immediately without waiting for completion.
// Use during WAL replay so apply path stays fast; the background worker persists when it can.
// Caller must not rely on durability before the next synchronous Save() or process exit.
func (hr *HashRegistry) SaveAsync() {
	if hr.ctx.Err() != nil {
		return
	}
	if !hr.skipShutdownCoordinatorCheck.Load() {
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil && coordinator.IsShutdownInitiated() {
			return
		}
	}
	var dataCopy map[string]string
	if err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySaveAsyncCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		dataCopy = make(map[string]string, len(hr.hashes))
		maps.Copy(dataCopy, hr.hashes)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
	}

	req := &saveRequest{
		data: dataCopy,
		done: make(chan error, 1),
	}

	select {
	case hr.saveQueue <- req:

		hr.wakeWorkerIfNeeded()
	default:

	}
}

// Save saves the hash registry to disk
// Uses a thread-safe queue to serialize all save operations, preventing race conditions
// from concurrent saves. The method enqueues a save request and waits for completion.
// Wakes worker if needed (on-demand pattern)
// Returns ErrShutdownInProgress if shutdown has been initiated
func (hr *HashRegistry) Save() error {
	req, err := hr.prepareSaveRequest()
	if err != nil {
		return err
	}

	select {
	case hr.saveQueue <- req:

		hr.wakeWorkerIfNeeded()
	case <-hr.ctx.Done():

		return errfmt.Errorf(DescHashRegCtxCancelEnqueue)
	default:

		return errfmt.Errorf(DescSaveQueueFull)
	}

	select {
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	default:
	}

	saveTimer := time.NewTimer(hashRegistrySaveMaxWait)
	defer saveTimer.Stop()
	select {
	case err := <-req.done:
		if hr.ctx.Err() != nil {
			return errfmt.Errorf(DescHashRegCtxCancelWait)
		}
		return err
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	case <-saveTimer.C:

		queueLen := len(hr.saveQueue)
		hashCount := len(req.data)
		workerRunning := hr.workerRunning.Load() != 0
		var fileSize int64
		if fi, err := fileutil.Stat(hr.filePath); err == nil {
			fileSize = fi.Size()
		}
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		timeoutErr := errfmt.Errorf(DescHashRegSaveNotComplete, hashRegistrySaveMaxWait)
		StorageLog(logger).Error(LogEventStorageHashRegistrySaveTimeoutErr, timeoutErr).
			Kind(hr.kind).
			String("file", hr.filePath).
			Int("hash_count", hashCount).
			Int("queue_len", queueLen).
			Bool(TagWorkerRunning, workerRunning).
			String(TagFileSizeBytes, fmt.Sprintf("%d", fileSize)).
			String("max_wait", hashRegistrySaveMaxWait.String()).
			Log()
		return errfmt.Errorf(DescHashRegSaveNotCompleteBg, hashRegistrySaveMaxWait)
	}
}

// SaveWithContext is like Save but stops waiting when ctx is cancelled (e.g. init timeout).
// Use this during storage init so the daemon does not hang indefinitely on HashRegistry I/O.
// If ctx is nil, behaves as Save() (no timeout).
func (hr *HashRegistry) SaveWithContext(ctx context.Context) error {
	if ctx == nil {
		return hr.Save()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if hr.ctx.Err() != nil {
		return errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}

	req, err := hr.prepareSaveRequest()
	if err != nil {
		return err
	}

	select {
	case hr.saveQueue <- req:
		hr.wakeWorkerIfNeeded()
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelEnqueue)
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errfmt.Errorf(DescSaveQueueFull)
	}

	select {
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case err := <-req.done:
		if hr.ctx.Err() != nil {
			return errfmt.Errorf(DescHashRegCtxCancelWait)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-hr.ctx.Done():
		return errfmt.Errorf(DescHashRegCtxCancelWait)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (hr *HashRegistry) prepareSaveRequest() (*saveRequest, error) {
	if hr.ctx.Err() != nil {
		return nil, errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}
	if !hr.skipShutdownCoordinatorCheck.Load() {
		coordinator := GetGlobalShutdownCoordinator()
		if coordinator != nil && coordinator.IsShutdownInitiated() {
			return nil, errfmt.Errorf(DescShutdownInProgressNoSave)
		}
	}
	if hr.ctx.Err() != nil {
		return nil, errfmt.Errorf(DescHashRegCtxCancelNoSave)
	}

	var dataCopy map[string]string
	err := concurrency.RunInRLockWithLogger(&hr.mu, locknames.LockNameHashRegistrySaveCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		dataCopy = make(map[string]string, len(hr.hashes))
		maps.Copy(dataCopy, hr.hashes)
		return nil
	})
	if err != nil {
		return nil, errfmt.Newf(DescCopyingHashRegData).Wrap(err)
	}

	return &saveRequest{
		data: dataCopy,
		done: make(chan error, 1),
	}, nil
}
