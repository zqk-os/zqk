package context

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

const emptyPipelineValue = ""

// ProcessingResult represents the result of processing a context through the pipeline
type ProcessingResult struct {
	// State is the final state reached
	State ContextState

	// Results contains results from all listeners (map of listener name -> result)
	Results map[string]any

	// Errors contains errors from listeners (map of listener name -> error)
	Errors map[string]error

	// FinalResult is the final result after all processing (if any listener returned a result)
	FinalResult any

	// FinalError is the final error if processing failed
	FinalError error

	// Duration is how long the processing took
	Duration time.Duration
}

// ProcessableContext is an interface for context objects that can be processed through the pipeline
type ProcessableContext interface {
	// GetState returns the current state of the context
	GetState() ContextState

	// SetState sets the state of the context
	SetState(state ContextState)

	// GetContext returns the Go context.Context for cancellation/timeout
	GetContext() context.Context
}

// ProcessContext processes a context object through the listener pipeline
// It triggers listeners based on the context's state and processes them asynchronously
// Returns a channel that will receive the final ProcessingResult when complete
func ProcessContext(ctxObj ProcessableContext) <-chan *ProcessingResult {
	resultChan := make(chan *ProcessingResult, 1)

	goroutinelabels.NewGoroutine("context_pipeline_processor", "processing context through listener pipeline").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartWithContext(ctxObj.GetContext(), func(ctx context.Context) error {
			startTime := time.Now()
			result := &ProcessingResult{
				Results: make(map[string]any),
				Errors:  make(map[string]error),
			}

			defer func() {
				result.Duration = time.Since(startTime)
				result.State = ctxObj.GetState()
				select {
				case resultChan <- result:
				case <-ctx.Done():
					// Context cancelled, skip sending result
				}
			}()

			// Get the current state
			currentState := ctxObj.GetState()

			// Get listeners for the current state
			listeners := GetListeners(currentState)
			if len(listeners) == 0 {
				// No listeners registered, mark as completed
				ctxObj.SetState(StateCompleted)
				return nil
			}

			// Set state to processing
			ctxObj.SetState(StateProcessing)

			// Process listeners
			// Separate async and sync listeners
			asyncListeners := make([]registeredListener, 0)
			syncListeners := make([]registeredListener, 0)

			for _, reg := range listeners {
				if reg.config.Async {
					asyncListeners = append(asyncListeners, reg)
				} else {
					syncListeners = append(syncListeners, reg)
				}
			}

			// Process sync listeners first (in order)
			for _, reg := range syncListeners {
				select {
				case <-ctx.Done():
					ctxObj.SetState(StateCancelled)
					result.FinalError = ctx.Err()
					return nil
				default:
				}

				listenerResult, err := executeListener(ctx, reg, ctxObj)
				if reg.config.Name != emptyPipelineValue {
					if err != nil {
						result.Errors[reg.config.Name] = err
						if reg.config.Required {
							ctxObj.SetState(StateFailed)
							result.FinalError = errfmt.Errorf("required listener %s failed: %w", reg.config.Name, err)
							return nil
						}
					} else {
						result.Results[reg.config.Name] = listenerResult
						if listenerResult != nil {
							result.FinalResult = listenerResult
						}
					}
				}
			}

			// Process async listeners in parallel
			if len(asyncListeners) > 0 {
				var wg sync.WaitGroup
				asyncResults := make(chan listenerResult, len(asyncListeners))

				for _, reg := range asyncListeners {
					regCopy := reg
					// Create a context with timeout if specified
					listenerCtx := ctx
					if regCopy.config.Timeout > 0 {
						var cancel context.CancelFunc
						listenerCtx, cancel = context.WithTimeout(ctx, regCopy.config.Timeout)
						// Cancel context when goroutine completes (via cleanup)
						goroutinelabels.NewGoroutine(fmt.Sprintf("context_listener_%s", regCopy.config.Name), fmt.Sprintf("executing listener %s", regCopy.config.Name)).
							WithWaitGroup(&wg).
							WithCleanup(func() {
								cancel()
							}).
							StartWithContext(listenerCtx, func(ctx context.Context) error {
								result, err := executeListener(ctx, regCopy, ctxObj)
								select {
								case asyncResults <- listenerResult{
									name:   regCopy.config.Name,
									result: result,
									err:    err,
								}:
								case <-ctx.Done():
									// Context cancelled, skip sending result
								}
								return nil
							})
						continue
					}
					goroutinelabels.NewGoroutine(fmt.Sprintf("context_listener_%s", regCopy.config.Name), fmt.Sprintf("executing listener %s", regCopy.config.Name)).
						WithWaitGroup(&wg).
						StartWithContext(listenerCtx, func(ctx context.Context) error {
							result, err := executeListener(ctx, regCopy, ctxObj)
							select {
							case asyncResults <- listenerResult{
								name:   regCopy.config.Name,
								result: result,
								err:    err,
							}:
							case <-ctx.Done():
								// Context cancelled, skip sending result
							}
							return nil
						})
				}

				// Wait for all async listeners to complete
				goroutinelabels.NewGoroutine("context_listener_collector", "collecting async listener results").
					WithCleanup(func() {
						close(asyncResults)
					}).
					StartWithContext(ctx, func(ctx context.Context) error {
						// WaitGroup.Wait() can't be cancelled, but we check context in the select below
						wg.Wait()
						return nil
					})

				// Collect async results
				for asyncResult := range asyncResults {
					if asyncResult.name != emptyPipelineValue {
						if asyncResult.err != nil {
							result.Errors[asyncResult.name] = asyncResult.err
							// Check if this was a required listener
							for _, reg := range asyncListeners {
								if reg.config.Name == asyncResult.name && reg.config.Required {
									ctxObj.SetState(StateFailed)
									result.FinalError = errfmt.Errorf("required async listener %s failed: %w", asyncResult.name, asyncResult.err)
									return nil
								}
							}
						} else {
							result.Results[asyncResult.name] = asyncResult.result
							if asyncResult.result != nil {
								result.FinalResult = asyncResult.result
							}
						}
					}
				}
			}

			// If we got here, all processing completed successfully
			ctxObj.SetState(StateCompleted)
			return nil
		})

	return resultChan
}

type listenerResult struct {
	name   string
	result any
	err    error
}

func executeListener(ctx context.Context, reg registeredListener, ctxObj any) (any, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Execute the listener
	return reg.listener(ctx, ctxObj)
}

// WaitForResult waits for a processing result with an optional timeout
func WaitForResult(resultChan <-chan *ProcessingResult, timeout time.Duration) (*ProcessingResult, error) {
	if timeout > 0 {
		select {
		case result := <-resultChan:
			return result, nil
		case <-time.After(timeout):
			return nil, errfmt.Errorf("timeout waiting for processing result")
		}
	}

	result := <-resultChan
	return result, nil
}
