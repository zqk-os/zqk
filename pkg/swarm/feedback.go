package swarm

import (
	"context"
	"errors"
	"fmt"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"github.com/zqk-os/zqk/pkg/logging"
	"golang.org/x/sync/errgroup"
)

// FeedbackProcessor handles the feedback loop for failed tasks.
type FeedbackProcessor struct {
	queue TaskQueue
}

// NewFeedbackProcessor creates a new FeedbackProcessor.
func NewFeedbackProcessor(queue TaskQueue) *FeedbackProcessor {
	return &FeedbackProcessor{queue: queue}
}

// Process consumes results from the dispatcher and feeds failed tasks back into the queue.
func (f *FeedbackProcessor) Process(ctx context.Context, results <-chan TaskResult) <-chan TaskResult {
	finalResults := make(chan TaskResult)
	logger := logging.GetLogger()

	eg, gCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		defer close(finalResults)
		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case res, ok := <-results:
				if !ok {
					return nil
				}
				if res.Error != nil {
					if errors.Is(res.Error, context.Canceled) {
						select {
						case <-gCtx.Done():
							return gCtx.Err()
						case finalResults <- res:
						}
						continue
					}
					if res.Task.CurrentRetries < res.Task.MaxRetries {
						res.Task.CurrentRetries++
						// Add feedback context to the prompt
						res.Task.UserPrompt += fmt.Sprintf("\n\n[System Feedback: Previous attempt failed with error: %v. Please adjust your approach and try again.]", res.Error)

						logging.FluentEvent(logger).Info("Retrying failed task").
							WithFields(
								logging.String("taskID", res.Task.ID),
								logging.Int("attempt", res.Task.CurrentRetries),
								logging.Error(res.Error),
							).Log()

						if err := f.queue.Enqueue(gCtx, res.Task); err != nil {
							logging.FluentEvent(logger).Error("Failed to enqueue retry", err).Log()
							select {
							case <-gCtx.Done():
								return gCtx.Err()
							case finalResults <- res:
							}
						}
						continue
					} else {
						logging.FluentEvent(logger).Warn("Task reached max retries").
							WithFields(logging.String("taskID", res.Task.ID)).Log()
					}
				}
				// If successful or max retries reached, pass it to final results
				select {
				case <-gCtx.Done():
					return gCtx.Err()
				case finalResults <- res:
				}
			}
		}
	})

	// Fire and forget the wait in the background to ensure any errors are logged
	// (though most are expected context cancellations).
	goroutinelabels.StartNamedGoroutine("swarm-feedback-processor", "process swarm feedback", func() {
		func(ctx context.Context) {
			if err := eg.Wait(); err != nil && err != context.Canceled {
				select {
				case <-ctx.Done():
					return
				default:
					logging.FluentEvent(logger).Error("FeedbackProcessor errgroup error", err).Log()
				}
			}
		}(ctx)
	})

	return finalResults
}
