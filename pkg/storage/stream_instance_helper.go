package storage

import (
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// AppendInstanceToStream appends a spec-backed instance map to the stream for its kind
// when stream storage is enabled for that kind. It uses AppendToStream under the hood
// so all stream kinds share the same concurrency, filename, and delta-field behavior.
//
// Expects instance kind and id (objects.FieldKeyKind / FieldKeyID) to be non-empty strings. If kind is not
// stream-enabled or required fields are missing, this is a no-op (best-effort).
func AppendInstanceToStream(projectRoot string, instance map[string]any) error {
	if projectRoot == emptyValue || len(instance) == 0 {
		return nil
	}
	rawKind, ok := instance[objects.FieldKeyKind].(string)
	if !ok || rawKind == emptyValue {
		return nil
	}
	if !StreamStorageEnabledForKind(rawKind) {
		return nil
	}
	rawID, ok := instance[objects.FieldKeyID].(string)
	if !ok || rawID == emptyValue {
		return nil
	}

	// Best-effort created_at extraction; fall back to now when missing or invalid.
	createdAt := time.Now().UTC()
	if rawCreatedAt := objects.GetString(instance, objects.FieldKeyCreatedAt); rawCreatedAt != emptyValue {
		if parsed, err := time.Parse(time.RFC3339, rawCreatedAt); err == nil {
			createdAt = parsed
		}
	}

	if _, _, err := AppendToStream(projectRoot, rawKind, rawID, instance, createdAt); err != nil {
		return errfmt.Newf(ConstStreamStreamInstanceAppend).Wrap(err)
	}
	return nil
}
