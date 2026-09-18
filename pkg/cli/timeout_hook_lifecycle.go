package cli

import (
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// GetTimeoutHook returns the global timeout hook instance
func GetTimeoutHook() *TimeoutHook {
	hookOnce.Do(func() {
		globalTimeoutHook = NewTimeoutHook()
	})
	return globalTimeoutHook
}

// NewTimeoutHook creates a new timeout hook
func NewTimeoutHook() *TimeoutHook {
	return &TimeoutHook{
		enabled:         true,
		maxTimeout:      24 * time.Hour, // Default max timeout (permits long-running swarm / daemon commands)
		commandTimeouts: make(map[string]time.Duration),
		logger:          logging.NewEventLogger(pkgctx.NewSystemContext()),
		timeoutConfig:   nil, // Use global config by default
	}
}

// SetEnabled enables or disables the timeout hook
func (h *TimeoutHook) SetEnabled(enabled bool) {
	_ = concurrency.RunInLockWithLogger(
		&h.mu, LockNameTimeoutHookSetEnabled, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.enabled = enabled
			return nil
		},
	)
}

// SetMaxTimeout sets the maximum timeout for commands
func (h *TimeoutHook) SetMaxTimeout(timeout time.Duration) {
	_ = concurrency.RunInLockWithLogger(
		&h.mu, LockNameTimeoutHookSetMaxTimeout, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.maxTimeout = timeout
			return nil
		},
	)
}

// SetCommandTimeout sets a specific timeout for a command (e.g., "mcp serve" -> 5m)
// This overrides the default timeout calculation for that specific command
func (h *TimeoutHook) SetCommandTimeout(command string, timeout time.Duration) {
	_ = concurrency.RunInLockWithLogger(
		&h.mu, LockNameTimeoutHookSetCommandTimeout, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if h.commandTimeouts == nil {
				h.commandTimeouts = make(map[string]time.Duration)
			}
			h.commandTimeouts[command] = timeout
			return nil
		},
	)
}

// SetMetricsStore sets the metrics store
func (h *TimeoutHook) SetMetricsStore(store MetricsStore) {
	_ = concurrency.RunInLockWithLogger(
		&h.mu, LockNameTimeoutHookSetMetricsStore, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.metricsStore = store
			return nil
		},
	)
}

// getTimeoutConfig returns the timeout configuration (uses global config if hook config is nil)
func (h *TimeoutHook) getTimeoutConfig() *CommandTimeoutConfig {
	if h.timeoutConfig != nil {
		return h.timeoutConfig
	}
	return globalTimeoutConfig
}

// Wait blocks until all asynchronous metrics recordings have completed
func (h *TimeoutHook) Wait() {
	h.wg.Wait()
}
