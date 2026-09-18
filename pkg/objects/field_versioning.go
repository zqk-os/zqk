package objects

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/zqktime"
)

// FieldLifecycleState represents the lifecycle state of a field
type FieldLifecycleState string

const (
	FieldStateActive     FieldLifecycleState = "active"     // Field is active and in use
	FieldStateCreated    FieldLifecycleState = "created"    // Field was just created
	FieldStateModified   FieldLifecycleState = "modified"   // Field was modified (breaking change)
	FieldStateDeprecated FieldLifecycleState = "deprecated" // Field is deprecated (will be removed)
	FieldStateArchived   FieldLifecycleState = "archived"   // Field is archived (removed but kept for history)
	FieldStateDeleted    FieldLifecycleState = "deleted"    // Field is deleted
)

// FieldVersionInfo tracks versioning and lifecycle information for a field
type FieldVersionInfo struct {
	State           FieldLifecycleState `yaml:"state"`            // Current lifecycle state
	CreatedAt       string              `yaml:"created_at"`       // When field was created (RFC3339)
	CreatedBy       string              `yaml:"created_by"`       // Who created the field
	ModifiedAt      string              `yaml:"modified_at"`      // When field was last modified (RFC3339)
	ModifiedBy      string              `yaml:"modified_by"`      // Who last modified the field
	DeprecatedAt    string              `yaml:"deprecated_at"`    // When field was deprecated (RFC3339)
	DeprecatedBy    string              `yaml:"deprecated_by"`    // Who deprecated the field
	ArchivedAt      string              `yaml:"archived_at"`      // When field was archived (RFC3339)
	ArchivedBy      string              `yaml:"archived_by"`      // Who archived the field
	DeletedAt       string              `yaml:"deleted_at"`       // When field was deleted (RFC3339)
	DeletedBy       string              `yaml:"deleted_by"`       // Who deleted the field
	Version         string              `yaml:"version"`          // Field version (SemVer, e.g., InitialFieldVersion)
	PreviousVersion string              `yaml:"previous_version"` // Previous version (for modified fields)
	BreakingChange  bool                `yaml:"breaking_change"`  // Whether this change is breaking
	ChangeReason    string              `yaml:"change_reason"`    // Reason for the change
	MigrationNotes  string              `yaml:"migration_notes"`  // Notes for migrating existing data
	ReplacedBy      string              `yaml:"replaced_by"`      // Field that replaces this one (for deprecated/deleted)
}

// FieldChange represents a change to a field
type FieldChange struct {
	FieldName      string          // Name of the field
	ChangeType     FieldChangeType // Type of change
	OldValue       any             // Old value (for modifications)
	NewValue       any             // New value
	BreakingChange bool            // Whether this is a breaking change
	Reason         string          // Reason for the change
	MigrationNotes string          // Migration guidance
	ReplacedBy     string          // Field that replaces this (for deletions)
}

// FieldChangeType represents the type of field change
type FieldChangeType string

const (
	FieldChangeCreated    FieldChangeType = "created"    // New field added
	FieldChangeModified   FieldChangeType = "modified"   // Field modified
	FieldChangeDeprecated FieldChangeType = "deprecated" // Field deprecated
	FieldChangeArchived   FieldChangeType = "archived"   // Field archived
	FieldChangeDeleted    FieldChangeType = "deleted"    // Field deleted
)

// fieldVerKey* / fieldSpecKey* are YAML map keys for nested field definition / version_info maps.
const (
	fieldSpecKeyEnum           = "enum"
	fieldSpecKeyPattern        = "pattern"
	fieldSpecKeyRequired       = "required"
	fieldSpecKeyValidation     = "validation"
	fieldVerKeyBreakingChange  = "breaking_change"
	fieldVerKeyChangeReason    = "change_reason"
	fieldVerKeyDeletedAt       = "deleted_at"
	fieldVerKeyDeletedBy       = "deleted_by"
	fieldVerKeyDeprecatedAt    = "deprecated_at"
	fieldVerKeyDeprecatedBy    = "deprecated_by"
	fieldVerKeyMigrationNotes  = "migration_notes"
	fieldVerKeyModifiedAt      = "modified_at"
	fieldVerKeyModifiedBy      = "modified_by"
	fieldVerKeyPreviousVersion = "previous_version"
	fieldVerKeyReplacedBy      = "replaced_by"
	fieldVerKeyState           = "state"
	fieldVerKeyVersionInfo     = "version_info"
)

