package httpheaders

// Common HTTP header names (avoid repeating string literals at Set/Add/Del call sites).
const (
	Authorization = "Authorization"
	ContentType   = "Content-Type"
	UserAgent     = "User-Agent"
	XAPIKey       = "X-API-Key" //nolint:gosec
	XEventType    = "X-Event-Type"
	XSource       = "X-Source"
	XTimestamp    = "X-Timestamp"
)
