package validation

import (
	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// SetValidationFunc sets the validation function to use for object validation
// This allows the async validator to use the same validation logic as sync check
func (av *AsyncValidator) SetValidationFunc(fn ValidationFunc) {
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorSetValidationFunc,
		lockLoggerSystem(),
		func() error {
			av.validationFunc = fn
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ErrMsgSetValFunc, err).Log()
	}
}

// SetResultCallback sets the callback function to be called when validation completes
// This allows writing output directly from the validation goroutine
func (av *AsyncValidator) SetResultCallback(callback ResultCallback) {
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorSetResultCallback,
		lockLoggerSystem(),
		func() error {
			av.resultCallback = callback
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ErrMsgSetResCallback, err).Log()
	}
}

// SetEventCallback sets the callback for emitting coordinator events
// This enables metrics, logs, and failures to be captured via the coordinator framework
// The callback is called from validation goroutines, so it must be thread-safe and non-blocking
func (av *AsyncValidator) SetEventCallback(callback ValidationEventCallback) {
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorSetEventCallback,
		lockLoggerSystem(),
		func() error {
			av.eventCallback = callback
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ErrMsgSetEvCallback, err).Log()
	}
}

// SetQueueEmptyCallback sets a callback that fires when the queue becomes empty
// This allows immediate completion detection without waiting for ticker updates
// The callback is called from worker goroutines, so it must be thread-safe and non-blocking
func (av *AsyncValidator) SetQueueEmptyCallback(callback func()) {
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorSetQueueEmptyCallback,
		lockLoggerSystem(),
		func() error {
			av.queueEmptyCallback = callback
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ErrMsgSetQEmptyCallback, err).Log()
	}
}
