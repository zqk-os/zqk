package system

import (
	"context"
	"strings"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/validation"
)

const (
	validationProgressStatusCompleted = "completed"
	validationProgressStatusError     = "error"
)

// startProgressDrainGoroutine starts the background goroutine to drain progress channel
func startProgressDrainGoroutine(vpc *ValidationProgressContext, ctxTimeout context.Context) {
	drainGoroutineID := getGoroutineID()
	drainActiveCount := incrementActiveGoroutines()
	logging.Fluent(vpc.Logger).Debug("Starting progress drain goroutine").
		GoroutineID(int(drainGoroutineID)).
		ActiveGoroutines(int(drainActiveCount)).
		Log()

	drainBud := goroutinelabels.DefaultBudget()
	drainBuilder := goroutinelabels.NewGoroutine("validation_progress_drain", "draining validation progress channel").
		WithCleanup(func() {
			decrementActiveGoroutines()
			close(vpc.DrainDone)
			logging.Fluent(vpc.Logger).Debug("Progress drain goroutine stopped").
				GoroutineID(int(drainGoroutineID)).
				ActiveGoroutines(int(getActiveGoroutines())).
				Log()
		})
	if drainBud != nil {
		drainBuilder = drainBuilder.WithBudget(drainBud)
	}
	drainBuilder.StartSimple(func() {

		drainCount := 0
		for {
			select {
			case <-ctxTimeout.Done():
				logging.Fluent(vpc.Logger).Debug("Drain goroutine: timeout").
					GoroutineID(int(drainGoroutineID)).
					TotalDrained(drainCount).
					Log()
				return
			case progress, ok := <-vpc.ProgressChan:
				if !ok {
					logging.Fluent(vpc.Logger).Debug("Progress channel closed, drain goroutine exiting").
						GoroutineID(int(drainGoroutineID)).
						TotalDrained(drainCount).
						ActiveGoroutines(int(getActiveGoroutines())).
						Log()
					return
				}
				drainCount++
				processProgressUpdate(vpc, progress, drainCount, drainGoroutineID)
			}
		}
	})
}

// processProgressUpdate processes a single progress update
func processProgressUpdate(vpc *ValidationProgressContext, progress validation.ValidationProgress, drainCount int, drainGoroutineID int64) {
	vpc.Mu.Lock()
	defer vpc.Mu.Unlock()

	if progress.Status == validationProgressStatusCompleted && progress.CurrentObject != emptyValue {
		vpc.Completed++
		vpc.CompletedObjectIDs[progress.CurrentObject] = true
		vpc.Metrics.IncrementValidated()

		if drainCount%100 == 0 || isDebugValidationObject(progress.CurrentObject) {
			logging.Fluent(vpc.Logger).Debug("Drain: progress update").
				GoroutineID(int(drainGoroutineID)).
				ActiveGoroutines(int(getActiveGoroutines())).
				ValidationProgressStatus(progress.Status).
				ObjectID(progress.CurrentObject).
				ValidationDrainCount(drainCount).
				Completed(vpc.Completed).
				FailedOps(vpc.Failed).
				Log()
		}
	} else if progress.Status == validationProgressStatusError {
		vpc.Failed++
		if progress.CurrentObject != emptyValue {
			vpc.CompletedObjectIDs[progress.CurrentObject] = true
			errorMsg := "Validation failed - see logs for details"
			if len(progress.Errors) > 0 {
				errorMsg = strings.Join(progress.Errors, "; ")
			}
			vpc.FailedObjectIDs[progress.CurrentObject] = errorMsg
		}
		vpc.Metrics.IncrementFailed()
		if len(progress.Errors) > 0 {
			vpc.Metrics.IncrementRetry()
		}
	}
}
