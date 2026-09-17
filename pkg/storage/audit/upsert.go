package audit

import "github.com/lanceman/zqk/pkg/objects"

// MetricUpdateFields copies metric for an upsert, dropping id/created_at/created_by
// and stamping updated_at / updated_by.
func MetricUpdateFields(metric map[string]any, updatedAt, updatedBy string) map[string]any {
	updates := make(map[string]any, len(metric)+2)
	for k, v := range metric {
		if k == objects.FieldKeyID || k == objects.FieldKeyCreatedAt || k == objects.FieldKeyCreatedBy {
			continue
		}
		updates[k] = v
	}
	if updatedAt != "" {
		updates[objects.FieldKeyUpdatedAt] = updatedAt
	}
	if updatedBy != "" {
		updates[objects.FieldKeyUpdatedBy] = updatedBy
	}
	return updates
}
