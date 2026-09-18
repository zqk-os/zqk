package logging

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// ProgressFormatter formats progress, status, and stream events for output
// Different formatters support different output styles (text, JSON, compact)
type ProgressFormatter interface {
	// FormatProgress formats a progress update
	FormatProgress(operationID string, progress int, message string, fields map[string]any) ([]byte, error)

	// FormatStatus formats a status change
	FormatStatus(operationID string, oldStatus, newStatus string, fields map[string]any) ([]byte, error)

	// FormatStreamEvent formats a streaming event
	FormatStreamEvent(eventType string, data any, fields map[string]any) ([]byte, error)

	// SupportsOverwrite returns true if this formatter supports line overwriting (for progress bars)
	// Formatters that support overwrite can use \r to update the same line
	// Formatters that don't support overwrite output each update as a separate line/event
	SupportsOverwrite() bool
}

// TextProgressFormatter formats progress as human-readable text with progress bars
type TextProgressFormatter struct{}

// NewTextProgressFormatter creates a new text progress formatter
func NewTextProgressFormatter() ProgressFormatter {
	return &TextProgressFormatter{}
}

func (f *TextProgressFormatter) SupportsOverwrite() bool {
	return true // Text formatter supports \r for line overwriting
}

func (f *TextProgressFormatter) FormatProgress(operationID string, progress int, message string, fields map[string]any) ([]byte, error) {
	// Generate progress bar
	bar := f.generateProgressBar(progress)

	// Build output line
	parts := []string{bar}

	// Add object ID if present
	if objectID, ok := fields["object_id"].(string); ok && objectID != emptyValue {
		parts = append(parts, objectID)
	}

	// Add message and progress percentage
	parts = append(parts, message, fmt.Sprintf("%d%%", progress))

	line := strings.Join(parts, " ")
	return []byte(line), nil
}

func (f *TextProgressFormatter) FormatStatus(operationID, oldStatus, newStatus string, fields map[string]any) ([]byte, error) {
	parts := []string{}

	// Add object ID if present
	if objectID, ok := fields["object_id"].(string); ok && objectID != emptyValue {
		parts = append(parts, fmt.Sprintf("[%s]", objectID))
	}

	// Add status transition
	parts = append(parts, fmt.Sprintf("Status: %s → %s", oldStatus, newStatus))

	line := strings.Join(parts, " ")
	return []byte(line + "\n"), nil
}

func (f *TextProgressFormatter) FormatStreamEvent(eventType string, data any, fields map[string]any) ([]byte, error) {
	// For text formatter, stream events are formatted as simple messages
	parts := []string{fmt.Sprintf("[%s]", eventType)}

	// Add object ID if present
	if objectID, ok := fields["object_id"].(string); ok && objectID != emptyValue {
		parts = append(parts, objectID)
	}

	// Add data summary
	parts = append(parts, fmt.Sprintf("%v", data))

	line := strings.Join(parts, " ")
	return []byte(line + "\n"), nil
}

func (f *TextProgressFormatter) generateProgressBar(progress int) string {
	const width = 20
	filled := (progress * width) / 100
	bar := make([]byte, width)
	for i := 0; i < width; i++ {
		if i < filled {
			bar[i] = '='
		} else {
			bar[i] = ' '
		}
	}
	return fmt.Sprintf("[%s]", string(bar))
}

// Reserved JSON keys for JSONProgressFormatter merges (extra fields must not clobber these).
var (
	jsonReservedProgress = map[string]struct{}{
		objectFieldKeyType: {}, objectFieldKeyOperationID: {}, "progress": {}, "message": {}, "timestamp": {},
	}
	jsonReservedStatus = map[string]struct{}{
		objectFieldKeyType: {}, objectFieldKeyOperationID: {}, "old_status": {}, "new_status": {}, "timestamp": {},
	}
	jsonReservedStream = map[string]struct{}{
		objectFieldKeyType: {}, objectFieldKeyEventType: {}, "data": {}, "timestamp": {},
	}
)

