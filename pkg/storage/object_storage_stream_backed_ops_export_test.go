package storage

import "time"

// CreateCleanupTestCommandMetricForTest exposes [createCleanupTestCommandMetric] for external tests.
func CreateCleanupTestCommandMetricForTest(id string, createdAt time.Time) map[string]any {
	return createCleanupTestCommandMetric(id, createdAt)
}
