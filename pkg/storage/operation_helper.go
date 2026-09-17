package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objectrecord"
)

// OperationConfig configures how storage operations should be executed
type OperationConfig struct {
	// Timeout for the operation (default: 5 minutes)
	Timeout time.Duration

	// Retry configuration (nil = no retries)
	RetryConfig *RetryConfig

	// Progress callback (called periodically during long operations)
	ProgressCallback func(message string)
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxAttempts   int           // Maximum number of attempts (default: 3)
	InitialDelay  time.Duration // Initial delay before retry (default: 100ms)
	MaxDelay      time.Duration // Maximum delay between retries (default: 5s)
	BackoffFactor float64       // Exponential backoff factor (default: 2.0)
}

// DefaultOperationConfig returns a default operation configuration
func DefaultOperationConfig() *OperationConfig {
	return &OperationConfig{
		Timeout: 5 * time.Minute,
		RetryConfig: &RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      5 * time.Second,
			BackoffFactor: 2.0,
		},
	}
}

// ExecuteWithConfig executes a storage operation with standard timeout, retry, and context handling
// This works transparently for both file and graph backends
func ExecuteWithConfig[T any](
	config *OperationConfig,
	operation func(ctx context.Context) (T, error),
) (T, error) {
	if config == nil {
		config = DefaultOperationConfig()
	}

	// Create context with timeout using system context
	// Note: This function may be called without a parent context, so use system context
	systemCtx := pkgctx.NewSystemContext()
	ctx, cancel := context.WithTimeout(systemCtx, config.Timeout)
	defer cancel()

	// Execute with retry if configured
	if config.RetryConfig != nil {
		return executeWithRetry(ctx, config, operation)
	}

	// Execute without retry
	return operation(ctx)
}

// Execute executes a storage operation with default configuration
func Execute[T any](operation func(ctx context.Context) (T, error)) (T, error) {
	return ExecuteWithConfig(nil, operation)
}

// RecordObjectStateChange records a state change in the command execution tracker if available
// This should be called after successful Create, Update, or Delete operations
func RecordObjectStateChange(ctx context.Context, operation, objectID string) {
	rec := objectrecord.FromContext(ctx)
	if rec == nil {
		return // No recorder available - tracking is optional
	}

	switch operation {
	case OpCreate:
		rec.RecordObjectCreated(objectID)
	case OpUpdate:
		rec.RecordObjectUpdated(objectID)
	case OpDelete:
		rec.RecordObjectDeleted(objectID)
	}
}

// executeWithRetry executes an operation with retry logic
func executeWithRetry[T any](
	ctx context.Context,
	config *OperationConfig,
	operation func(ctx context.Context) (T, error),
) (T, error) {
	var zero T
	var lastErr error
	delay := config.RetryConfig.InitialDelay

	for attempt := 0; attempt < config.RetryConfig.MaxAttempts; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		// Show progress if this is a retry
		if attempt > 0 && config.ProgressCallback != nil {
			config.ProgressCallback(fmt.Sprintf("Retry attempt %d/%d...", attempt+1, config.RetryConfig.MaxAttempts))
		}

		// Execute the operation
		result, err := operation(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable (context/timeout errors are retryable)
		if !isRetryableError(err) {
			return zero, err
		}

		// Last attempt, don't wait
		if attempt == config.RetryConfig.MaxAttempts-1 {
			break
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * config.RetryConfig.BackoffFactor)
		if delay > config.RetryConfig.MaxDelay {
			delay = config.RetryConfig.MaxDelay
		}
	}

	// All retries exhausted
	return zero, errfmt.Errorf(ConstMiscOperationFailedAfterDAttemptsW, config.RetryConfig.MaxAttempts, lastErr)
}

// RetryableErrorChecker is a function type for checking if an error is retryable
// If nil, uses default isRetryableError
type RetryableErrorChecker func(err error) bool

// ExecuteSimpleRetry executes an operation with retry logic (simplified version without generics)
// This is useful for operations that don't return values
// errorChecker: optional custom error checker (nil = use default isRetryableError)
func ExecuteSimpleRetry(ctx context.Context, config *RetryConfig, operation func() error, errorChecker RetryableErrorChecker) error {
	if config == nil {
		config = &RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      5 * time.Second,
			BackoffFactor: 2.0,
		}
	}

	// Use default error checker if none provided
	if errorChecker == nil {
		errorChecker = isRetryableError
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Check context cancellation
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute the operation
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable using provided checker
		if !errorChecker(err) {
			return err
		}

		// Last attempt, don't wait
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Wait before retry with exponential backoff
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// Continue to next attempt
			}
		} else {
			time.Sleep(delay)
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * config.BackoffFactor)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	// All retries exhausted
	return errfmt.Errorf(ConstMiscOperationFailedAfterDAttemptsW, config.MaxAttempts, lastErr)
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Context/timeout errors are retryable
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// Check for timeout in error message (heuristic)
	errStr := err.Error()
	if containsError(errStr, "timeout") || containsError(errStr, "deadline") || containsError(errStr, ConstMiscContextCanceled) {
		return true
	}

	// Network/connection errors are retryable
	if containsError(errStr, "connection") || containsError(errStr, "network") || containsError(errStr, "temporary") {
		return true
	}

	// By default, don't retry (safer)
	return false
}

// containsError checks if an error message contains a substring (case-insensitive)
func containsError(errStr, substr string) bool {
	if len(errStr) < len(substr) {
		return false
	}
	substrLower := toLower(substr)
	errStrLower := toLower(errStr)

	// Check if substr is at start, end, or middle
	if errStrLower == substrLower {
		return true
	}
	if len(errStrLower) > len(substrLower) {
		if errStrLower[:len(substrLower)] == substrLower ||
			errStrLower[len(errStrLower)-len(substrLower):] == substrLower {
			return true
		}
		// Check middle
		for i := 0; i <= len(errStrLower)-len(substrLower); i++ {
			if errStrLower[i:i+len(substrLower)] == substrLower {
				return true
			}
		}
	}
	return false
}

func toLower(s string) string {
	// Simple lowercase conversion
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + ('a' - 'A')
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// ListWithConfig executes a List operation with standard configuration
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func ListWithConfig(
	storage ObjectStorageProvider,
	config *OperationConfig,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	filter ListFilter,
) (*QueryResult, error) {
	return ExecuteWithConfig(config, func(ctx context.Context) (*QueryResult, error) {
		if config.ProgressCallback != nil {
			config.ProgressCallback(fmt.Sprintf(ConstMiscQueryingSObjects, filter.Kind))
		}
		return storage.List(ctx, secCtx, storageCtx, filter)
	})
}

// List executes a List operation with default configuration
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func List(
	storage ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	filter ListFilter,
) (*QueryResult, error) {
	return ListWithConfig(storage, DefaultOperationConfig(), secCtx, storageCtx, filter)
}
