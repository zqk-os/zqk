package storage

// fieldUnsetMarker is a sentinel value for Update()'s updates map: the key is removed
// from the persisted object instead of being set. Used by CLI --unset-field.
// Do not serialize this value to YAML/JSON.
type fieldUnsetMarker struct{}

// FieldUnset marks a field for removal during FileObjectStorage/GraphObjectStorage Update merge.
var FieldUnset any = fieldUnsetMarker{}

// IsFieldUnset reports whether v is the FieldUnset sentinel.
func IsFieldUnset(v any) bool {
	_, ok := v.(fieldUnsetMarker)
	return ok
}

// UnsetFieldKeys returns the keys in updates whose value is FieldUnset.
// Graph UpdateNode SET-merges properties, so these keys must be REMOVEd
// after the in-memory delete or they linger on the node.
// TRACK: BLI-REDACTED — persisted active_order on in_progress is invalid.
func UnsetFieldKeys(updates map[string]any) []string {
	if len(updates) == 0 {
		return nil
	}
	var keys []string
	for k, v := range updates {
		if IsFieldUnset(v) {
			keys = append(keys, k)
		}
	}
	return keys
}
