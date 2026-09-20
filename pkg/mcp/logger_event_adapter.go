package mcp

// LoggerEventAdapter adapts the mcp.Logger interface to emit events
// This allows log messages to be automatically converted to events
// that can be subscribed to by MCP clients
type LoggerEventAdapter struct {
	logger       Logger
	eventEmitter *EventEmitter
	enabled      bool
}

// NewLoggerEventAdapter creates a logger adapter that emits events
func NewLoggerEventAdapter(logger Logger, eventEmitter *EventEmitter) *LoggerEventAdapter {
	return &LoggerEventAdapter{
		logger:       logger,
		eventEmitter: eventEmitter,
		enabled:      true,
	}
}

// Debug logs a debug message and emits an event
func (la *LoggerEventAdapter) Debug(msg string, fields ...LogField) {
	// Call underlying logger
	if la.logger != nil {
		la.logger.Debug(msg, fields...)
	}

	// Emit event if enabled
	if la.enabled && la.eventEmitter != nil {
		event := &Event{
			Type:     EventTypeLogDebug,
			Message:  msg,
			Severity: "debug",
			Fields:   convertLogFieldsToMap(fields),
		}
		la.eventEmitter.Emit(event)
	}
}

// SetEnabled enables or disables event emission
func (la *LoggerEventAdapter) SetEnabled(enabled bool) {
	la.enabled = enabled
}

// convertLogFieldsToMap converts LogField slice to map
func convertLogFieldsToMap(fields []LogField) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		result[field.Key] = field.Value
	}
	return result
}
