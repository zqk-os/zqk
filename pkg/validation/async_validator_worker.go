package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var reBucketDateDir = regexp.MustCompile(`^(\d{4}-\d{2}(-\d{2})?|[a-z0-9])$`)

// validationTimeoutForKind returns the per-object validation timeout for a kind.
// Uses config from config/zqk.yaml validation.per_object_timeout (default_seconds, kind_overrides).
func validationTimeoutForKind(kind string) time.Duration {
	return GetGlobalValidationTimeoutConfig().TimeoutForKind(kind)
}

// awaitNestedValidation waits for the nested validate goroutine. On timeout it still
// blocks until done so the caller's semaphore cleanup cannot run while validate is in-flight.
func awaitNestedValidation(done <-chan struct{}, timeout <-chan struct{}) (timedOut bool) {
	select {
	case <-done:
		return false
	case <-timeout:
		<-done
		return true
	}
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
		return fileutil.ReadFile(path)
	}

	segmentPath := path[:sepIdx]
	offsetStr := path[sepIdx+2:]
	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		return fileutil.ReadFile(path)
	}

	f, err := fileutil.Open(segmentPath)
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
					Error("lock failed in worker shutdown callback: %v\n", err).Log()
			}

			callback(
				av.ctx,
				projectRoot,
				nil, // Storage provider not available in validation package
				fmt.Sprintf("worker_%d", id), "worker_shutdown", "stopped",
				int(av.activeWorkers.Load()),
				processedCount,
				failedCount,
				time.Since(startTime),
			)
		}

		logging.Fluent(av.logger).Debug("Worker stopped").
			WorkerID(id).
			GoroutineID(int(goroutineID)).
			ActiveGoroutines(int(getActiveGoroutines())).
			Log()
	}()

	av.workerStates.Store(id, "running")
	logging.Fluent(av.logger).Debug("Worker started").
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
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in worker start callback: %v\n", err).Log()
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
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in queue empty check: %v\n", err).Log()
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
					Error("lock failed in queue size update: %v\n", err).Log()
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
					logging.Fluent(av.logger).Info("Validation worker shutting down due to idle timeout").
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
								Error("lock failed in idle shutdown callback: %v\n", err).Log()
						}

						callback(
							av.ctx,
							projectRoot,
							nil, // Storage provider not available in validation package
							fmt.Sprintf("worker_%d", id), "worker_idle_shutdown", "idle_shutdown",
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
			validationGoroutineName := fmt.Sprintf("validation_task_%s", task.ObjectID)
			validationGoroutinePurpose := fmt.Sprintf("validating %s (%s)", task.ObjectID, task.ObjectKind)

			// Acquire semaphore slot with timeout to prevent indefinite blocking.
			// Foreign host CPU (AV, other tenants) can refuse extra slots; self-heat does not
			// bounce a held slot (TDE-CEF-HOST-CPU-BACKPRESSURE-001).
			held, timedOut := av.holdHostAwareValidationSlot(taskCopy.ObjectID)
			if timedOut {
				// Slot wait expired. Re-enqueue always. Warn/coordinator only when the
				// queue is not draining — a shrinking 8k check is backpressure, not stuck.
				queueSize := av.priorityQueue.Size()
				shouldWarn := true
				if err := concurrency.RunInLockWithLogger(
					&av.mu,
					LockNameAsyncValidatorSemaphoreFullSample,
					lockLoggerSystem(),
					func() error {
						shouldWarn = semaphoreFullShouldWarn(av.lastSemaphoreFullQueueSize, queueSize)
						av.lastSemaphoreFullQueueSize = queueSize
						return nil
					},
				); err != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
						Error("Failed to sample semaphore-full queue size", err).Log()
				}

				logSemaphoreFull := func(e *logging.FluentEntry) {
					e.ObjectID(taskCopy.ObjectID).
						QueueSize(queueSize).
						SemaphoreCapacity(cap(av.validationSemaphore)).
						WaitTime(semaphoreFullWaitTimeout.String()).
						Log()
				}
				if shouldWarn {
					logSemaphoreFull(logging.Fluent(av.logger).Warn("Semaphore full, validation goroutines may be stuck"))
				} else {
					logSemaphoreFull(logging.Fluent(av.logger).Debug("Semaphore full, validation goroutines may be stuck"))
				}

				if shouldWarn && av.eventCallback != nil {
					av.eventCallback("semaphore_full", taskCopy.ObjectID,
						fmt.Sprintf("Semaphore full (capacity: %d), validation goroutines may be stuck", cap(av.validationSemaphore)),
						map[string]any{
							"queue_size": queueSize, "semaphore_capacity": cap(av.validationSemaphore),
						}, "high")
				}

				av.priorityQueue.Enqueue(taskCopy)
				av.notifyWorkers() // Signal workers (might free up a slot)
				failedCount++      // Track failed enqueue due to semaphore
				continue           // Continue worker loop to process other tasks
			}
			if !held {
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
						Error("lock failed in getting callbacks: %v\n", err).Log()
				}

				logging.Fluent(av.logger).Debug("Validating object").
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

					if isCASFile && strings.Contains(readErr.Error(), "no such file or directory") {
						// Common case after discovery optimization: we may have constructed an incorrect
						// bucket path for a CAS-style hash file (we know the hash from the filename, but
						// picked the wrong bucket directory). Before declaring a stale entry, try to
						// locate the same hash file under other bucket directories.
						if av.projectRoot != emptyValue {
							if dirName := objects.GetDirectoryFromKind(taskCopy.ObjectKind); dirName != emptyValue {
								kindDir := datacell.CellCASPrimaryDir(av.projectRoot, dirName)
								// Check the main kindDir first (pre-bucketing legacy location)
								legacyPath := filepath.Join(kindDir, filename)
								if _, sErr := fileutil.Stat(legacyPath); sErr == nil {
									if resolvedData, rErr := fileutil.ReadFile(legacyPath); rErr == nil {
										taskCopy.FilePath = legacyPath
										data = resolvedData
										readErr = nil
									}
								}
								if readErr != nil {
									if entries, eErr := fileutil.ReadDir(kindDir); eErr == nil {
										for _, entry := range entries {
											if !entry.IsDir() || !reBucketDateDir.MatchString(entry.Name()) {
												continue
											}
											candidate := filepath.Join(kindDir, entry.Name(), filename)
											if _, sErr := fileutil.Stat(candidate); sErr == nil {
												if resolvedData, rErr := fileutil.ReadFile(candidate); rErr == nil {
													taskCopy.FilePath = candidate
													data = resolvedData
													readErr = nil
													break
												}
											}
										}
									}
								}
								// Object-id-cache / discovery may still name a deleted hash after
								// update. Prefer the live listing-index mapping before blaming CAS.
								// TRACK: follow-up in kernel backlog
								if readErr != nil {
									if livePath, liveData, ok := readLiveCASBlobFromIndex(kindDir, taskCopy.ObjectKind, taskCopy.ObjectID); ok {
										taskCopy.FilePath = livePath
										data = liveData
										readErr = nil
									}
								}
							}
						}

						if readErr != nil {
							// Missing CAS blob and no live index mapping: ghost cache path or true
							// stale listing-index entry (deleted object).
							logging.Fluent(av.logger).Warn("Missing CAS file detected - stale index entry").
								String("object_id", taskCopy.ObjectID).
								String("object_kind", taskCopy.ObjectKind).
								String("file_path", taskCopy.FilePath).
								String("note", "CAS index entry exists but hash file is missing - recovery needed").
								Log()

							err = errfmt.Errorf("Missing CAS file detected: stale index entry for object %s (%s)", taskCopy.ObjectID, "CAS index entry exists but hash file is missing - recovery needed")
						}
					}

					if readErr != nil {
						// File read errors are permanent - don't retry.
						// Missing files, permission errors, etc. won't be fixed by retrying.
						if err == nil {
							err = errfmt.Newf("failed to read file").Wrap(readErr)
						}

						// Emit coordinator event for file read error
						if av.eventCallback != nil {
							av.eventCallback("file_read_error", taskCopy.ObjectID,
								fmt.Sprintf("Failed to read file: %s", taskCopy.FilePath),
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

					// Nested goroutine so a ctx-ignoring validate (blocking I/O) can still trip
					// the fail-fast budget for progress. The semaphore MUST stay held until that
					// goroutine returns: releasing on timeout leaked one OS thread (M) per object
					// and starved the host (1116 threads in sample). A stuck object occupying one
					// of ≤8 slots is the fail-closed trade. TRACK
					var valState *ValidationState
					var valErr error
					doneChan := make(chan struct{})

					goroutinelabels.NewGoroutine("validation_nested", "per-object validateObjectWithFunc").
						StartSimple(func() {
							defer close(doneChan)
							valState, valErr = av.validateObjectWithFunc(validationCtx, taskCopy.ObjectID, taskCopy.ObjectKind, taskCopy.FilePath, data, validationFunc)
						})

					timedOut := awaitNestedValidation(doneChan, validationCtx.Done())
					if timedOut {
						// Under fan-out load the 5s budget can fire on objects that already
						// validated cleanly. Prefer a finished result, then prior cache, then error.
						if valState != nil && valErr == nil {
							state = valState
							err = nil
						} else if prev, ok := av.GetCachedState(taskCopy.ObjectID); ok && isReusablePriorValidationState(prev) {
							logging.Fluent(av.logger).Info("validation timed out; retaining prior non-blocking cached state").
								String("object_id", taskCopy.ObjectID).
								String("file_path", taskCopy.FilePath).
								String("timeout", validationTimeout.String()).
								Log()
							state = prev
							err = nil
						} else {
							isTimeoutError = true
							err = errfmt.Errorf("validation timeout after %s for %s", validationTimeout, taskCopy.ObjectID)
							logging.Fluent(av.logger).Warn("Validation timeout - validation function may be stuck").
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
								logging.Fluent(av.logger).Debug("Progress channel full on timeout, skipping update").
									String("object_id", taskCopy.ObjectID).
									Log()
							}
						}
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
					logging.Fluent(av.logger).Error("Validation failed", err).
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
						logging.Fluent(av.logger).Debug("Retrying validation").
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
