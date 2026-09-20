package logging

import (
	"github.com/zqk-os/zqk/pkg/concurrency"
)

// LockLoggerAdapter adapts logging.Logger to concurrency.LockLogger
// This allows pkg/concurrency to use logging without creating import cycles
type LockLoggerAdapter struct {
	logger Logger
}

// NewLockLoggerAdapter creates an adapter that makes a logging.Logger implement concurrency.LockLogger
func NewLockLoggerAdapter(logger Logger) concurrency.LockLogger {
	if logger == nil {
		return nil
	}
	return &LockLoggerAdapter{logger: logger}
}

// GetLockLoggerFromProfile is a convenience function that gets a logger and wraps it as LockLogger
// Use this when calling concurrency.WithLockTimeout or concurrency.WithRLockTimeout
func GetLockLoggerFromProfile(profile string) concurrency.LockLogger {
	logger := GetLoggerFromProfile(profile)
	return NewLockLoggerAdapter(logger)
}

// Debug implements concurrency.LockLogger
func (a *LockLoggerAdapter) Debug(msg string, fields ...concurrency.LockField) {
	if a == nil || a.logger == nil {
		return
	}
	if len(fields) == 0 {
		Fluent(a.logger).Debug(msg).Log()
		return
	}
	logFields := make([]Field, len(fields))
	for i, f := range fields {
		logFields[i] = Field{Key: f.Key, Value: f.Value}
	}
	Fluent(a.logger).Debug(msg).WithFields(logFields...).Log()
}

// Warn implements concurrency.LockLogger
func (a *LockLoggerAdapter) Warn(msg string, fields ...concurrency.LockField) {
	if a == nil || a.logger == nil {
		return
	}
	if len(fields) == 0 {
		Fluent(a.logger).Warn(msg).Log()
		return
	}
	logFields := make([]Field, len(fields))
	for i, f := range fields {
		logFields[i] = Field{Key: f.Key, Value: f.Value}
	}
	Fluent(a.logger).Warn(msg).WithFields(logFields...).Log()
}
