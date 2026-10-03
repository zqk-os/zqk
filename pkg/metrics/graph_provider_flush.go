package metrics

import (
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// FlushGraphProviderMetricsToStorage encodes the global graph provider metrics snapshot
// and persists it asynchronously as a base_metric (metricsrecording-gated, best-effort).
func FlushGraphProviderMetricsToStorage(sp storage.ObjectStorageProvider, logger logging.Logger) {
	FlushMetricsIfReady(sp, logger, func(l logging.Logger) {
		c := provider.GetGlobalGraphProviderMetricsCollector()
		snap := c.GetMetrics()
		data, err := provider.EncodeMetricsSnapshotForPersistence(snap)
		if err != nil {
			logging.Fluent(l).Warn("Failed to encode graph provider metrics for persistence").
				WithError(err).
				Log()
			return
		}
		meta := GraphProviderPersistMeta{
			WindowStart:     snap.Timestamp,
			WindowEnd:       time.Now().UTC(),
			OperationKinds:  len(snap.Operations),
			QueryKinds:      len(snap.Queries),
			TotalOperations: snap.TotalOperations,
		}
		PersistGraphProviderMetricsJSONAsync(sp, data, l, meta)
	})
}
