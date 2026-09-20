package logging

// String values match pkg/objects field_keys.go. Duplicated here because pkg/objects
// imports pkg/logging (e.g. field_discovery); logging must not import objects.
const (
	objectFieldKeyContext     = "context"
	objectFieldKeyEventType   = "event_type"
	objectFieldKeyField       = "field"
	objectFieldKeyID          = "id"
	objectFieldKeyKind        = "kind"
	objectFieldKeyObjectRef   = "object_ref"
	objectFieldKeyOperationID = "operation_id"
	objectFieldKeyType        = "type"
)
