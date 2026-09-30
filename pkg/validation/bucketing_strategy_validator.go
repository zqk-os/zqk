package validation

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/when"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ValidateBucketingStrategy validates a bucketing strategy spec
// This includes validating that applies_to kinds are valid registered object kinds
// Returns ValidationError objects that can be used with the standard validation system
//
//nolint:gocyclo // Function orchestrates multiple validation checks; complexity reduced via helper functions
func ValidateBucketingStrategy(strategy map[string]any) []ValidationError {
	var errors []ValidationError

	// Validate strategy_type - early return if missing
	strategyType, err := validateStrategyType(strategy)
	if err != nil {
		errors = append(errors, *err)
		return errors // Can't continue without strategy_type
	}

	// Validate other fields
	errors = append(errors, validateStrategyField(strategy)...)
	errors = append(errors, validateChronologicalFormat(strategy, strategyType)...)
	errors = append(errors, validateAppliesTo(strategy)...)
	errors = append(errors, validateEnabled(strategy)...)

	// Validate archive_strategy if present
	if archiveStrategy, ok := strategy[objects.FieldKeyArchiveStrategy].(map[string]any); ok {
		errors = append(errors, validateArchiveStrategy(archiveStrategy)...)
	}

	// Note: Uniqueness constraint (one strategy per kind+version) is validated
	// during BucketStrategyLoader.Initialize(), not here, because it requires
	// checking against all loaded strategies and resolving schema versions.

	return errors
}

// validateStrategyType validates strategy_type field
func validateStrategyType(strategy map[string]any) (string, *ValidationError) {
	strategyType, ok := strategy[objects.FieldKeyStrategyType].(string)
	if !ok {
		return "", &ValidationError{
			Field:   "strategy_type",
			Message: "strategy_type is required and must be a string",
		}
	}

	validTypes := map[string]bool{
		"chronological": true,
		"state":         true,
		"size":          true,
		"composite":     true,
		"first_letter":  true,
	}
	if !validTypes[strategyType] {
		return strategyType, &ValidationError{
			Field:   "strategy_type",
			Message: fmt.Sprintf("strategy_type must be one of: chronological, state, size, composite, first_letter (got: %s)", strategyType),
		}
	}

	return strategyType, nil
}

// validateStrategyField validates field field
func validateStrategyField(strategy map[string]any) []ValidationError {
	field, ok := strategy[objects.FieldKeyField].(string)
	if !ok || field == emptyValue {
		return []ValidationError{{
			Field:   "field",
			Message: "field is required and must be a non-empty string",
		}}
	}
	return nil
}

// validateChronologicalFormat validates format for chronological strategies
func validateChronologicalFormat(strategy map[string]any, strategyType string) []ValidationError {
	if strategyType != "chronological" {
		return nil
	}

	format, ok := strategy[objects.FieldKeyFormat].(string)
	if !ok || format == emptyValue {
		return []ValidationError{{
			Field:   "format",
			Message: "format is required for chronological strategies",
		}}
	}

	return nil
}

// validateAppliesTo validates applies_to field
func validateAppliesTo(strategy map[string]any) []ValidationError {
	var errors []ValidationError

	appliesTo, ok := strategy[objects.FieldKeyAppliesTo].([]any)
	if !ok {
		if strategy[objects.FieldKeyAppliesTo] != nil {
			errors = append(errors, ValidationError{
				Field:   "applies_to",
				Message: "applies_to must be an array",
			})
		}
		return errors
	}

	// Get the kind mapper to validate object kinds
	kindMapper := objects.GetGlobalKindMapper()
	if err := kindMapper.Initialize(); err != nil {
		// If initialization fails, log but continue (validation shouldn't fail due to init issues)
		// We'll still try to validate using GetDirectoryFromKind
	}

	for i, kindVal := range appliesTo {
		kind, ok := kindVal.(string)
		if !ok {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("applies_to[%d]", i),
				Message: "applies_to items must be strings",
			})
			continue
		}

		// Validate that the kind is a registered object kind
		dir := kindMapper.GetDirectoryFromKind(kind)
		if dir == emptyValue {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("applies_to[%d]", i),
				Message: fmt.Sprintf("object kind '%s' is not a registered object kind", kind),
			})
		}
	}

	return errors
}

// validateEnabled validates enabled field
func validateEnabled(strategy map[string]any) []ValidationError {
	var errors []ValidationError

	enabled, ok := strategy[objects.FieldKeyEnabled].(bool)
	if !ok {
		// enabled might be missing, which is OK (defaults to false)
		// But if present, it must be a boolean
		if _, exists := strategy[objects.FieldKeyEnabled]; exists {
			errors = append(errors, ValidationError{
				Field:   "enabled",
				Message: "enabled must be a boolean",
			})
		}
		return errors
	}

	// If enabled is true, check applies_to is not empty
	if enabled {
		appliesTo, _ := strategy[objects.FieldKeyAppliesTo].([]any)
		if len(appliesTo) == 0 {
			errors = append(errors, ValidationError{
				Field:   "enabled",
				Message: "enabled strategy must have at least one object kind in applies_to",
			})
		}
	}

	return errors
}