// mergeJSONFields copies entries from src into dst for keys not in reserved.
func mergeJSONFields(dst map[string]any, src map[string]any, reserved map[string]struct{}) {
	if src == nil {
		return
	}
	for k, v := range src {
		if _, skip := reserved[k]; !skip {
			dst[k] = v
		}
	}
}

// JSONProgressFormatter formats progress as structured JSON events
// Each update is a separate JSON object (no line overwriting)
type JSONProgressFormatter struct{}

// NewJSONProgressFormatter creates a new JSON progress formatter
func NewJSONProgressFormatter() ProgressFormatter {
	return &JSONProgressFormatter{}
}

func (f *JSONProgressFormatter) SupportsOverwrite() bool {
	return false // JSON formatter outputs each update as separate JSON object
}

func (f *JSONProgressFormatter) FormatProgress(operationID string, progress int, message string, fields map[string]any) ([]byte, error) {
	data := map[string]any{
		objectFieldKeyType:        "progress",
		objectFieldKeyOperationID: operationID,
		"progress":                progress,
		"message":                 message,
		"timestamp":               zqktime.NowRFC3339UTC(),
	}

	mergeJSONFields(data, fields, jsonReservedProgress)

	result, err := json.Marshal(data)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal progress JSON").Wrap(err)
	}

	return append(result, '\n'), nil
}

func (f *JSONProgressFormatter) FormatStatus(operationID, oldStatus, newStatus string, fields map[string]any) ([]byte, error) {
	data := map[string]any{
		objectFieldKeyType:        "status",
		objectFieldKeyOperationID: operationID,
		"old_status":              oldStatus,
		"new_status":              newStatus,
		"timestamp":               zqktime.NowRFC3339UTC(),
	}

	mergeJSONFields(data, fields, jsonReservedStatus)

	result, err := json.Marshal(data)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal status JSON").Wrap(err)
	}

	return append(result, '\n'), nil
}

func (f *JSONProgressFormatter) FormatStreamEvent(eventType string, data any, fields map[string]any) ([]byte, error) {
	eventData := map[string]any{
		objectFieldKeyType:      "stream_event",
		objectFieldKeyEventType: eventType,
		"data":                  data,
		"timestamp":             zqktime.NowRFC3339UTC(),
	}

	mergeJSONFields(eventData, fields, jsonReservedStream)

	result, err := json.Marshal(eventData)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal stream event JSON").Wrap(err)
	}

	return append(result, '\n'), nil
}

// CompactProgressFormatter formats progress compactly for debug output
type CompactProgressFormatter struct{}

// NewCompactProgressFormatter creates a new compact progress formatter
func NewCompactProgressFormatter() ProgressFormatter {
	return &CompactProgressFormatter{}
}

func (f *CompactProgressFormatter) SupportsOverwrite() bool {
	return true // Compact formatter supports \r for line overwriting
}

func (f *CompactProgressFormatter) FormatProgress(operationID string, progress int, message string, fields map[string]any) ([]byte, error) {
	// Compact format: [op-123] 50% Processing
	parts := []string{fmt.Sprintf("[%s]", operationID)}
	parts = append(parts, fmt.Sprintf("%d%%", progress), message)

	line := strings.Join(parts, " ")
	return []byte(line), nil
}

func (f *CompactProgressFormatter) FormatStatus(operationID, oldStatus, newStatus string, fields map[string]any) ([]byte, error) {
	// Compact format: [op-123] pending→in_progress
	line := fmt.Sprintf("[%s] %s→%s", operationID, oldStatus, newStatus)
	return []byte(line + "\n"), nil
}

func (f *CompactProgressFormatter) FormatStreamEvent(eventType string, data any, fields map[string]any) ([]byte, error) {
	// Compact format: [event_type] data
	line := fmt.Sprintf("[%s] %v", eventType, data)
	return []byte(line + "\n"), nil
}
