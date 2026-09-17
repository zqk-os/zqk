package audit

// NormalizeEventType ensures event_type is in allowed; if not, it coerces to
// fallback and records the original value in metadata.
func NormalizeEventType(eventType string, metadata map[string]any, allowed map[string]struct{}, fallback string) (string, map[string]any) {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if _, ok := allowed[eventType]; ok {
		return eventType, metadata
	}
	metadata[MetadataKeyOriginalEventType] = eventType
	if fallback != "" {
		return fallback, metadata
	}
	return eventType, metadata
}
