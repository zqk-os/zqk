package validation

import (
	"context"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// asyncValidatorShutdownPipelineKind is the general identity for logs/metrics; stage names add specificity.
const asyncValidatorShutdownPipelineKind = "validation.async_validator.shutdown"

type noopAsyncValidatorShutdownMetrics struct{}

func (noopAsyncValidatorShutdownMetrics) RecordStage(context.Context, string, string, time.Duration, error) {
}

func (noopAsyncValidatorShutdownMetrics) RecordStageWithBuckets(context.Context, string, string, time.Duration, error, map[string]string) {
}

type asyncValidatorStopPayload struct {
	av *AsyncValidator

	workerTimeout time.Duration
	cacheTimeout  time.Duration
}

// runAsyncValidatorStopPipeline runs ordered shutdown: log shutdown → wait workers → log stopped →
// persist cache → close progress channel → close decision-logger file destinations (TryCloseLoggerDestinations).
// workerTimeout and cacheTimeout must come from the same critical section that cancelled av.ctx (see Stop).
func runAsyncValidatorStopPipeline(workerTimeout, cacheTimeout time.Duration, av *AsyncValidator) error {
	p := &asyncValidatorStopPayload{
		av:            av,
		workerTimeout: workerTimeout,
		cacheTimeout:  cacheTimeout,
	}
	pl := pipeline.NewBuilder(asyncValidatorShutdownPipelineKind, av.logger).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopAsyncValidatorShutdownMetrics{}, Strategy: pipeline.NoopBucketing{}}).
		WithProfile("system").
		AddStage("FINALIZE_log_shutting_down", asyncValidatorStopStageLogShuttingDown).
		AddStage("FINALIZE_wait_workers", asyncValidatorStopStageWaitWorkers).
		AddStage("FINALIZE_log_stopped", asyncValidatorStopStageLogStopped).
		AddStage("FINALIZE_persist_cache", asyncValidatorStopStagePersistCache).
		AddStage("FINALIZE_close_progress", asyncValidatorStopStageCloseProgress).
		AddStage("FINALIZE_close_logger_destinations", asyncValidatorStopStageCloseLoggerDestinations).
		Build()

	pctx := &pipeline.Context{
		Ctx:     context.Background(),
		Outcome: make(map[string]any),
	}
	_, err := pl.Run(pctx, p)
	return err
}

func asyncValidatorStopStageLogShuttingDown(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidatorStopPayload)
	av := p.av

	queueSize := av.priorityQueue.Size()
	activeWorkers := int(av.activeWorkers.Load())
	activeGoroutines := int(getActiveGoroutines())
	logging.Fluent(av.logger).Info("Stopping async validator").
		ActiveWorkers(activeWorkers).
		ActiveGoroutines(activeGoroutines).
		QueueSize(queueSize).
		WorkerStopTimeout(p.workerTimeout.String()).
		Log()

	av.workerStates.Range(func(key, value any) bool {
		logging.Fluent(av.logger).Debug("Worker state before stop").
			WorkerID(key.(int)).
			String("state", value.(string)).
			Log()
		return true
	})
	return payload, nil
}

func asyncValidatorStopStageWaitWorkers(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidatorStopPayload)
	av := p.av

	stopComplete := make(chan struct{})
	stopCtx, stopCancel := context.WithTimeout(context.Background(), p.workerTimeout)
	defer stopCancel()

	stopBud := goroutinelabels.DefaultBudget()
	stopWaitBuilder := goroutinelabels.NewGoroutine("async_validator_stop_wait", asyncValidatorShutdownPipelineKind+"/FINALIZE_wait_workers").
		WithCleanup(func() {
			close(stopComplete)
		})
	if stopBud != nil {
		stopWaitBuilder = stopWaitBuilder.WithBudget(stopBud)
	}
	stopWaitBuilder.StartWithContext(stopCtx, func(ctx context.Context) error {
		av.wg.Wait()
		return nil
	})

	select {
	case <-stopComplete:
		logging.Fluent(av.logger).Info("All workers stopped").Log()
	case <-stopCtx.Done():
		av.workerStopTimedOut.Store(true)
		remainingWorkers := int(av.activeWorkers.Load())
		remainingGoroutines := int(getActiveGoroutines())
		remainingQueueSize := av.priorityQueue.Size()
		logging.Fluent(av.logger).Warn("Timeout waiting for workers to stop - some workers may be stuck").
			ActiveWorkers(remainingWorkers).
			ActiveGoroutines(remainingGoroutines).
			QueueSize(remainingQueueSize).
			Timeout(p.workerTimeout.String()).
			DiagnosticDetail("Workers may be blocked on validation, I/O, or locks").
			Log()
		workerCount := 0
		av.workerStates.Range(func(key, value any) bool {
			workerCount++
			logging.Fluent(av.logger).Warn("Worker still active after stop timeout").
				WorkerID(key.(int)).
				WorkerStateLabel(value.(string)).
				Log()
			return true
		})
		if workerCount == 0 && remainingWorkers > 0 {
			logging.Fluent(av.logger).Debug("Worker state snapshot empty at stop timeout (workers may have exited immediately after)").
				ActiveWorkersRemainingAtTimeout(remainingWorkers).
				Log()
		}
	}
	return payload, nil
}

