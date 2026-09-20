package provider

import (
	"sync"
)

var (
	globalGraphMetricsCollector     *DefaultMetricsCollector
	globalGraphMetricsCollectorOnce sync.Once
)

// GetGlobalGraphProviderMetricsCollector returns a process-wide default metrics collector
// used by MemGraph/Bolt instrumentation and flush-to-storage paths.
func GetGlobalGraphProviderMetricsCollector() *DefaultMetricsCollector {
	globalGraphMetricsCollectorOnce.Do(func() {
		globalGraphMetricsCollector = NewDefaultMetricsCollector()
	})
	return globalGraphMetricsCollector
}
