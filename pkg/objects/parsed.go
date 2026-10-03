package objects

import (
	"errors"
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/nildecode"
)

const (
	fieldChangeLog           = "change_log"
	errTimeValueNil          = "time value is nil"
	errUnableParseTimeFmt    = "unable to parse time string: %s"
	errUnexpectedTimeTypeFmt = "time value has unexpected type: %T"
	timeFormatRFC3339NoNanos = "2006-01-02T15:04:05Z07:00"
	timeFormatSpaceSeparated = "2006-01-02 15:04:05"
)

// ParsedObject represents an object that has been parsed once for optimized access
// Complex nested structures are extracted as typed structs, and list fields
// are converted to typed slices for faster filtering/sorting operations.
// Simple fields (strings, bools) remain in the map since there's little performance gain.
type ParsedObject struct {
	// Raw map with all fields (simple fields accessed directly)
	Raw map[string]any

	// Extracted complex nested structures (removed from Raw to avoid duplication)
	StatusHistory []StatusHistoryEntry
	ChangeLog     []ChangeLogEntry

	// Typed list fields for faster iteration during filtering/sorting
	// These are extracted from Raw and converted to typed slices
	GoalRefs        []string
	MilestoneRefs   []string
	RequirementRefs []string
	WorkstreamRefs  []string
	BacklogItemRefs []string
	CriteriaRefs    []string
	TestCaseRefs    []string
	DocEntryRefs    []string
	Artifacts       []string
	Dependencies    []string
	Stakeholders    []string
	Questions       []string

	// Common simple fields cached for quick access (but also in Raw)
	ID            string
	Kind          string
	NamespaceID   string
	Status        string
	Title         string
	Category      string
	Priority      string
	SchemaVersion string
	TargetKind    string
	TargetID      string
	Operation     string
	Severity      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ParseObject parses an object map once, extracting complex types and converting
// lists to typed slices for optimized filtering/sorting operations.
// The original map is modified to remove extracted fields (to avoid duplication).
func ParseObject(obj map[string]any) (*ParsedObject, error) {
	parsed := &ParsedObject{
		Raw: obj,
	}

	// Extract complex nested structures
	if history, err := GetStatusHistory(obj); err == nil {
		parsed.StatusHistory = history
		// Remove from raw map to avoid duplication
		delete(obj, FieldKeyStatusHistory)
	}

	if changeLog, err := GetChangeLog(obj); err == nil {
		parsed.ChangeLog = changeLog
		// Remove from raw map to avoid duplication
		delete(obj, fieldChangeLog)
	}

	// Extract and convert list fields to typed slices
	parsed.GoalRefs = extractStringList(obj, FieldKeyGoalRefs)
	parsed.MilestoneRefs = extractStringList(obj, FieldKeyMilestoneRefs)
	parsed.RequirementRefs = extractStringList(obj, FieldKeyRequirementRefs)
	parsed.WorkstreamRefs = extractStringList(obj, FieldKeyWorkstreamRefs)
	parsed.BacklogItemRefs = extractStringList(obj, FieldKeyBacklogItemRefs)
	parsed.CriteriaRefs = extractStringList(obj, FieldKeyCriteriaRefs)
	parsed.TestCaseRefs = extractStringList(obj, FieldKeyTestCaseRefs)
	parsed.DocEntryRefs = collectRefIDs(
		extractStringList(obj, FieldKeyDocEntryRefs),
		extractStringList(obj, FieldKeyDocumentRefs),
	)
	parsed.Artifacts = extractStringList(obj, FieldKeyArtifacts)
	parsed.Dependencies = extractStringList(obj, FieldKeyDependencies)
	parsed.Stakeholders = extractStringList(obj, FieldKeyStakeholders)
	parsed.Questions = extractStringList(obj, FieldKeyQuestions)

	// Cache common simple fields for quick access (but keep in Raw too)
	populateCoreIdentityFields(parsed, obj)
	if title, ok := obj[FieldKeyTitle].(string); ok {
		parsed.Title = title
	}
	if category, ok := obj[FieldKeyCategory].(string); ok {
		parsed.Category = category
	}
	if priority, ok := obj[FieldKeyPriority].(string); ok {
		parsed.Priority = priority
	}
	if schemaVersion, ok := obj[FieldKeySchemaVersion].(string); ok {
		parsed.SchemaVersion = schemaVersion
	}
	if targetKind, ok := obj[FieldKeyTargetKind].(string); ok {
		parsed.TargetKind = targetKind
	}
	if targetID, ok := obj[FieldKeyTargetID].(string); ok {
		parsed.TargetID = targetID
	}
	if operation, ok := obj[FieldKeyOperation].(string); ok {
		parsed.Operation = operation
	}
	if severity, ok := obj[FieldKeySeverity].(string); ok {
		parsed.Severity = severity
	}

	// Parse timestamps
	if createdAt, err := parseTime(obj[FieldKeyCreatedAt]); err == nil {
		parsed.CreatedAt = createdAt
	}
	if updatedAt, err := parseTime(obj[FieldKeyUpdatedAt]); err == nil {
		parsed.UpdatedAt = updatedAt
	}

	return parsed, nil
}

// ToMap converts the parsed object back to a map, restoring extracted fields
func (p *ParsedObject) ToMap() map[string]any {
	// Start with a copy of the raw map
	result := make(map[string]any)
	maps.Copy(result, p.Raw)

	// Restore complex nested structures
	if len(p.StatusHistory) > 0 {
		SetStatusHistory(result, p.StatusHistory)
	}
	if len(p.ChangeLog) > 0 {
		SetChangeLog(result, p.ChangeLog)
	}

	// Restore list fields (only if they were extracted)
	if len(p.GoalRefs) > 0 {
		result[FieldKeyGoalRefs] = p.GoalRefs
	}
	if len(p.MilestoneRefs) > 0 {
		result[FieldKeyMilestoneRefs] = p.MilestoneRefs
	}
	if len(p.RequirementRefs) > 0 {
		result[FieldKeyRequirementRefs] = p.RequirementRefs
	}
	if len(p.WorkstreamRefs) > 0 {
		result[FieldKeyWorkstreamRefs] = p.WorkstreamRefs
	}
	if len(p.BacklogItemRefs) > 0 {
		result[FieldKeyBacklogItemRefs] = p.BacklogItemRefs
	}
	if len(p.CriteriaRefs) > 0 {
		result[FieldKeyCriteriaRefs] = p.CriteriaRefs
	}
	if len(p.TestCaseRefs) > 0 {
		result[FieldKeyTestCaseRefs] = p.TestCaseRefs
	}
	if len(p.DocEntryRefs) > 0 {
		result[FieldKeyDocEntryRefs] = p.DocEntryRefs
	}
	if len(p.Artifacts) > 0 {
		result[FieldKeyArtifacts] = p.Artifacts
	}
	if len(p.Dependencies) > 0 {
		result[FieldKeyDependencies] = p.Dependencies
	}
	if len(p.Stakeholders) > 0 {
		result[FieldKeyStakeholders] = p.Stakeholders
	}
	if len(p.Questions) > 0 {
		result[FieldKeyQuestions] = p.Questions
	}

	return result
}

// extractStringList extracts a string list from the map and removes it
// Returns empty slice if field doesn't exist or is not a list
func extractStringList(obj map[string]any, fieldName string) []string {
	raw, ok := obj[fieldName]
	if !ok {
		return []string{}
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return []string{}
	}

	switch v := raw.(type) {
	case []string:
		// Already typed, remove from map and return
		delete(obj, fieldName)
		return v
	case []any:
		// Convert []any to []string
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		// Remove from map since we've extracted it
		delete(obj, fieldName)
		return result
	default:
		// Try to handle []any (which is []any at runtime)
		// Use type assertion to check if it's a slice
		if slice, ok := raw.([]any); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			delete(obj, fieldName)
			return result
		}
		// Not a list, leave in map
		return []string{}
	}
}

