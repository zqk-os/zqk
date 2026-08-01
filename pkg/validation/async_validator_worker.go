package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

var reBucketDateDir = regexp.MustCompile(`^(\d{4}-\d{2}(-\d{2})?|[a-z0-9])$`)

// validationTimeoutForKind returns the per-object validation timeout for a kind.
// Uses config from .zqk/config/config.yaml validation.per_object_timeout (default_seconds, kind_overrides).
func validationTimeoutForKind(kind string) time.Duration {
	return GetGlobalValidationTimeoutConfig().TimeoutForKind(kind)
}

// semaphoreFullWaitTimeout is how long a worker waits to acquire the validation semaphore
// before assuming slots are stuck. With a large queue and ~NumCPU*2 slots, all slots can
// be busy for several seconds under normal load; use a longer timeout to avoid spurious warnings.
const semaphoreFullWaitTimeout = 15 * time.Second

// readValidationInput reads either a regular file path or a stream location path
// in the form "<segmentPath>::<offset>" and returns one JSON object payload.
func readValidationInput(path string) ([]byte, error) {
	sepIdx := strings.LastIndex(path, "::")
	if sepIdx < 0 {
		return os.ReadFile(path)
	}

	segmentPath := path[:sepIdx]
	offsetStr := path[sepIdx+2:]
	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		return os.ReadFile(path)
	}

	f, err := os.Open(segmentPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	var raw json.RawMessage
	dec := json.NewDecoder(f)
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

// worker is the validation worker goroutine
// Implements on-demand pattern: processes work, then shuts down after idle timeout
//
//nolint:gocyclo // complexity from state machine and validation branches; refactor separately
func (av *AsyncValidator) worker(id int) {
	goroutineID := getGoroutineID()
	activeCount := incrementActiveGoroutines()
	startTime := time.Now()
	idleStartTime := time.Now()
	processedCount := 0
	failedCount := 0

	defer func() {
		av.activeWorkers.Add(-1)
		decrementActiveGoroutines()
		av.workerStates.Delete(id)
		av.wg.Done()

		// Emit worker shutdown event
		callback := getValidationLifecycleEventCallback()
		if callback != nil && av.projectRoot != emptyValue {
			var projectRoot string
			if err := concurrency.RunInRLockWithLogger(
				&av.mu,
				LockNameAsyncValidatorWorkerShutdownGetRoot,
				lockLoggerSystem(),
				func() error {
					projectRoot = av.projectRoot
					return nil
				},
			); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Use validator's context (or derived context) instead of creating new Background()
					Error(ConstMagic42bbcbd4, err).Log()
			}

			callback(
				av.ctx,
				projectRoot,
				nil, // Storage provider not available in validation package
				fmt.Sprintf("worker_%d", id), ConstMagicExtracted_19, "stopped",
				int(av.activeWorkers.Load()),
				processedCount,
				failedCount,
				time.Since(startTime),
			)
		}

		logging.Fluent(av.logger).Debug(ConstMagicExtracted_20).
			WorkerID(id).
			GoroutineID(int(goroutineID)).
			ActiveGoroutines(int(getActiveGoroutines())).
			Log()
	}()

	av.workerStates.Store(id, "running")
	logging.Fluent(av.logger).Debug(ConstMagicExtracted_21).
		WorkerID(id).
		GoroutineID(int(goroutineID)).
		ActiveGoroutines(int(activeCount)).
		Log()

	// Emit worker start event
	callback := getValidationLifecycleEventCallback()
	if callback != nil && av.projectRoot != emptyValue {
		var projectRoot string
		if err := concurrency.RunInRLockWithLogger(
			&av.mu,
			LockNameAsyncValidatorWorkerGetProjectRoot,
			lockLoggerSystem(),
			func() error {
				projectRoot = av.projectRoot
				return nil
			},
		); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstMagica5c96b8a, err).Log()
		}
		callback(
			av.ctx,
			projectRoot,
			nil, // Storage provider not available in validation package
			fmt.Sprintf("worker_%d", id),
			"worker_start",
			"started",
			int(av.activeWorkers.Load()),
			0,
			0,
			0,
		)
	}

	for {
		select {
		case <-av.ctx.Done():
			av.workerStates.Store(id, "stopping")
			logging.Fluent(av.logger).Info("Worker stopping (ctx done)").
				WorkerID(id).
				GoroutineID(int(goroutineID)).
				ActiveGoroutines(int(getActiveGoroutines())).
				Log()
			return
		case <-av.shutdown:
			av.workerStates.Store(id, "stopping")
			logging.Fluent(av.logger).Info("Worker stopping (shutdown signal)").
				WorkerID(id).
				GoroutineID(int(goroutineID)).
				Log()
			return
		default:
			logging.Fluent(av.logger).Debug("Worker dequeuing task").WorkerID(id).Log()
			// Dequeue task
			av.workerStates.Store(id, "dequeuing")

			// Get last queue size to detect empty transition
			var wasNonEmpty bool
			if err := concurrency.RunInLockWithLogger(
				&av.mu,
				LockNameAsyncValidatorWorkerCheckQueueEmpty,
				lockLoggerSystem(),
				func() error {
					wasNonEmpty = av.lastQueueSize > 0
					return nil
				},
			); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstMagic4dbd9cb3, err).Log()
			}

			task := av.priorityQueue.Dequeue()

			// Check queue size after dequeue to detect if it became empty
			queueSizeAfter := av.priorityQueue.Size()

			// Update last queue size and check if queue just became empty
			var qCallback func()
			if err := concurrency.RunInLockWithLogger(
				&av.mu,
				LockNameAsyncValidatorWorkerUpdateQueueSize,
				lockLoggerSystem(),
				func() error {
					av.lastQueueSize = queueSizeAfter
					qCallback = av.queueEmptyCallback
					return nil
				},
			); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Fire callback if queue transitioned from non-empty to empty
					Error(ConstMagicd5eab1ac, err).Log()
			}

			if wasNonEmpty && queueSizeAfter == 0 && qCallback != nil {
				// Queue just became empty - fire callback to trigger immediate completion check
				qCallback()
			}

			if task == nil {
				// No tasks available - check idle timeout
				idleDuration := time.Since(idleStartTime)
				if idleDuration >= asyncValidatorIdleTimeout {
					// Idle timeout reached - shut down worker
					logging.Fluent(av.logger).Info(ConstMagic01bd43e7).
						WorkerID(id).
						IdleDuration(idleDuration.String()).
						ProcessedCount(processedCount).
						FailedCount(failedCount).
						Log()

					// Emit worker idle shutdown event
					callback := getValidationLifecycleEventCallback()
					if callback != nil && av.projectRoot != emptyValue {
						var projectRoot string
						if err := concurrency.RunInRLockWithLogger(
							&av.mu,
							LockNameAsyncValidatorWorkerIdleShutdownGetRoot,
							lockLoggerSystem(),
							func() error {
								projectRoot = av.projectRoot
								return nil
							},
						); err != nil {
							logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

								// Use validator's context (or derived context) instead of creating new Background()
								Error(ConstMagicbcbfecc9, err).Log()
						}

						callback(
							av.ctx,
							projectRoot,
							nil, // Storage provider not available in validation package
							fmt.Sprintf("worker_%d", id), ConstMagic8118ad7d, "idle_shutdown",
							int(av.activeWorkers.Load()),
							processedCount,
							failedCount,
							time.Since(startTime),
						)
					}
					return
				}

				// Wait a bit before checking again
				av.workerStates.Store(id, "waiting")
				select {
				case <-av.ctx.Done():
					return
				case <-av.shutdown:
					return
				case <-av.taskAvailable:
					// Task available - try to dequeue again
					task = av.priorityQueue.Dequeue()
					if task == nil {
						// Spurious wake-up or task already taken - continue loop
						continue
					}
				case <-time.After(asyncValidatorCheckInterval):
					// Periodic check - continue loop to check idle timeout
					continue
				}
			}

			// Reset idle timer when we get work
			idleStartTime = time.Now()

			// Launch goroutine to validate object and write output
			// Each object gets its own goroutine - validates and writes result directly
			// Use separate wait group so Stop() doesn't wait for validation goroutines
			// PERFORMANCE: Use semaphore to limit concurrent validation goroutines
			av.workerStates.Store(id, fmt.Sprintf("launching:%s", task.ObjectID))
			taskCopy := task // Capture task for goroutine
			validationGoroutineName := fmt.Sprintf(ConstMagic319b4ba0, task.ObjectID)
			validationGoroutinePurpose := fmt.Sprintf(ConstMagic018b055e, task.ObjectID, task.ObjectKind)

			// Acquire semaphore slot with timeout to prevent indefinite blocking
			// If all slots are taken and validation goroutines are stuck, we don't want
			// workers to block forever. Use a short timeout and re-enqueue if needed.
			semaphoreStart := time.Now()
			select {
			case av.validationSemaphore <- struct{}{}:
				// Successfully acquired semaphore slot - proceed
				waitTime := time.Since(semaphoreStart)
				if waitTime > 100*time.Millisecond {
					logging.Fluent(av.logger).Debug(ConstMagic8d316b48).
						ObjectID(taskCopy.ObjectID).
						WaitTime(waitTime.String()).
						Log()
				}
			case <-time.After(semaphoreFullWaitTimeout):
				// Semaphore at capacity for full timeout - may indicate stuck goroutines or very heavy load
				// Re-enqueue task and log warning, then continue to process other tasks
				logging.Fluent(av.logger).Warn(ConstMagicf317420a).
					ObjectID(taskCopy.ObjectID).
					QueueSize(av.priorityQueue.Size()).
					SemaphoreCapacity(cap(av.validationSemaphore)).
					WaitTime(semaphoreFullWaitTimeout.String()).
					Log()

				// Emit coordinator event for semaphore full warning
				if av.eventCallback != nil {
					av.eventCallback(ConstMagicExtracted_22, taskCopy.ObjectID,
						fmt.Sprintf(ConstMagic8f530026, cap(av.validationSemaphore)),
						map[string]any{
							"queue_size": av.priorityQueue.Size(), ConstMagicExtracted_23: cap(av.validationSemaphore),
						}, "high")
				}

				av.priorityQueue.Enqueue(taskCopy)
				av.notifyWorkers() // Signal workers (might free up a slot)
				failedCount++      // Track failed enqueue due to semaphore
				continue           // Continue worker loop to process other tasks
			case <-av.ctx.Done():
				// Context cancelled - stop worker
				return
			case <-av.shutdown:
				// Shutdown initiated - stop worker
				return
			}

			valBud := goroutinelabels.DefaultBudget()
			valBuilder := goroutinelabels.NewGoroutine(validationGoroutineName, validationGoroutinePurpose).
				WithWaitGroup(&av.validationWg).
				WithCleanup(func() {
					<-av.validationSemaphore // Release semaphore slot
				})
			if valBud != nil {
				valBuilder = valBuilder.WithBudget(valBud)
			}
			valBuilder.StartWithContext(av.ctx, func(ctx context.Context) error {
				incrementActiveGoroutines()
				defer decrementActiveGoroutines()

				// FIXED: Acquire outer lock once, read callback, release lock
				// Then do all work without holding the outer lock
				var resCallback ResultCallback
				var validationFunc ValidationFunc
				if err := concurrency.RunInRLockWithLogger(
					&av.mu,
					LockNameAsyncValidatorWorkerGetCallbacks,
					lockLoggerSystem(),
					func() error {
						resCallback = av.resultCallback
						validationFunc = av.validationFunc
						return nil
					},
				); err != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

						// Validate object (no locks held during validation)
						Error(ConstMagic9fd12c74, err).Log()
				}

				logging.Fluent(av.logger).Debug(ConstMagicb0dc7afb).
					String("object_id", taskCopy.ObjectID).
					String("object_kind", taskCopy.ObjectKind).
					Log()

				// Read file once (no lock held)
				var state *ValidationState
				var err error
				var isTimeoutError bool
				data, readErr := readValidationInput(taskCopy.FilePath)
				if readErr != nil {
					// File read errors - check if it's a missing CAS file (stale index entry)
					// CAS files have hash-based filenames (64-char hex + .yaml)
					filename := filepath.Base(taskCopy.FilePath)
					isCASFile := len(filename) == 69 && strings.HasSuffix(filename, ".yaml") && isHexString(filename[:64])

					if isCASFile && strings.Contains(readErr.Error(), ConstMagic54779b0a) {
						// Common case after discovery optimization: we may have constructed an incorrect
						// bucket path for a CAS-style hash file (we know the hash from the filename, but
						// picked the wrong bucket directory). Before declaring a stale entry, try to
						// locate the same hash file under other bucket directories.
						if av.projectRoot != emptyValue {
							if dirName := objects.GetDirectoryFromKind(taskCopy.ObjectKind); dirName != emptyValue {
								kindDir := datacell.CellCASPrimaryDir(av.projectRoot, dirName)
								// Check the main kindDir first (pre-bucketing legacy location)
								legacyPath := filepath.Join(kindDir, filename)
								if _, sErr := os.Stat(legacyPath); sErr == nil {
									if resolvedData, rErr := os.ReadFile(legacyPath); rErr == nil {
										taskCopy.FilePath = legacyPath
										data = resolvedData
										readErr = nil
									}
								}
								if readErr != nil {
									if entries, eErr := os.ReadDir(kindDir); eErr == nil {
										for _, entry := range entries {
											if !entry.IsDir() || !reBucketDateDir.MatchString(entry.Name()) {
												continue
											}
											candidate := filepath.Join(kindDir, entry.Name(), filename)
											if _, sErr := os.Stat(candidate); sErr == nil {
												if resolvedData, rErr := os.ReadFile(candidate); rErr == nil {
													taskCopy.FilePath = candidate
													data = resolvedData
													readErr = nil
													break
												}
											}
										}
									}
								}
							}
						}

						if readErr != nil {
							// Missing CAS file even after resolution attempt: likely stale CAS index entry.
							logging.Fluent(av.logger).Warn(ConstMagic7797b91f).
								String("object_id", taskCopy.ObjectID).
								String("object_kind", taskCopy.ObjectKind).
								String("file_path", taskCopy.FilePath).
								String("note", ConstMagiccc4ad9f7).
								Log()

							err = errfmt.Errorf("Missing CAS file detected: stale index entry for object %s (%s)", taskCopy.ObjectID, ConstMagiccc4ad9f7)
						}
					}

					if readErr != nil {
						// File read errors are permanent - don't retry.
						// Missing files, permission errors, etc. won't be fixed by retrying.
						if err == nil {
							err = errfmt.Newf(ConstMagic9713ffa0).Wrap(readErr)
						}

						// Emit coordinator event for file read error
						if av.eventCallback != nil {
							av.eventCallback(ConstMagicExtracted_24, taskCopy.ObjectID,
								fmt.Sprintf(ConstMagic120e0c48, taskCopy.FilePath),
								map[string]any{
									objects.FieldKeyFilePath: taskCopy.FilePath,
									"error":                  readErr.Error(),
								}, "medium")
						}

						// Skip retry logic for file read errors - fail immediately
					}
				}

				if err == nil && readErr == nil {
					// Validate with pre-read data (no lock held)
					// Add timeout to prevent validation from blocking indefinitely
					// Use kind-specific timeout (doc_entry can load referenced content and take longer)
					validationTimeout := validationTimeoutForKind(taskCopy.ObjectKind)
					validationCtx, validationCancel := context.WithTimeout(av.ctx, validationTimeout)

					// Run validation in a nested goroutine to ensure timeout is respected
					// even if the validation function does not check context cancellation (e.g. blocking file I/O).
					// This prevents deadlocks where a single blocked validation hangs the worker slot.
					var valState *ValidationState
					var valErr error
					doneChan := make(chan struct{})

					goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
						StartSimple(func() {
							func() {
								defer close(doneChan)
								valState, valErr = av.validateObjectWithFunc(validationCtx, taskCopy.ObjectID, taskCopy.ObjectKind, taskCopy.FilePath, data, validationFunc)
							}()
						})

					select {
					case <-doneChan:
						// Validation completed normally
					case <-validationCtx.Done():
						// Timeout reached before validation completed
					}
					// Check if context was cancelled (timeout)
					if validationCtx.Err() != nil {
						isTimeoutError = true
						err = errfmt.Errorf(ConstMagice69b3c5b, validationTimeout, taskCopy.ObjectID)
						logging.Fluent(av.logger).Warn(ConstMagic3eb4bae9).
							String("object_id", taskCopy.ObjectID).
							String("file_path", taskCopy.FilePath).
							String("timeout", validationTimeout.String()).
							Log()
						// CRITICAL: Always send progress update on timeout to unblock progress drain
						// This ensures completion detection works even when validation goroutines are stuck
						if !av.sendProgress(ValidationProgress{
							Status:        objects.ObjectStatusError,
							CurrentObject: taskCopy.ObjectID,
							Errors:        []string{err.Error()},
						}) {
							// Channel full - log but don't block
							logging.Fluent(av.logger).Debug(ConstMagicd47cc7ff).
								String("object_id", taskCopy.ObjectID).
								Log()
						}
						// Don't retry timeout errors - they indicate stuck operations
						// Fail immediately to free semaphore slot
					} else {
						state = valState
						err = valErr
					}
					validationCancel() // Cancel immediately after use (no defer in loops)
				}

				if err != nil {
					// Track failed validation (increment in worker's failedCount)
					// Note: We can't directly modify worker's failedCount from here,
					// but we track it via the validation result callback if needed

					// Log validation failure
					logging.Fluent(av.logger).Error(ConstMagicd10fb60c, err).
						String("object_id", taskCopy.ObjectID).
						String("object_kind", taskCopy.ObjectKind).
						String("file_path", taskCopy.FilePath).
						Log()

					// Only retry transient validation errors (not file read or timeout)
					// File read errors are permanent; timeout indicates stuck/slow work - retrying wastes resources
					isFileReadError := readErr != nil
					if !isFileReadError && !isTimeoutError && taskCopy.RetryCount < taskCopy.MaxRetries {
						taskCopy.RetryCount++
						av.priorityQueue.Enqueue(taskCopy)
						av.notifyWorkers() // Signal workers for retry
						logging.Fluent(av.logger).Debug(ConstMagic7fc49d59).
							String("object_id", taskCopy.ObjectID).
							Int("retry_count", taskCopy.RetryCount).
							Log()
						return nil
					}

					// Max retries exceeded - call result callback with error (no lock held)
					if resCallback != nil {
						resCallback(nil, err)
					}

					// Send error progress (non-blocking - drop if channel full or closed)
					av.sendProgress(ValidationProgress{
						Status:        objects.ObjectStatusError,
						CurrentObject: taskCopy.ObjectID,
						Errors:        []string{err.Error()},
					})
					return nil
				}

				// Store in cache only for kinds we persist (skip high-volume/ephemeral to avoid unbounded growth)
				if ShouldCacheValidationState(state.ObjectKind) {
					av.stateCache.Set(state)
				}

				// Call result callback to write output directly (non-blocking, no lock held)
				if resCallback != nil {
					resCallback(state, nil)
				}

				// Send completion progress (non-blocking - drop if channel full or closed)
				av.sendProgress(ValidationProgress{
					Status:        objects.ObjectStatusCompleted,
					CurrentObject: taskCopy.ObjectID,
				})
				return nil
			})
		}
	}
}