// GetFieldVersionInfo extracts version info from a field definition
func GetFieldVersionInfo(fieldDef map[string]any) *FieldVersionInfo {
	versionInfo := &FieldVersionInfo{
		State: FieldStateActive, // Default state
	}

	if versionMap, ok := fieldDef[fieldVerKeyVersionInfo].(map[string]any); ok {
		if state, ok := versionMap[fieldVerKeyState].(string); ok {
			versionInfo.State = FieldLifecycleState(state)
		}
		if createdAt, ok := versionMap[FieldKeyCreatedAt].(string); ok {
			versionInfo.CreatedAt = createdAt
		}
		if createdBy, ok := versionMap[FieldKeyCreatedBy].(string); ok {
			versionInfo.CreatedBy = createdBy
		}
		if modifiedAt, ok := versionMap[fieldVerKeyModifiedAt].(string); ok {
			versionInfo.ModifiedAt = modifiedAt
		}
		if modifiedBy, ok := versionMap[fieldVerKeyModifiedBy].(string); ok {
			versionInfo.ModifiedBy = modifiedBy
		}
		if deprecatedAt, ok := versionMap[fieldVerKeyDeprecatedAt].(string); ok {
			versionInfo.DeprecatedAt = deprecatedAt
		}
		if deprecatedBy, ok := versionMap[fieldVerKeyDeprecatedBy].(string); ok {
			versionInfo.DeprecatedBy = deprecatedBy
		}
		if archivedAt, ok := versionMap[FieldKeyArchivedAt].(string); ok {
			versionInfo.ArchivedAt = archivedAt
		}
		if archivedBy, ok := versionMap[FieldKeyArchivedBy].(string); ok {
			versionInfo.ArchivedBy = archivedBy
		}
		if deletedAt, ok := versionMap[fieldVerKeyDeletedAt].(string); ok {
			versionInfo.DeletedAt = deletedAt
		}
		if deletedBy, ok := versionMap[fieldVerKeyDeletedBy].(string); ok {
			versionInfo.DeletedBy = deletedBy
		}
		if version, ok := versionMap[FieldKeyVersion].(string); ok {
			versionInfo.Version = version
		}
		if prevVersion, ok := versionMap[fieldVerKeyPreviousVersion].(string); ok {
			versionInfo.PreviousVersion = prevVersion
		}
		if breaking, ok := versionMap[fieldVerKeyBreakingChange].(bool); ok {
			versionInfo.BreakingChange = breaking
		}
		if reason, ok := versionMap[fieldVerKeyChangeReason].(string); ok {
			versionInfo.ChangeReason = reason
		}
		if notes, ok := versionMap[fieldVerKeyMigrationNotes].(string); ok {
			versionInfo.MigrationNotes = notes
		}
		if replacedBy, ok := versionMap[fieldVerKeyReplacedBy].(string); ok {
			versionInfo.ReplacedBy = replacedBy
		}
	}

	return versionInfo
}

// SetFieldVersionInfo sets version info in a field definition
func SetFieldVersionInfo(fieldDef map[string]any, versionInfo *FieldVersionInfo) {
	if fieldDef == nil {
		return
	}

	versionMap := make(map[string]any)
	versionMap[fieldVerKeyState] = string(versionInfo.State)
	if versionInfo.CreatedAt != emptyValue {
		versionMap[FieldKeyCreatedAt] = versionInfo.CreatedAt
	}
	if versionInfo.CreatedBy != emptyValue {
		versionMap[FieldKeyCreatedBy] = versionInfo.CreatedBy
	}
	if versionInfo.ModifiedAt != emptyValue {
		versionMap[fieldVerKeyModifiedAt] = versionInfo.ModifiedAt
	}
	if versionInfo.ModifiedBy != emptyValue {
		versionMap[fieldVerKeyModifiedBy] = versionInfo.ModifiedBy
	}
	if versionInfo.DeprecatedAt != emptyValue {
		versionMap[fieldVerKeyDeprecatedAt] = versionInfo.DeprecatedAt
	}
	if versionInfo.DeprecatedBy != emptyValue {
		versionMap[fieldVerKeyDeprecatedBy] = versionInfo.DeprecatedBy
	}
	if versionInfo.ArchivedAt != emptyValue {
		versionMap[FieldKeyArchivedAt] = versionInfo.ArchivedAt
	}
	if versionInfo.ArchivedBy != emptyValue {
		versionMap[FieldKeyArchivedBy] = versionInfo.ArchivedBy
	}
	if versionInfo.DeletedAt != emptyValue {
		versionMap[fieldVerKeyDeletedAt] = versionInfo.DeletedAt
	}
	if versionInfo.DeletedBy != emptyValue {
		versionMap[fieldVerKeyDeletedBy] = versionInfo.DeletedBy
	}
	if versionInfo.Version != emptyValue {
		versionMap[FieldKeyVersion] = versionInfo.Version
	}
	if versionInfo.PreviousVersion != emptyValue {
		versionMap[fieldVerKeyPreviousVersion] = versionInfo.PreviousVersion
	}
	if versionInfo.BreakingChange {
		versionMap[fieldVerKeyBreakingChange] = true
	}
	if versionInfo.ChangeReason != emptyValue {
		versionMap[fieldVerKeyChangeReason] = versionInfo.ChangeReason
	}
	if versionInfo.MigrationNotes != emptyValue {
		versionMap[fieldVerKeyMigrationNotes] = versionInfo.MigrationNotes
	}
	if versionInfo.ReplacedBy != emptyValue {
		versionMap[fieldVerKeyReplacedBy] = versionInfo.ReplacedBy
	}

	fieldDef[fieldVerKeyVersionInfo] = versionMap
}

