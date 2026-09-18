package utility

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// identifyPIIFieldsFromSpec identifies PII fields from object spec
func identifyPIIFieldsFromSpec(spec *objects.Spec) map[string]bool {
	piiFields := make(map[string]bool)
	if spec == nil || spec.Fields == nil {
		return piiFields
	}

	for fieldName, fieldDefAny := range spec.Fields {
		if fieldDef, ok := fieldDefAny.(map[string]any); ok {
			if checklist, ok := fieldDef["checklist"].(map[string]any); ok {
				if security, ok := checklist["security"].(string); ok {
					securityLower := strings.ToLower(security)
					if strings.Contains(securityLower, "sensitive") ||
						strings.Contains(securityLower, "confidential") ||
						strings.Contains(securityLower, "pii") {
						piiFields[fieldName] = true
					}
				}
			}
		}
	}

	return piiFields
}

// getCommonPIIPatterns returns common PII field patterns
func getCommonPIIPatterns() []string {
	return []string{
		"email", "phone", "address", "ssn", "password", "secret",
		"api_key", "token", "credential", "personal", "private",
	}
}

// matchesPIIPattern checks if a field name matches common PII patterns
func matchesPIIPattern(key string, patterns []string) bool {
	keyLower := strings.ToLower(key)
	for _, pattern := range patterns {
		if strings.Contains(keyLower, pattern) {
			return true
		}
	}
	return false
}

// shouldExcludeField checks if a field should be excluded
func shouldExcludeField(key string, alwaysExclude map[string]bool, piiFields map[string]bool, commonPIIPatterns []string) bool {
	// Skip if always excluded
	if alwaysExclude[key] {
		return true
	}

	// Skip if marked as PII in spec
	if piiFields[key] {
		return true
	}

	// Skip if matches common PII patterns
	if matchesPIIPattern(key, commonPIIPatterns) {
		return true
	}

	return false
}

// applyFieldOverride applies field override if present
func applyFieldOverride(key string, value any, config *ScenarioCopyConfig) any {
	if overrideValue, hasOverride := config.FieldOverrides[key]; hasOverride {
		return overrideValue
	}
	return value
}

// applyPreserveStateLogic applies preserve state logic to filtered object
func applyPreserveStateLogic(filtered map[string]any, config *ScenarioCopyConfig) {
	if !config.PreserveState {
		// Reset timestamps to current time
		filtered[objects.FieldKeyCreatedAt] = ""
		filtered[objects.FieldKeyUpdatedAt] = ""
		filtered[objects.FieldKeyCreatedBy] = scenarioBuilderAccountSystem
		filtered[objects.FieldKeyUpdatedBy] = scenarioBuilderAccountSystem
	}
}

// getAlwaysExcludeFields returns fields that should always be excluded
func getAlwaysExcludeFields() map[string]bool {
	return map[string]bool{
		objects.FieldKeyID: false, // Will be regenerated
	}
}