func asyncValidatorStopStageLogStopped(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidatorStopPayload)
	av := p.av
	logging.Fluent(av.logger).Info("Async validator stopped").
		ActiveGoroutines(int(getActiveGoroutines())).
		Log()
	return payload, nil
}

func asyncValidatorStopStagePersistCache(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidatorStopPayload)
	av := p.av

	stateCount, staleCount, withIssuesCount := av.stateCache.Count()
	saveTimeout := p.cacheTimeout
	if stateCount > 0 {
		extra := time.Duration(stateCount/1000) * 6 * time.Second
		maxSave := 3 * time.Minute
		if capped := p.cacheTimeout + extra; capped <= maxSave {
			saveTimeout = capped
		} else {
			saveTimeout = maxSave
		}
	}
	saveDone := make(chan error, 1)
	saveCtx, saveCancel := context.WithTimeout(context.Background(), saveTimeout)
	defer saveCancel()

	cacheSaveBud := goroutinelabels.DefaultBudget()
	cacheSaveBuilder := goroutinelabels.NewGoroutine("async_validator_cache_saver", asyncValidatorShutdownPipelineKind+"/FINALIZE_persist_cache")
	if cacheSaveBud != nil {
		cacheSaveBuilder = cacheSaveBuilder.WithBudget(cacheSaveBud)
	}
	cacheSaveBuilder.StartWithContext(saveCtx, func(ctx context.Context) error {
		const maxRetries = 4
		backoff := 200 * time.Millisecond
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				if backoff > 2*time.Second {
					backoff = 2 * time.Second
				}
				select {
				case <-ctx.Done():
					err = ctx.Err()
					break
				case <-time.After(backoff):
					backoff += 200 * time.Millisecond
				}
			}
			if ctx.Err() != nil {
				break
			}
			err = av.stateCache.Save()
			if err == nil {
				break
			}
			if !strings.Contains(err.Error(), "cache file is locked by another process") {
				break
			}
		}
		select {
		case saveDone <- err:
		case <-ctx.Done():
		}
		return nil
	})

	select {
	case err := <-saveDone:
		if err != nil {
			logging.Fluent(av.logger).Warn("Failed to save validation cache").
				WithError(err).
				Log()
		}
	case <-saveCtx.Done():
		av.cacheSaveTimedOut.Store(true)
		logging.Fluent(av.logger).Warn("Timeout saving validation cache - cache may not be saved").
			String("timeout", saveTimeout.String()).
			Int("state_count", stateCount).
			Int("stale_count", staleCount).
			Int("with_issues_count", withIssuesCount).
			String("diagnostic", "Cache save may be slow due to large state count, file locking, or disk I/O").
			Log()
	}
	return payload, nil
}

func asyncValidatorStopStageCloseProgress(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidatorStopPayload)
	av := p.av
	err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorStopCloseChannel,
		lockLoggerSystem(),
		func() error {
			if !av.progressClosed {
				close(av.progressChan)
				av.progressClosed = true
			}
			return nil
		},
	)
	return payload, err
}

func asyncValidatorStopStageCloseLoggerDestinations(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidatorStopPayload)
	av := p.av
	if err := logging.TryCloseLoggerDestinations(av.logger); err != nil {
		// Last resort: primary logger may be partially closed; use system profile logger.
		sys := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(sys).Warn("Async validator: close logger destinations returned error").
			WithError(err).
			Log()
	}
	return payload, nil
}
