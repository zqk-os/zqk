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
