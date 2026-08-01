package metrics

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/storage"
)

// FlushCacheValidationMetricsToStorage encodes CAS async validation cache + per-kind validation metrics
// and persists asynchronously as a base_metric (metricsrecording-gated, best-effort).
func FlushCacheValidationMetricsToStorage(sp storage.ObjectStorageProvider, logger logging.Logger) {
	if sp == nil {
		return
	}
	if !metricsrecording.Enabled() {
		return
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	data, meta, err := storage.EncodeCacheMetricsForPersistence()
	if err != nil {
		logging.Fluent(logger).Warn("Failed to encode CAS cache metrics for persistence").
			WithError(err).
			Log()
		return
	}
	PersistCacheValidationMetricsJSONAsync(sp, data, logger, meta)
}
