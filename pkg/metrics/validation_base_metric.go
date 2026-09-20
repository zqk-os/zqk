package metrics

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// maxValidationMetricsJSONLen caps stored JSON on base_metric rows (stream-friendly).
const (
	maxValidationMetricsJSONLen = 256 * 1024
	emptyValue                  = ""
)

// asyncValidationMetricTitle builds the canonical title for async validation metrics.
func asyncValidationMetricTitle(validated, failed, total int) string {
	s := fmt.Sprintf("Async check: %d validated, %d failed of %d", validated, failed, total)
	if len(s) < 5 {
		s = "Async check metrics"
	}
	return s
}

func truncateTag(s string, max int) string {
	if max <= 0 {
		return emptyValue
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// BuildAsyncValidationBaseMetricInstance builds a base_metric instance from finalized validation metrics.
// jsonPayload should be the result of ValidationMetrics.JSONSnapshot() (possibly truncated by the caller).
func BuildAsyncValidationBaseMetricInstance(
	vm *validation.ValidationMetrics,
	operationID string,
	metricID string,
	jsonPayload []byte,
) (map[string]any, error) {
	if vm == nil {
		return nil, errfmt.Errorf("validation metrics is nil")
	}
	if metricID == emptyValue {
		return nil, errfmt.Errorf("metric id is required")
	}
	start := zqktime.FormatRFC3339UTC(vm.StartTime)
	end := vm.EndTime
	if end.IsZero() {
		end = time.Now().UTC()
	} else {
		end = end.UTC()
	}
	endStr := end.Format(time.RFC3339)

	tags := []string{
		MetricTagSystem,
		"validation",
		"async_check",
	}
	if operationID != emptyValue {
		tags = append(tags, truncateTag(operationID, 50))
	}

	builder := bldr_instance_v1.NewBaseMetricInstanceBuilder(objects.DefaultSchemaVersion)
	builder.ID(metricID).
		Title(asyncValidationMetricTitle(vm.ValidatedObjects, vm.FailedObjects, vm.TotalObjects)).
		MetricType(MetricTypeSystem).
		Source(MetricSourceAsyncValidation).
		MetricTypeSpecific("validation_async_check").
		Tags(tags).
		CollectionCount(1).
		ObjectCount(vm.TotalObjects).
		ObjectKind("async_validation").
		FirstSeen(start).
		LastSeen(endStr).
		WindowStart(start).
		WindowEnd(endStr)

	payload := string(jsonPayload)
	if len(payload) > maxValidationMetricsJSONLen {
		suffix := "\n...(truncated)"
		payload = payload[:maxValidationMetricsJSONLen-len(suffix)] + suffix
	}
	contextMap := map[string]any{objects.FieldKeyValidationMetricsJSON: payload}
	if operationID != emptyValue {
		contextMap[objects.FieldKeyOperationID] = operationID
	}
	builder.SetContext(contextMap)

	return builder.Build()
}

// PersistAsyncValidationMetricsToStorageAsync persists finalized validation metrics to a base_metric (best-effort, non-blocking).
func PersistAsyncValidationMetricsToStorageAsync(
	sp storage.ObjectStorageProvider,
	vm *validation.ValidationMetrics,
	operationID string,
	logger logging.Logger,
) {
	if sp == nil || vm == nil {
		return
	}
	if !metricsrecording.Enabled() {
		return
	}

	goroutinelabels.NewGoroutine("async_validation_base_metric", "persist validation metrics to base_metric").
		StartSimple(func() {
			jsonBytes, err := vm.JSONSnapshot()
			if err != nil {
				if logger != nil {
					logging.Fluent(logger).Warn("Failed to snapshot validation metrics for base_metric").
						WithError(err).
						Log()
				}
				return
			}

			metricID := fmt.Sprintf("BAS-%d", time.Now().UnixNano())
			instance, buildErr := BuildAsyncValidationBaseMetricInstance(vm, operationID, metricID, jsonBytes)
			if buildErr != nil {
				if logger != nil {
					logging.Fluent(logger).Warn("Failed to build async validation base_metric").
						WithError(buildErr).
						Log()
				}
				return
			}

			ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 60*time.Second)
			defer cancel()
			secCtx := pkgctx.NewSystemSecurityContext()
			if createErr := sp.Create(ctx, secCtx, instance); createErr != nil {
				if logger != nil {
					logging.Fluent(logger).Warn("Failed to persist async validation base_metric").
						MetricID(metricID).
						WithError(createErr).
						Log()
				}
			}
		})
}
