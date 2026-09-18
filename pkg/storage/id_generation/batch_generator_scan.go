// Extracted from pkg/storage/id_generation/batch_generator.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package id_generation

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// scanDirectory scans the directory to find the highest existing sequence number
// CRITICAL: Releases generator lock during file I/O to prevent blocking other generators
// Uses per-directory scan lock to prevent concurrent scans of the same directory
// Must be called with g.mu locked, but releases lock for I/O
// NOTE: Uses direct mutex unlock/lock because it releases the lock before I/O and re-acquires
// it after, which doesn't fit the timeout wrapper pattern (wrapper expects to manage full lifecycle)
func (g *BatchIDGenerator) scanDirectory() error {
	ctx := g.ctx
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	g.ensurePattern()
	// Check cache first (fast path, no I/O)
	var cached *dirScanResult
	var exists bool
	var scanTime time.Time
	var cachedMaxSeq int
	_ = concurrency.RunInRLockOrLog(
		&dirScanCacheLock, locknames.LockNameBatchGeneratorScanCacheCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			cached, ok = dirScanCache[g.kindDir]
			exists = ok
			if exists {
				_ = concurrency.RunInRLockOrLog(
					&cached.mu, locknames.LockNameBatchGeneratorScanCacheRead, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						scanTime = cached.scanTime
						cachedMaxSeq = cached.maxSequence
						return nil
					},
				)
			}
			return nil
		},
	)

	if exists {

		if time.Since(scanTime) < dirScanConfig.CacheTTL {
			if cachedMaxSeq > g.lastSequence {
				g.lastSequence = cachedMaxSeq
			}
			return nil
		}
	}

	cacheKey := fmt.Sprintf("%s:%s", g.kindDir, g.prefix)

	dirLock := getDirScanLock(ctx, cacheKey)
	return concurrency.RunInLockWithLogger(
		dirLock, locknames.LockNameBatchGeneratorDirScan, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Double-check cache after acquiring directory lock (another goroutine may have scanned)
			var scanTime2 time.Time
			var cachedMaxSeq2 int
			var exists2 bool
			_ = concurrency.RunInRLockOrLog(
				&dirScanCacheLock, locknames.LockNameBatchGeneratorScanCacheDoubleCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					var ok bool
					cached, ok = dirScanCache[cacheKey]
					exists2 = ok
					if exists2 {
						_ = concurrency.RunInRLockOrLog(
							&cached.mu, locknames.LockNameBatchGeneratorScanCacheRead2, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
							func() error {
								scanTime2 = cached.scanTime
								cachedMaxSeq2 = cached.maxSequence
								return nil
							},
						)
					}
					return nil
				},
			)

			if exists2 {

				if time.Since(scanTime2) < dirScanConfig.CacheTTL {
					if cachedMaxSeq2 > g.lastSequence {
						g.lastSequence = cachedMaxSeq2
					}
					return nil
				}
			}

			config := dirScanConfig

			var entries []fileutil.DirEntry
			var lastErr error
			delay := config.RetryInitialDelay

			for attempt := 0; attempt < config.RetryMaxAttempts; attempt++ {

				if ctx.Err() != nil {
					return errfmt.Newf(ConstDirectoryReadCancelled).Wrap(ctx.Err())
				}

				// Non-blocking ReadDir with timeout (per IO + Async On-Demand pattern)
				type readDirResult struct {
					entries []fileutil.DirEntry
					err     error
				}
				resultChan := make(chan readDirResult, 1)

				goroutinelabels.NewGoroutine(ConstBatchGeneratorReaddirTimeout, fmt.Sprintf("reading directory %s with timeout (attempt %d/%d)", g.kindDir, attempt+1, config.RetryMaxAttempts)).
					WithContext(ctx).
					StartSimple(func() {
						readEntries, readErr := fileutil.ReadDir(g.kindDir)
						resultChan <- readDirResult{entries: readEntries, err: readErr}
					})

				select {
				case result := <-resultChan:
					entries = result.entries
					if result.err == nil || fileutil.IsNotExist(result.err) {

						lastErr = nil
						break
					}
					lastErr = result.err

					if !isRetryableReadDirError(result.err) {
						return result.err
					}
				case <-time.After(config.ReadDirTimeout):

					lastErr = errfmt.Errorf(ConstDirectoryReadTimeoutForSAfterVDirectoryMayHaveTooManyFiles, g.kindDir, config.ReadDirTimeout)
				case <-ctx.Done():

					return errfmt.Newf(ConstDirectoryReadCancelled).Wrap(ctx.Err())
				}

				if lastErr == nil {
					break
				}

				if attempt == config.RetryMaxAttempts-1 {
					break
				}

				select {
				case <-ctx.Done():
					return errfmt.Newf(ConstDirectoryReadCancelled).Wrap(ctx.Err())
				case <-time.After(delay):

				}

				delay = time.Duration(float64(delay) * config.RetryBackoffFactor)
				if delay > config.RetryMaxDelay {
					delay = config.RetryMaxDelay
				}
			}

			if lastErr != nil {
				return errfmt.Errorf(ConstDirectoryReadFailedAfterDAttemptsW, config.RetryMaxAttempts, lastErr)
			}

			maxSeq := g.startAt - 1

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := filepath.Ext(entry.Name())
				if ext != ".yaml" && ext != ".yml" {
					continue
				}

				filenameID := strings.TrimSuffix(entry.Name(), ".yaml")
				filenameID = strings.TrimSuffix(filenameID, ".yml")

				matches := g.seqPattern.FindStringSubmatch(filenameID)
				if len(matches) > 1 {
					seq, err := strconv.Atoi(matches[1])
					if err == nil && seq > maxSeq {
						maxSeq = seq
					}
				}
			}

			if g.casProvider != nil {
				casIDs, err := g.casProvider(ctx, g.kind, g.kindDir, g.prefix)
				if err == nil {

					for _, casID := range casIDs {
						matches := g.seqPattern.FindStringSubmatch(casID)
						if len(matches) > 1 {
							seq, err := strconv.Atoi(matches[1])
							if err == nil && seq > maxSeq {
								maxSeq = seq
							}
						}
					}
				}

			}

			_ = concurrency.RunInLockOrLog(
				&dirScanCacheLock, locknames.LockNameBatchGeneratorUpdateCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					dirScanCache[cacheKey] = &dirScanResult{
						maxSequence: maxSeq,
						scanTime:    time.Now(),
					}
					return nil
				},
			)

			if maxSeq > g.lastSequence {
				g.lastSequence = maxSeq
			}

			return nil
		},
	)
}

// isRetryableReadDirError determines if a ReadDir error is retryable
// Per architecture: timeout errors and temporary errors are retryable
func isRetryableReadDirError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
		return true
	}

	if strings.Contains(errStr, "temporary") {
		return true
	}

	if strings.Contains(errStr, "permission") || strings.Contains(errStr, "not found") {
		return false
	}

	return false
}
