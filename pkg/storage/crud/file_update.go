package crud

import (
	"github.com/lanceman/zqk/pkg/objects"
	"reflect"
)

func ComputeUpdatesMap(previous, newObj map[string]any) map[string]any {
	if newObj == nil {
		return nil
	}
	updates := make(map[string]any)
	for k, newVal := range newObj {
		prevVal, had := previous[k]
		if !had || !reflect.DeepEqual(prevVal, newVal) {
			updates[k] = newVal
		}
	}
	return updates
}

// effectiveUpdateFieldsForClassification returns the subset of requested fields that effectively
// changed after merge/normalization, plus metadata runtime fields auto-managed by ensureObjectMetadata.
func EffectiveUpdateFieldsForClassification(previous, current, requested map[string]any) map[string]any {
	updates := make(map[string]any)
	if current == nil {
		return updates
	}
	for k := range requested {
		if k == "expected_updated_at" {
			continue
		}
		prevVal, prevOK := previous[k]
		currVal, currOK := current[k]
		if prevOK != currOK || !reflect.DeepEqual(prevVal, currVal) {
			if currOK {
				updates[k] = currVal
			} else {
				updates[k] = nil
			}
		}
	}
	for _, k := range []string{objects.FieldKeyUpdatedAt, objects.FieldKeyUpdatedBy} {
		prevVal, prevOK := previous[k]
		currVal, currOK := current[k]
		if prevOK != currOK || !reflect.DeepEqual(prevVal, currVal) {
			if currOK {
				updates[k] = currVal
			} else {
				updates[k] = nil
			}
		}
	}
	return updates
}

func ClassifyUpdateMutation(idUpdated, runtimeDeltaOnly bool) string {
	if idUpdated {
		return "id_change"
	}
	if runtimeDeltaOnly {
		return "runtime_delta"
	}
	return "structural"
}

func ShockwaveRouterIsArmed(status string) bool {
	switch status {
	case objects.ObjectStatusActive, objects.ObjectStatusApproved, objects.ObjectStatusInProgress:
		return true
	default:
		return false
	}
}
