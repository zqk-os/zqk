package overseer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrDaemonUnhealthy indicates a daemon failed active health probes.
	ErrDaemonUnhealthy = errors.New("daemon health check failed")
	// ErrDaemonHeartbeatTimeout indicates a daemon missed its heartbeat threshold.
	ErrDaemonHeartbeatTimeout = errors.New("daemon heartbeat timed out")
)

// HealthState represents runtime health status.
type HealthState string

const (
	// HealthStateHealthy indicates normal operations.
	HealthStateHealthy HealthState = "healthy"
	// HealthStateDegraded indicates transient errors within retry threshold.
	HealthStateDegraded HealthState = "degraded"
	// HealthStateUnhealthy indicates threshold exceeded requiring restart.
	HealthStateUnhealthy HealthState = "unhealthy"
)

// DaemonHealthCheck defines an active probe for a daemon.
type DaemonHealthCheck func(ctx context.Context) error

// HealthMonitor tracks heartbeats and executes active probes for registered daemons.
type HealthMonitor struct {
	mu           sync.RWMutex
	probes       map[string]DaemonHealthCheck
	heartbeats   map[string]time.Time
	consecErrors map[string]int
	maxErrors    int
	timeout      time.Duration
}

// NewHealthMonitor instantiates a health monitor.
func NewHealthMonitor(maxErrors int, timeout time.Duration) *HealthMonitor {
	if maxErrors <= 0 {
		maxErrors = 3
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HealthMonitor{
		probes:       make(map[string]DaemonHealthCheck),
		heartbeats:   make(map[string]time.Time),
		consecErrors: make(map[string]int),
		maxErrors:    maxErrors,
		timeout:      timeout,
	}
}

// RegisterProbe registers an active health check for a daemon.
func (hm *HealthMonitor) RegisterProbe(name string, probe DaemonHealthCheck) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.probes[name] = probe
}

// RecordHeartbeat logs a successful heartbeat ping from a daemon.
func (hm *HealthMonitor) RecordHeartbeat(name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.heartbeats[name] = time.Now()
	hm.consecErrors[name] = 0
}

// CheckHealth evaluates active probe and heartbeat recency for a daemon.
func (hm *HealthMonitor) CheckHealth(ctx context.Context, name string) (HealthState, error) {
	hm.mu.RLock()
	probe, hasProbe := hm.probes[name]
	lastHB, hasHB := hm.heartbeats[name]
	hm.mu.RUnlock()

	// If heartbeat exists, verify freshness
	if hasHB && time.Since(lastHB) > hm.timeout {
		hm.recordFailure(name)
		return HealthStateUnhealthy, fmt.Errorf("%w: %s", ErrDaemonHeartbeatTimeout, name)
	}

	// If probe exists, run it
	if hasProbe && probe != nil {
		probeCtx, cancel := context.WithTimeout(ctx, hm.timeout)
		defer cancel()
		if err := probe(probeCtx); err != nil {
			fails := hm.recordFailure(name)
			if fails >= hm.maxErrors {
				return HealthStateUnhealthy, fmt.Errorf("%w: %v", ErrDaemonUnhealthy, err)
			}
			return HealthStateDegraded, fmt.Errorf("probe warning: %w", err)
		}
	}

	hm.mu.Lock()
	hm.consecErrors[name] = 0
	hm.mu.Unlock()
	return HealthStateHealthy, nil
}

func (hm *HealthMonitor) recordFailure(name string) int {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.consecErrors[name]++
	return hm.consecErrors[name]
}

// FailureCount returns consecutive failure count for a daemon.
func (hm *HealthMonitor) FailureCount(name string) int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.consecErrors[name]
}
