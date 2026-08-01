package provider

import (
	"context"
	"time"
)

// Retry executes a function with retry logic based on the provided configuration
func Retry(ctx context.Context, config *RetryConfig, fn func() error) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return &GraphError{
				Code:    ErrorCodeDeadlineExceeded,
				Message: "context cancelled",
				Cause:   ctx.Err(),
			}
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		graphErr, ok := err.(*GraphError)
		if !ok {
			// Not a GraphError, check if it's a timeout or context error
			if ctx.Err() != nil {
				return &GraphError{
					Code:    ErrorCodeDeadlineExceeded,
					Message: "context cancelled",
					Cause:   ctx.Err(),
				}
			}
			// Non-retryable error, return immediately
			return err
		}

		// Check if this error code is retryable
		if !graphErr.IsRetryable() {
			// Check if it's in the retryable list
			retryable := false
			for _, code := range config.RetryableErrors {
				if graphErr.Code == code {
					retryable = true
					break
				}
			}
			if !retryable {
				return err // Not retryable
			}
		}

		// Last attempt, don't wait
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return &GraphError{
				Code:    ErrorCodeDeadlineExceeded,
				Message: "context cancelled during retry",
				Cause:   ctx.Err(),
			}
		case <-time.After(delay):
			// Continue to next attempt
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * config.BackoffFactor)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	// All retries exhausted
	return &GraphError{
		Code:    ErrorCodeRetryExhausted,
		Message: "retry attempts exhausted",
		Cause:   lastErr,
	}
}

// WithTimeout wraps a context with a timeout if not already set
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		// Context already has a deadline, return as-is
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