// parseTime parses a time value from various formats
func parseTime(v any) (time.Time, error) {
	if v == nil {
		return time.Time{}, errors.New(errTimeValueNil)
	}

	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		// Try RFC3339 first
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed, nil
		}
		// Try other common formats
		formats := []string{
			time.RFC3339Nano,
			timeFormatRFC3339NoNanos,
			timeFormatSpaceSeparated,
		}
		for _, format := range formats {
			if parsed, err := time.Parse(format, t); err == nil {
				return parsed, nil
			}
		}
		return time.Time{}, errfmt.Errorf(errUnableParseTimeFmt, t)
	default:
		return time.Time{}, errfmt.Errorf(errUnexpectedTimeTypeFmt, v)
	}
}

// GetField returns a field value, checking typed fields first, then raw map
// This provides a unified accessor that prefers typed fields when available
func (p *ParsedObject) GetField(name string) (any, bool) {
	if IsKernelObjectRefField(name) {
		if val, ok := kernelRefFieldValue(p.KernelObjectRefIDs(name), name); ok {
			return val, true
		}
	}
	// Check typed fields first (faster access)
	switch name {
	case FieldKeyID:
		if p.ID != emptyValue {
			return p.ID, true
		}
	case FieldKeyKind:
		if p.Kind != emptyValue {
			return p.Kind, true
		}
	case FieldKeyNamespaceID:
		if p.NamespaceID != emptyValue {
			return p.NamespaceID, true
		}
	case FieldKeyStatus:
		if p.Status != emptyValue {
			return p.Status, true
		}
	case FieldKeyTitle:
		if p.Title != emptyValue {
			return p.Title, true
		}
	case FieldKeyCategory:
		if p.Category != emptyValue {
			return p.Category, true
		}
	case FieldKeyPriority:
		if p.Priority != emptyValue {
			return p.Priority, true
		}
	case FieldKeySchemaVersion:
		if p.SchemaVersion != emptyValue {
			return p.SchemaVersion, true
		}
	case FieldKeyCreatedAt:
		if !p.CreatedAt.IsZero() {
			return p.CreatedAt, true
		}
	case FieldKeyUpdatedAt:
		if !p.UpdatedAt.IsZero() {
			return p.UpdatedAt, true
		}
	case FieldKeyGoalRefs:
		if len(p.GoalRefs) > 0 {
			return p.GoalRefs, true
		}
	case FieldKeyMilestoneRefs:
		if len(p.MilestoneRefs) > 0 {
			return p.MilestoneRefs, true
		}
	case FieldKeyRequirementRefs:
		if len(p.RequirementRefs) > 0 {
			return p.RequirementRefs, true
		}
	case FieldKeyWorkstreamRefs:
		if len(p.WorkstreamRefs) > 0 {
			return p.WorkstreamRefs, true
		}
	case FieldKeyBacklogItemRefs:
		if len(p.BacklogItemRefs) > 0 {
			return p.BacklogItemRefs, true
		}
	case FieldKeyCriteriaRefs:
		if len(p.CriteriaRefs) > 0 {
			return p.CriteriaRefs, true
		}
	case FieldKeyTestCaseRefs:
		if len(p.TestCaseRefs) > 0 {
			return p.TestCaseRefs, true
		}
	case FieldKeyDocEntryRefs, FieldKeyDocumentRefs:
		if len(p.DocEntryRefs) > 0 {
			return p.DocEntryRefs, true
		}
	case FieldKeyStatusHistory:
		if len(p.StatusHistory) > 0 {
			return p.StatusHistory, true
		}
	case FieldKeyChangeLog:
		if len(p.ChangeLog) > 0 {
			return p.ChangeLog, true
		}
	case FieldKeyTargetKind:
		if p.TargetKind != emptyValue {
			return p.TargetKind, true
		}
	case FieldKeyTargetID:
		if p.TargetID != emptyValue {
			return p.TargetID, true
		}
	case FieldKeyOperation:
		if p.Operation != emptyValue {
			return p.Operation, true
		}
	case FieldKeySeverity:
		if p.Severity != emptyValue {
			return p.Severity, true
		}
	}

	// Fall back to raw map
	if val, ok := p.Raw[name]; ok {
		return val, true
	}

	return nil, false
}

