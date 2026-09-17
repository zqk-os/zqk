package metrics

import (
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/storage"
)

// FlushGraphProviderMetricsToStorage encodes the global graph provider metrics snapshot
// and persists it asynchronously as a base_metric (metricsrecording-gated, best-effort).
func FlushGraphProviderMetricsToStorage(sp storage.ObjectStorageProvider, logger logging.Logger) {
	if sp == nil {
		return
	}
	if !metricsrecording.Enabled() {
		return
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	c := provider.GetGlobalGraphProviderMetricsCollector()
	snap := c.GetMetrics()
	data, err := provider.EncodeMetricsSnapshotForPersistence(snap)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to encode graph provider metrics for persistence").
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
	PersistGraphProviderMetricsJSONAsync(sp, data, logger, meta)
}
