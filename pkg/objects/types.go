package objects

import (
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/nildecode"
)

// Objects remain as map[string]any for flexibility, but complex nested structures
// use typed Go structs for type safety and better developer experience.
// This "croptop" approach: lightweight and flexible, structured where it matters.

// StatusHistoryEntry represents a status change in the status history
type StatusHistoryEntry struct {
	Status    string    `yaml:"status" json:"status"`
	Timestamp time.Time `yaml:"timestamp" json:"timestamp"`
	Reason    string    `yaml:"reason,omitempty" json:"reason,omitempty"`
	ChangedBy string    `yaml:"changed_by,omitempty" json:"changed_by,omitempty"`
}

// ChangeLogEntry represents a change in the change log
type ChangeLogEntry struct {
	Timestamp time.Time      `yaml:"timestamp" json:"timestamp"`
	ChangedBy string         `yaml:"changed_by" json:"changed_by"`
	Changes   map[string]any `yaml:"changes" json:"changes"`
	Reason    string         `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// GetStatusHistory extracts and parses status_history from an object map
// Returns typed StatusHistoryEntry slice for type-safe access
func GetStatusHistory(obj map[string]any) ([]StatusHistoryEntry, error) {
	historyRaw, ok := obj[FieldKeyStatusHistory]
	if !ok {
		return []StatusHistoryEntry{}, nil
	}
	historyRaw, ok = nildecode.DecodeNonNilPayload[any](historyRaw)
	if !ok {
		return []StatusHistoryEntry{}, nil
	}

	// Handle both []any (from YAML) and []StatusHistoryEntry
	switch v := historyRaw.(type) {
	case []StatusHistoryEntry:
		return v, nil
	case []any:
		history := make([]StatusHistoryEntry, 0, len(v))
		for _, entryRaw := range v {
			// Handle already-typed structs
			if entry, ok := entryRaw.(StatusHistoryEntry); ok {
				history = append(history, entry)
				continue
			}
			// Handle map[string]any (from YAML parsing)
			entryMap, ok := entryRaw.(map[string]any)
			if !ok {
				return nil, errfmt.Errorf("status_history entry is not a map or StatusHistoryEntry: %T", entryRaw)
			}
			entry, err := parseStatusHistoryEntry(entryMap)
			if err != nil {
				return nil, errfmt.Newf("failed to parse status_history entry").Wrap(err)
			}
			history = append(history, entry)
		}
		return history, nil
	default:
		return nil, errfmt.Errorf("status_history has unexpected type: %T", v)
	}
}

// SetStatusHistory sets status_history in an object map from typed slice
func SetStatusHistory(obj map[string]any, history []StatusHistoryEntry) {
	if len(history) == 0 {
		delete(obj, "status_history")
		return
	}
	// Convert to []any for YAML compatibility
	historyRaw := make([]any, len(history))
	for i, entry := range history {
		historyRaw[i] = entry
	}
	obj[FieldKeyStatusHistory] = historyRaw
}

// parseStatusHistoryEntry parses a single status history entry from a map
func parseStatusHistoryEntry(entryMap map[string]any) (StatusHistoryEntry, error) {
	entry := StatusHistoryEntry{}

	if status, ok := entryMap[FieldKeyStatus].(string); ok {
		entry.Status = status
	}

	if timestampRaw, ok := entryMap["timestamp"]; ok {
		switch v := timestampRaw.(type) {
		case time.Time:
			entry.Timestamp = v
		case string:
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return entry, errfmt.Newf("invalid timestamp format").Wrap(err)
			}
			entry.Timestamp = t
		default:
			return entry, errfmt.Errorf("timestamp has unexpected type: %T", v)
		}
	}

	if reason, ok := entryMap[FieldKeyReason].(string); ok {
		entry.Reason = reason
	}

	if changedBy, ok := entryMap["changed_by"].(string); ok {
		entry.ChangedBy = changedBy
	}

	return entry, nil
}

// GetChangeLog extracts and parses change_log from an object map
// Returns typed ChangeLogEntry slice for type-safe access
func GetChangeLog(obj map[string]any) ([]ChangeLogEntry, error) {
	changeLogRaw, ok := obj[FieldKeyChangeLog]
	if !ok {
		return []ChangeLogEntry{}, nil
	}
	changeLogRaw, ok = nildecode.DecodeNonNilPayload[any](changeLogRaw)
	if !ok {
		return []ChangeLogEntry{}, nil
	}

	switch v := changeLogRaw.(type) {
	case []ChangeLogEntry:
		return v, nil
	case []any:
		changeLog := make([]ChangeLogEntry, 0, len(v))
		for _, entryRaw := range v {
			// Handle already-typed structs
			if entry, ok := entryRaw.(ChangeLogEntry); ok {
				changeLog = append(changeLog, entry)
				continue
			}
			// Handle map[string]any (from YAML parsing)
			entryMap, ok := entryRaw.(map[string]any)
			if !ok {
				return nil, errfmt.Errorf("change_log entry is not a map or ChangeLogEntry: %T", entryRaw)
			}
			entry, err := parseChangeLogEntry(entryMap)
			if err != nil {
				return nil, errfmt.Newf("failed to parse change_log entry").Wrap(err)
			}
			changeLog = append(changeLog, entry)
		}
		return changeLog, nil
	default:
		return nil, errfmt.Errorf("change_log has unexpected type: %T", v)
	}
}

// SetChangeLog sets change_log in an object map from typed slice
func SetChangeLog(obj map[string]any, changeLog []ChangeLogEntry) {
	if len(changeLog) == 0 {
		delete(obj, "change_log")
		return
	}
	// Convert to []any for YAML compatibility
	changeLogRaw := make([]any, len(changeLog))
	for i, entry := range changeLog {
		changeLogRaw[i] = entry
	}
	obj[FieldKeyChangeLog] = changeLogRaw
}

// parseChangeLogEntry parses a single change log entry from a map
func parseChangeLogEntry(entryMap map[string]any) (ChangeLogEntry, error) {
	entry := ChangeLogEntry{}

	if timestampRaw, ok := entryMap["timestamp"]; ok {
		switch v := timestampRaw.(type) {
		case time.Time:
			entry.Timestamp = v
		case string:
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return entry, errfmt.Newf("invalid timestamp format").Wrap(err)
			}
			entry.Timestamp = t
		default:
			return entry, errfmt.Errorf("timestamp has unexpected type: %T", v)
		}
	}

	if changedBy, ok := entryMap["changed_by"].(string); ok {
		entry.ChangedBy = changedBy
	}

	switch v := entryMap["changes"].(type) {
	case map[string]any:
		entry.Changes = v
	case map[any]any:
		// Handle YAML map[any]any
		entry.Changes = make(map[string]any)
		for k, v := range v {
			if key, ok := k.(string); ok {
				entry.Changes[key] = v
			}
		}
	}

	if reason, ok := entryMap[FieldKeyReason].(string); ok {
		entry.Reason = reason
	}

	return entry, nil
}

// AddStatusHistoryEntry adds a new entry to status_history in an object map
func AddStatusHistoryEntry(obj map[string]any, status, reason, changedBy string) error {
	history, err := GetStatusHistory(obj)
	if err != nil {
		return errfmt.Newf("failed to get status history").Wrap(err)
	}

	entry := StatusHistoryEntry{
		Status:    status,
		Timestamp: time.Now(),
		Reason:    reason,
		ChangedBy: changedBy,
	}

	history = append(history, entry)
	SetStatusHistory(obj, history)
	return nil
}

// AddChangeLogEntry adds a new entry to change_log in an object map
func AddChangeLogEntry(obj, changes map[string]any, reason, changedBy string) {
	changeLog, err := GetChangeLog(obj)
	if err != nil {
		// If parsing fails, start fresh
		changeLog = []ChangeLogEntry{}
	}

	entry := ChangeLogEntry{
		Timestamp: time.Now(),
		ChangedBy: changedBy,
		Changes:   changes,
		Reason:    reason,
	}

	changeLog = append(changeLog, entry)
	SetChangeLog(obj, changeLog)
}