// validateArchiveStrategy validates archive strategy configuration
//
//nolint:gocyclo // Function handles single-tier and multi-tier validation; complexity reduced via helper functions
func validateArchiveStrategy(archiveStrategy map[string]any) []ValidationError {
	var errors []ValidationError

	enabled, ok := archiveStrategy[objects.FieldKeyEnabled].(bool)
	if !ok || !enabled {
		return errors // Only validate if enabled
	}

	tierProgression, hasTierProgression := archiveStrategy["tier_progression"].([]any)

	if !hasTierProgression || len(tierProgression) == 0 {
		// Simple single-tier archival - requires archive_after
		errors = append(errors, validateSingleTierArchive(archiveStrategy)...)
	} else {
		// Multi-tier progression - validate each tier
		errors = append(errors, validateMultiTierArchive(tierProgression)...)
	}

	return errors
}

// validateSingleTierArchive validates single-tier archive strategy
func validateSingleTierArchive(archiveStrategy map[string]any) []ValidationError {
	var errors []ValidationError

	archiveAfter, ok := archiveStrategy["archive_after"].(string)
	if !ok || archiveAfter == emptyValue {
		errors = append(errors, ValidationError{
			Field:   "archive_strategy.archive_after",
			Message: "archive_after is required when archive_strategy.enabled is true and tier_progression is not specified",
		})
	} else if !isValidDuration(archiveAfter) {
		errors = append(errors, ValidationError{
			Field:   "archive_strategy.archive_after",
			Message: fmt.Sprintf("archive_after must be a valid duration (e.g., '720h', '30d'), got: %s", archiveAfter),
		})
	}

	return errors
}

// validateMultiTierArchive validates multi-tier archive progression
func validateMultiTierArchive(tierProgression []any) []ValidationError {
	var errors []ValidationError

	for i, tierVal := range tierProgression {
		tier, ok := tierVal.(map[string]any)
		if !ok {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("archive_strategy.tier_progression[%d]", i),
				Message: "tier_progression items must be objects",
			})
			continue
		}

		errors = append(errors, validateTier(tier, i)...)
	}

	return errors
}

// validateTier validates a single tier in tier_progression
func validateTier(tier map[string]any, index int) []ValidationError {
	var errors []ValidationError

	// Validate tier enum
	if err := validateTierName(tier, index); err != nil {
		errors = append(errors, *err)
	}

	// Validate duration
	if err := validateTierDuration(tier, index); err != nil {
		errors = append(errors, *err)
	}

	return errors
}

// validateTierName validates tier name (warm, cold, iced)
func validateTierName(tier map[string]any, index int) *ValidationError {
	tierName, ok := tier[objects.FieldKeyTier].(string)
	if !ok || tierName == emptyValue {
		return &ValidationError{
			Field:   fmt.Sprintf("archive_strategy.tier_progression[%d].tier", index),
			Message: "tier is required and must be one of: warm, cold, iced",
		}
	}

	validTiers := map[string]bool{"warm": true, "cold": true, "iced": true}
	if !validTiers[tierName] {
		return &ValidationError{
			Field:   fmt.Sprintf("archive_strategy.tier_progression[%d].tier", index),
			Message: fmt.Sprintf("tier must be one of: warm, cold, iced (got: %s)", tierName),
		}
	}

	return nil
}

// validateTierDuration validates tier duration
func validateTierDuration(tier map[string]any, index int) *ValidationError {
	duration, ok := tier["duration"].(string)
	if !ok || duration == emptyValue {
		return &ValidationError{
			Field:   fmt.Sprintf("archive_strategy.tier_progression[%d].duration", index),
			Message: "duration is required (use '0' for final tier to keep indefinitely)",
		}
	}

	if duration != "0" && !isValidDuration(duration) {
		return &ValidationError{
			Field:   fmt.Sprintf("archive_strategy.tier_progression[%d].duration", index),
			Message: fmt.Sprintf("duration must be a valid duration (e.g., '720h', '30d') or '0' for indefinite, got: %s", duration),
		}
	}

	return nil
}

// isValidDuration checks if a string is a valid duration format
// Supports formats like "720h", "30d", "60m", "3600s"
func isValidDuration(duration string) bool {
	if duration == "0" {
		return true
	}
	// Simple regex-like check: one or more digits followed by h, m, s, or d
	if len(duration) < 2 {
		return false
	}
	// Check if it matches pattern: digits followed by unit
	hasDigits := false
	for i, r := range duration {
		var retVal bool
		var shouldReturn bool
		when.When(func() bool { return r >= '0' && r <= '9' }).Then(func() {
			hasDigits = true
		}).OrElseWhen(func() bool { return r == 'h' || r == 'm' || r == 's' || r == 'd' }).Then(func() {
			retVal = hasDigits && i == len(duration)-1
			shouldReturn = true
		}).OrElse(func() {
			retVal = false
			shouldReturn = true
		}).Run()

		if shouldReturn {
			return retVal
		}
	}
	return false
}
