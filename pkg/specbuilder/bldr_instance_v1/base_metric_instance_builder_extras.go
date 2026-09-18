package bldr_instance_v1

import "github.com/zqk-os/zqk/pkg/objects"

// ValidationMetricsJSON sets the validation_metrics_json field.
func (b *BaseMetricInstanceBuilder) ValidationMetricsJSON(value string) *BaseMetricInstanceBuilder {
	b.SetField(objects.FieldKeyValidationMetricsJSON, value)
	return b
}

// SetValidationMetricsJSON is a compatibility alias for ValidationMetricsJSON.
func (b *BaseMetricInstanceBuilder) SetValidationMetricsJSON(value string) *BaseMetricInstanceBuilder {
	return b.ValidationMetricsJSON(value)
}

// OperationID sets the operation_id field.
func (b *BaseMetricInstanceBuilder) OperationID(value string) *BaseMetricInstanceBuilder {
	b.SetField(objects.FieldKeyOperationID, value)
	return b
}

// SetOperationID is a compatibility alias for OperationID.
func (b *BaseMetricInstanceBuilder) SetOperationID(value string) *BaseMetricInstanceBuilder {
	return b.OperationID(value)
}

// CasValidationCacheMetricsJSON sets the cas_validation_cache_metrics_json field.
func (b *BaseMetricInstanceBuilder) CasValidationCacheMetricsJSON(value string) *BaseMetricInstanceBuilder {
	b.SetField(objects.FieldKeyCasValidationCacheMetricsJSON, value)
	return b
}

// SetCasValidationCacheMetricsJSON is a compatibility alias for CasValidationCacheMetricsJSON.
func (b *BaseMetricInstanceBuilder) SetCasValidationCacheMetricsJSON(value string) *BaseMetricInstanceBuilder {
	return b.CasValidationCacheMetricsJSON(value)
}
