package metrics

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// FlushCacheValidationMetricsToStorage encodes CAS async validation cache + per-kind validation metrics
// and persists asynchronously as a base_metric (metricsrecording-gated, best-effort).
func FlushCacheValidationMetricsToStorage(sp storage.ObjectStorageProvider, logger logging.Logger) {
	FlushMetricsIfReady(sp, logger, func(l logging.Logger) {
		data, meta, err := storage.EncodeCacheMetricsForPersistence()
		if err != nil {
			logging.Fluent(l).Warn("Failed to encode CAS cache metrics for persistence").
				WithError(err).
				Log()
			return
		}
		PersistCacheValidationMetricsJSONAsync(sp, data, l, meta)
	})
}
