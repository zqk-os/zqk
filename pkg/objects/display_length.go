package objects

// GetDisplayLength gets the display_length constraint from a spec field definition
// Returns the display_length value if found, or the default value if not found
// This is a shared utility used by table formatters and validators
func GetDisplayLength(spec *Spec, fieldName string, defaultLen int) int {
	if SpecResolvedFieldsMissing(spec) {
		return defaultLen
	}

	fieldDef, ok := spec.ResolvedFields[fieldName]
	if !ok {
		return defaultLen
	}

	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		return defaultLen
	}

	validation, ok := fieldMap["validation"].(map[string]any)
	if !ok {
		return defaultLen
	}
	switch v := validation["display_length"].(type) {
	case int:
		return v
	case

		// Try float64 (YAML numbers can be parsed as float64)
		float64:
		return int(v)
	}

	return defaultLen
}

// GetDisplayLengthFromFieldDef gets display_length from a field definition map directly
// Useful when you already have the field definition
func GetDisplayLengthFromFieldDef(fieldDef map[string]any, defaultLen int) int {
	validation, ok := fieldDef["validation"].(map[string]any)
	if !ok {
		return defaultLen
	}
	switch v := validation["display_length"].(type) {
	case int:
		return v
	case

		// Try float64 (YAML numbers can be parsed as float64)
		float64:
		return int(v)
	}

	return defaultLen
}