// MarkFieldCreated marks a field as newly created
func MarkFieldCreated(fieldDef map[string]any, createdBy string) {
	versionInfo := GetFieldVersionInfo(fieldDef)
	if versionInfo.CreatedAt == emptyValue {
		versionInfo.State = FieldStateCreated
		versionInfo.CreatedAt = zqktime.NowRFC3339UTC()
		versionInfo.CreatedBy = createdBy
		versionInfo.Version = InitialFieldVersion
		SetFieldVersionInfo(fieldDef, versionInfo)
	}
}

// MarkFieldModified marks a field as modified (with breaking change detection)
func MarkFieldModified(fieldDef map[string]any, modifiedBy string, breakingChange bool, reason, migrationNotes string) {
	versionInfo := GetFieldVersionInfo(fieldDef)

	// Increment version
	if versionInfo.Version == emptyValue {
		versionInfo.Version = InitialFieldVersion
	}
	versionInfo.PreviousVersion = versionInfo.Version

	// Bump version based on breaking change
	if breakingChange {
		// Major version bump for breaking changes
		versionInfo.Version = incrementMajorVersion(versionInfo.Version)
	} else {
		// Minor version bump for non-breaking changes
		versionInfo.Version = incrementMinorVersion(versionInfo.Version)
	}

	versionInfo.State = FieldStateModified
	versionInfo.ModifiedAt = zqktime.NowRFC3339UTC()
	versionInfo.ModifiedBy = modifiedBy
	versionInfo.BreakingChange = breakingChange
	versionInfo.ChangeReason = reason
	versionInfo.MigrationNotes = migrationNotes

	SetFieldVersionInfo(fieldDef, versionInfo)
}

// MarkFieldDeprecated marks a field as deprecated
func MarkFieldDeprecated(fieldDef map[string]any, deprecatedBy, replacedBy, reason string) {
	versionInfo := GetFieldVersionInfo(fieldDef)
	versionInfo.State = FieldStateDeprecated
	versionInfo.DeprecatedAt = zqktime.NowRFC3339UTC()
	versionInfo.DeprecatedBy = deprecatedBy
	versionInfo.ReplacedBy = replacedBy
	versionInfo.ChangeReason = reason
	SetFieldVersionInfo(fieldDef, versionInfo)
}

// MarkFieldArchived marks a field as archived
func MarkFieldArchived(fieldDef map[string]any, archivedBy, reason string) {
	versionInfo := GetFieldVersionInfo(fieldDef)
	versionInfo.State = FieldStateArchived
	versionInfo.ArchivedAt = zqktime.NowRFC3339UTC()
	versionInfo.ArchivedBy = archivedBy
	versionInfo.ChangeReason = reason
	SetFieldVersionInfo(fieldDef, versionInfo)
}

// MarkFieldDeleted marks a field as deleted
func MarkFieldDeleted(fieldDef map[string]any, deletedBy, replacedBy, reason string) {
	versionInfo := GetFieldVersionInfo(fieldDef)
	versionInfo.State = FieldStateDeleted
	versionInfo.DeletedAt = zqktime.NowRFC3339UTC()
	versionInfo.DeletedBy = deletedBy
	versionInfo.ReplacedBy = replacedBy
	versionInfo.ChangeReason = reason
	SetFieldVersionInfo(fieldDef, versionInfo)
}

