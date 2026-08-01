package storage

import (
	"sync"
	"sync/atomic"
)

var (
	updateMutationMetricsMu         sync.RWMutex
	updateMutationMetrics           = make(map[string]map[string]int64) // kind -> mutation_class -> count
	globalMutationsRecordedTotal    atomic.Int64
	globalMutationKindsTrackedTotal atomic.Int64
)

// GetUpdateMutationTotalStats returns lifetime counters for total mutations recorded and unique kinds tracked.
func GetUpdateMutationTotalStats() (recorded, kinds int64) {
	return globalMutationsRecordedTotal.Load(), globalMutationKindsTrackedTotal.Load()
}

// RecordUpdateMutationClass increments the in-memory counter for a kind/class pair.
// This is process-local telemetry for quick diagnostics (not persisted).
func RecordUpdateMutationClass(kind, mutationClass string) {
	if kind == emptyValue || mutationClass == emptyValue {
		return
	}
	globalMutationsRecordedTotal.Add(1)
	updateMutationMetricsMu.Lock()
	defer updateMutationMetricsMu.Unlock()
	byClass, ok := updateMutationMetrics[kind]
	if !ok {
		byClass = make(map[string]int64)
		updateMutationMetrics[kind] = byClass
		globalMutationKindsTrackedTotal.Add(1)
	}
	byClass[mutationClass]++
}

// GetUpdateMutationClassSnapshot returns a deep-copy snapshot of process-local mutation counters.
func GetUpdateMutationClassSnapshot() map[string]map[string]int64 {
	updateMutationMetricsMu.RLock()
	defer updateMutationMetricsMu.RUnlock()
	out := make(map[string]map[string]int64, len(updateMutationMetrics))
	for kind, byClass := range updateMutationMetrics {
		inner := make(map[string]int64, len(byClass))
		for class, count := range byClass {
			inner[class] = count
		}
		out[kind] = inner
	}
	return out
}

// ResetUpdateMutationClassMetrics clears process-local mutation counters.
func ResetUpdateMutationClassMetrics() {
	updateMutationMetricsMu.Lock()
	defer updateMutationMetricsMu.Unlock()
	updateMutationMetrics = make(map[string]map[string]int64)
}
