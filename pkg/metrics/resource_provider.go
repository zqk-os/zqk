package metrics

import (
	"runtime"

	"github.com/lanceman/zqk/pkg/concurrency"
)

// ResourceAvailability represents the current available system resources.
type ResourceAvailability struct {
	TotalGoroutines  int
	UsedGoroutines   int
	AvailableCompute int // Available goroutine budget
	MemoryAllocBytes uint64
}

// GetResourceAvailability samples current system resources.
func GetResourceAvailability() *ResourceAvailability {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	used := runtime.NumGoroutine()
	ceiling := concurrency.DefaultGoroutineCeiling

	available := ceiling - used
	if available < 0 {
		available = 0
	}

	return &ResourceAvailability{
		TotalGoroutines:  ceiling,
		UsedGoroutines:   used,
		AvailableCompute: available,
		MemoryAllocBytes: m.Alloc,
	}
}