// DetectBreakingChange compares two field definitions and detects if the change is breaking
func DetectBreakingChange(oldFieldDef, newFieldDef map[string]any) (bool, []string) {
	var breakingReasons []string

	// Check type changes
	oldType, oldHasType := oldFieldDef[FieldKeyType].(string)
	newType, newHasType := newFieldDef[FieldKeyType].(string)
	if oldHasType && newHasType && oldType != newType {
		breakingReasons = append(breakingReasons, fmt.Sprintf("type changed from %s to %s", oldType, newType))
	}

	// Check required changes (adding required is breaking)
	oldValidation, oldHasValidation := oldFieldDef[fieldSpecKeyValidation].(map[string]any)
	newValidation, newHasValidation := newFieldDef[fieldSpecKeyValidation].(map[string]any)
	if oldHasValidation && newHasValidation {
		oldRequired, oldHasRequired := oldValidation[fieldSpecKeyRequired].(bool)
		newRequired, newHasRequired := newValidation[fieldSpecKeyRequired].(bool)
		if oldHasRequired && newHasRequired {
			if !oldRequired && newRequired {
				breakingReasons = append(breakingReasons, "field changed from optional to required")
			}
		} else if !oldHasRequired && newHasRequired && newRequired {
			breakingReasons = append(breakingReasons, "field changed from optional to required")
		}
	}

	// Check enum changes (removing values is breaking)
	oldEnum, oldHasEnum := oldValidation[fieldSpecKeyEnum].([]any)
	newEnum, newHasEnum := newValidation[fieldSpecKeyEnum].([]any)
	if oldHasEnum && newHasEnum {
		oldEnumSet := make(map[string]bool)
		for _, v := range oldEnum {
			if str, ok := v.(string); ok {
				oldEnumSet[str] = true
			}
		}
		for _, v := range newEnum {
			if str, ok := v.(string); ok {
				if !oldEnumSet[str] {
					// New value added - not breaking
				}
			}
		}
		// Check for removed values
		newEnumSet := make(map[string]bool)
		for _, v := range newEnum {
			if str, ok := v.(string); ok {
				newEnumSet[str] = true
			}
		}
		for _, v := range oldEnum {
			if str, ok := v.(string); ok {
				if !newEnumSet[str] {
					breakingReasons = append(breakingReasons, fmt.Sprintf("enum value %s removed", str))
				}
			}
		}
	}

	// Check pattern changes (stricter pattern is breaking)
	oldPattern, oldHasPattern := oldValidation[fieldSpecKeyPattern].(string)
	newPattern, newHasPattern := newValidation[fieldSpecKeyPattern].(string)
	if oldHasPattern && newHasPattern && oldPattern != newPattern {
		breakingReasons = append(breakingReasons, "validation pattern changed")
	}

	// Check trait changes (removing traits can be breaking)
	oldTraits := ExtractFieldTraits(oldFieldDef)
	newTraits := ExtractFieldTraits(newFieldDef)
	oldTraitSet := make(map[string]bool)
	for _, t := range oldTraits {
		oldTraitSet[t] = true
	}
	for _, t := range newTraits {
		if !oldTraitSet[t] {
			// New trait added - not breaking
		}
	}
	// Check for removed traits
	newTraitSet := make(map[string]bool)
	for _, t := range newTraits {
		newTraitSet[t] = true
	}
	for _, t := range oldTraits {
		if !newTraitSet[t] {
			// Trait removed - potentially breaking depending on trait
			if t == "readable" || t == "writable" {
				breakingReasons = append(breakingReasons, fmt.Sprintf("trait %s removed", t))
			}
		}
	}

	return len(breakingReasons) > 0, breakingReasons
}

// incrementMajorVersion increments the major version (e.g., InitialFieldVersion -> DefaultSchemaVersion)
func incrementMajorVersion(version string) string {
	// Simple version increment - assumes SemVer format
	// For production, use a proper SemVer library
	if version == emptyValue {
		return InitialFieldVersion
	}
	// Simple increment: find first number and increment
	// This is a simplified version - proper SemVer parsing would be better
	parts := []rune(version)
	for i, r := range parts {
		if r >= '0' && r <= '9' {
			// Found first digit - increment
			if r == '9' {
				// Handle carry
				parts[i] = '0'
				// Find next digit or add one
				if i+1 < len(parts) && parts[i+1] == '.' {
					// Look for next number
					for j := i + 2; j < len(parts); j++ {
						if parts[j] >= '0' && parts[j] <= '9' {
							if parts[j] == '9' {
								parts[j] = '0'
							} else {
								parts[j]++
								break
							}
						}
					}
				}
			} else {
				parts[i]++
			}
			break
		}
	}
	return string(parts)
}

// incrementMinorVersion increments the minor version (e.g., InitialFieldVersion -> 1.1.0)
func incrementMinorVersion(version string) string {
	// Simple version increment - assumes SemVer format
	// For production, use a proper SemVer library
	if version == emptyValue {
		return InitialFieldVersion
	}
	// Find second number (after first dot) and increment
	parts := []rune(version)
	dotCount := 0
	for i, r := range parts {
		if r == '.' {
			dotCount++
			if dotCount == 1 {
				// Found first dot, increment next number
				if i+1 < len(parts) && parts[i+1] >= '0' && parts[i+1] <= '9' {
					if parts[i+1] == '9' {
						parts[i+1] = '0'
					} else {
						parts[i+1]++
					}
				}
				break
			}
		}
	}
	return string(parts)
}
