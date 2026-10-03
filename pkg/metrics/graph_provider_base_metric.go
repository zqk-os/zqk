package metrics

import (
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
)

const maxGraphProviderMetricsJSONLen = 256 * 1024

// GraphProviderPersistMeta carries summary fields for base_metric rows (avoids importing pkg/graph/provider from here).
type GraphProviderPersistMeta struct {
	WindowStart     time.Time
	WindowEnd       time.Time
	OperationKinds  int
	QueryKinds      int
	TotalOperations int64
}

func graphProviderMetricTitle(opCount, queryCount int, totalOps int64) string {
	s := fmt.Sprintf("Graph: %d ops, %d queries, %d total", opCount, queryCount, totalOps)
	if len(s) < 5 {
		s = "Graph provider metrics"
	}
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}

// BuildGraphProviderMetricsBaseMetricInstance builds a base_metric from encoded graph provider JSON.
func BuildGraphProviderMetricsBaseMetricInstance(metricID string, jsonPayload []byte, meta GraphProviderPersistMeta) (map[string]any, error) {
	start, end, err := FormatMetricWindow(metricID, meta.WindowStart, meta.WindowEnd)
	if err != nil {
		return nil, err
	}

	tags := []string{
		MetricTagSystem,
		"graph",
		"provider",
	}

	oc := ComputeEffectiveObjectCount(meta.TotalOperations, meta.OperationKinds+meta.QueryKinds)

	builder := instance_builders.NewForKind(objects.KindBaseMetric, objects.DefaultSchemaVersion)
	builder.SetID(metricID).
		SetField(objects.FieldKeyTitle, graphProviderMetricTitle(meta.OperationKinds, meta.QueryKinds, meta.TotalOperations)).
		SetField(objects.FieldKeyMetricType, MetricTypeSystem).
		SetField(objects.FieldKeySource, MetricSourceGraphProvider).
		SetField(objects.FieldKeyMetricTypeSpecific, "graph_provider_snapshot").
		SetField(objects.FieldKeyTags, tags).
		SetField(objects.FieldKeyCollectionCount, 1).
		SetField(objects.FieldKeyObjectCount, oc).
		SetField(objects.FieldKeyObjectKind, "graph_provider").
		SetField(objects.FieldKeyFirstSeen, start).
		SetField(objects.FieldKeyLastSeen, end).
		SetField(objects.FieldKeyWindowStart, start).
		SetField(objects.FieldKeyWindowEnd, end)

	payload := string(jsonPayload)
	if len(payload) > maxGraphProviderMetricsJSONLen {
		suffix := "\n...(truncated)"
		payload = payload[:maxGraphProviderMetricsJSONLen-len(suffix)] + suffix
	}
	builder.SetField(objects.FieldKeyGraphProviderMetricsJSON, payload)

	return builder.Build()
}

// PersistGraphProviderMetricsJSONAsync persists pre-encoded graph provider metrics JSON to base_metric.
func PersistGraphProviderMetricsJSONAsync(sp storage.ObjectStorageProvider, jsonBytes []byte, logger logging.Logger, meta GraphProviderPersistMeta) {
	if sp == nil {
		return
	}
	if !metricsrecording.Enabled() {
		return
	}

	goroutinelabels.NewGoroutine("graph_provider_base_metric", "persist graph provider metrics to base_metric").
		StartSimple(func() {
			metricID := fmt.Sprintf("BAS-%d", time.Now().UnixNano())
			inst, err := BuildGraphProviderMetricsBaseMetricInstance(metricID, jsonBytes, meta)
			if err != nil {
				if logger != nil {
					logging.Fluent(logger).Warn("Failed to build graph provider base_metric").
						WithError(err).
						Log()
				}
				return
			}

			PersistBaseMetricInstance(sp, inst, metricID, "graph provider base_metric", logger)
		})
}