// ParseObjectMinimal parses only common metadata fields from a map, avoiding expensive
// full-schema extraction and deep list conversion. Use for high-volume high-frequency
// scans like retention cleanup and aggregation.
func populateCoreIdentityFields(parsed *ParsedObject, obj map[string]any) {
	if id, ok := obj[FieldKeyID].(string); ok {
		parsed.ID = id
	}
	if kind, ok := obj[FieldKeyKind].(string); ok {
		parsed.Kind = kind
	}
	if ns, ok := obj[FieldKeyNamespaceID].(string); ok {
		parsed.NamespaceID = ns
	}
	if status, ok := obj[FieldKeyStatus].(string); ok {
		parsed.Status = status
	}
}

// ParseObjectMinimal parses only common metadata fields from a map, avoiding expensive
// full-schema extraction and deep list conversion. Use for high-volume high-frequency
// scans like retention cleanup and aggregation.
func ParseObjectMinimal(obj map[string]any) *ParsedObject {
	parsed := &ParsedObject{
		Raw: obj,
	}

	// Extract only the fields needed for discovery, filtering, and basic lifecycle
	populateCoreIdentityFields(parsed, obj)
	if createdAt, ok := obj[FieldKeyCreatedAt].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			parsed.CreatedAt = t
		}
	}

	return parsed
}
