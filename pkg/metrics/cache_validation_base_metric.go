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

const maxCacheValidationMetricsJSONLen = 256 * 1024

func cacheValidationMetricTitle(asyncKinds, kindRows int, totalVal int64) string {
	s := fmt.Sprintf("CAS cache: %d async kinds, %d kind rows, %d validations", asyncKinds, kindRows, totalVal)
	if len(s) < 5 {
		s = "CAS validation cache metrics"
	}
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}

// BuildCacheValidationBaseMetricInstance builds a base_metric from encoded CAS cache metrics JSON.
func BuildCacheValidationBaseMetricInstance(metricID string, jsonPayload []byte, meta storage.CacheMetricsEncodeMeta) (map[string]any, error) {
	start, end, err := FormatMetricWindow(metricID, meta.WindowStart, meta.WindowEnd)
	if err != nil {
		return nil, err
	}

	tags := []string{
		MetricTagSystem,
		"storage",
		"cas_validation_cache",
	}

	oc := ComputeEffectiveObjectCount(meta.TotalValidationsSum, meta.AsyncStrategyKinds+meta.KindValidationMetrics)

	builder := instance_builders.NewForKind(objects.KindBaseMetric, objects.DefaultSchemaVersion)
	builder.SetID(metricID).
		SetField(objects.FieldKeyTitle, cacheValidationMetricTitle(meta.AsyncStrategyKinds, meta.KindValidationMetrics, meta.TotalValidationsSum)).
		SetField(objects.FieldKeyMetricType, MetricTypeSystem).
		SetField(objects.FieldKeySource, MetricSourceCASValidationCache).
		SetField(objects.FieldKeyMetricTypeSpecific, "cas_validation_cache_snapshot").
		SetField(objects.FieldKeyTags, tags).
		SetField(objects.FieldKeyCollectionCount, 1).
		SetField(objects.FieldKeyObjectCount, oc).
		SetField(objects.FieldKeyObjectKind, "cas_validation_cache").
		SetField(objects.FieldKeyFirstSeen, start).
		SetField(objects.FieldKeyLastSeen, end).
		SetField(objects.FieldKeyWindowStart, start).
		SetField(objects.FieldKeyWindowEnd, end)

	payload := string(jsonPayload)
	if len(payload) > maxCacheValidationMetricsJSONLen {
		suffix := "\n...(truncated)"
		payload = payload[:maxCacheValidationMetricsJSONLen-len(suffix)] + suffix
	}
	builder.SetField(objects.FieldKeyContext, map[string]any{objects.FieldKeyCasValidationCacheMetricsJSON: payload})

	return builder.Build()
}

// PersistCacheValidationMetricsJSONAsync persists pre-encoded CAS cache metrics to base_metric.
func PersistCacheValidationMetricsJSONAsync(sp storage.ObjectStorageProvider, jsonBytes []byte, logger logging.Logger, meta storage.CacheMetricsEncodeMeta) {
	if sp == nil {
		return
	}
	if !metricsrecording.Enabled() {
		return
	}

	goroutinelabels.NewGoroutine("cas_cache_validation_base_metric", "persist CAS cache metrics to base_metric").
		StartSimple(func() {
			metricID := fmt.Sprintf("BAS-%d", time.Now().UnixNano())
			inst, err := BuildCacheValidationBaseMetricInstance(metricID, jsonBytes, meta)
			if err != nil {
				if logger != nil {
					logging.Fluent(logger).Warn("Failed to build CAS cache validation base_metric").
						WithError(err).
						Log()
				}
				return
			}

			PersistBaseMetricInstance(sp, inst, metricID, "CAS cache validation base_metric", logger)
		})
}
