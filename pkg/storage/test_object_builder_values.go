// Extracted from test_object_builder.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func generateStringTestValue(fieldName string, fieldMap, fieldValidation map[string]any, index int) any {
	// Check for pattern constraints first
	if fieldValidation != nil {
		if pattern := objects.GetString(fieldValidation, "pattern"); pattern != "" {
			if val := matchPatternAndGenerate(pattern, fieldName, index); val != nil {
				return val
			}
		}
	}

	// Check semantic type for hints
	semanticType, _ := fieldMap["semantic_type"].(string)
	if semanticType == "timestamp" {
		return generateISO8601Timestamp(index)
	}
	if semanticType == "reference" {
		return generateReferenceFromString(fieldName, fieldValidation)
	}

	// Handle special field name patterns
	if strings.HasSuffix(fieldName, "_by") {
		return pkgctx.SystemAccountID
	}
	if fieldName == ConstMiscOriginProject {
		return validation.DefaultOriginProject
	}
	if fieldName == "origin_system" {
		return validation.DefaultOriginSystem
	}

	// Default: generate a test string
	return fmt.Sprintf("test_%s_%d", fieldName, index+1)
}

// matchPatternAndGenerate matches common regex patterns and generates appropriate values
//
//nolint:gocyclo // Function intentionally handles many regex pattern types
func matchPatternAndGenerate(pattern, fieldName string, index int) any {
	// ISO 8601 timestamp patterns (exact match: ^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$)
	if pattern == ConstMiscD4D2D2TD2D2D2Z ||
		strings.Contains(pattern, ConstMiscD4D2D2TD2D2D2Z1) {
		return generateISO8601Timestamp(index)
	}

	// Duration patterns (flush_interval: ^\d+[smhd]|0$)
	if pattern == ConstMiscDSmhd0 || strings.Contains(pattern, "\\d+[smhd]") {
		// Generate valid duration: "5m", "10s", "1h", "0"
		durations := []string{"5m", "10s", "1h", "30m", "0"}
		return durations[index%len(durations)]
	}

	// Lowercase identifier patterns
	if pattern == "^[a-z_]+$" || strings.Contains(pattern, "[a-z_]") {
		return generateLowercaseIdentifier(fieldName)
	}

	// Date patterns (YYYY-MM-DD, not datetime)
	if pattern == ConstMiscD4D2D2 || (strings.Contains(pattern, ConstMiscD4D2D21) && !strings.Contains(pattern, "T")) {
		day := (index % 28) + 1
		return fmt.Sprintf("2025-01-%02d", day)
	}

	// Account ID patterns
	if strings.Contains(pattern, "account:") || (strings.Contains(pattern, "ACC-") && strings.Contains(pattern, "\\d{3,}")) {
		return pkgctx.SystemAccountID
	}

	// Namespace ID patterns
	if strings.Contains(pattern, ConstMiscZqkDomainIntegration) && strings.Contains(pattern, ":") {
		return "zqk:kernel"
	}

	// Date range patterns
	if strings.Contains(fieldName, "horizon") || strings.Contains(fieldName, "range") {
		return ConstMisc20250101To20271231
	}

	return nil
}

// generateISO8601Timestamp generates an ISO 8601 formatted timestamp
// Format: YYYY-MM-DDTHH:MM:SSZ (exact pattern: ^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$)
func generateISO8601Timestamp(index int) string {
	day := (index % 28) + 1
	hour := (index % 24)
	minute := (index * 2) % 60
	second := (index * 3) % 60
	return fmt.Sprintf(ConstMisc20250102dt02d02d02dz, day, hour, minute, second)
}

// generateLowercaseIdentifier generates a lowercase identifier from field name
func generateLowercaseIdentifier(fieldName string) string {
	baseName := strings.ToLower(fieldName)
	baseName = strings.ReplaceAll(baseName, "-", "_")
	var result strings.Builder
	for _, r := range baseName {
		if (r >= 'a' && r <= 'z') || r == '_' {
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		result.WriteString("test_field")
	}
	return result.String()
}

// generateReferenceFromString generates a reference value for string fields with reference semantic type
func generateReferenceFromString(fieldName string, fieldValidation map[string]any) string {
	refKind := ""
	if fieldValidation != nil {
		if rk := objects.GetString(fieldValidation, ConstMiscReferenceKind); rk != "" {
			refKind = rk
		}
	}
	if refKind == emptyValue {
		refKind = inferReferenceKind(fieldName)
	}
	return generateReferenceID(refKind)
}

// generateReferenceID generates a reference ID for a given kind
func generateReferenceID(refKind string) string {
	if refKind == emptyValue {
		return fmt.Sprintf("REF-%d", testReferenceIDSerial)
	}
	if refKind == objects.KindAccount {
		return ConstMiscAccountTestuser // Use valid username that matches validation pattern
	}
	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err == nil {
		prefixes := idValidator.GetValidPrefixes(refKind)
		if len(prefixes) > 0 {
			prefix := strings.TrimSuffix(prefixes[0], "-")
			return fmt.Sprintf("%s-%d", prefix, testReferenceIDSerial)
		}
	}
	if len(refKind) >= 3 {
		return fmt.Sprintf("%s-%d", strings.ToUpper(refKind[:3]), testReferenceIDSerial)
	}
	return fmt.Sprintf("REF-%d", testReferenceIDSerial)
}

// generateReferenceTestValue generates a test value for reference type fields
func generateReferenceTestValue(fieldName string, fieldValidation map[string]any) string {
	refKind := ""
	if fieldValidation != nil {
		if rk := objects.GetString(fieldValidation, ConstMiscReferenceKind); rk != "" {
			refKind = rk
		}
	}
	if refKind == emptyValue {
		refKind = inferReferenceKind(fieldName)
	}
	return generateReferenceID(refKind)
}

// generateDateTimeTestValue generates a test datetime value
func generateDateTimeTestValue(fieldValidation map[string]any, index int) string {
	if fieldValidation != nil {
		if pattern := objects.GetString(fieldValidation, "pattern"); pattern != "" {
			if strings.Contains(pattern, ConstMiscD4D2D2T) ||
				strings.Contains(pattern, ConstMiscD4D2D2T1) ||
				strings.Contains(pattern, ConstMiscD4D2D2TD2D2D2Z1) {
				day := (index % 28) + 1
				return fmt.Sprintf(ConstMisc20250102dt000000z, day)
			}
		}
	}
	day := (index % 28) + 1
	return fmt.Sprintf(ConstMisc20250102dt000000z, day)
}

// generateDateTestValue generates a test date value (YYYY-MM-DD format)
func generateDateTestValue(fieldValidation map[string]any, index int) string {
	if fieldValidation != nil {
		if pattern := objects.GetString(fieldValidation, "pattern"); pattern != "" {
			if pattern == ConstMiscD4D2D2 || (strings.Contains(pattern, ConstMiscD4D2D21) && !strings.Contains(pattern, "T")) {
				day := (index % 28) + 1
				return fmt.Sprintf("2025-01-%02d", day)
			}
		}
	}
	day := (index % 28) + 1
	return fmt.Sprintf("2025-01-%02d", day)
}

// generateListTestValue generates a test list/array value
func generateListTestValue(fieldName string, fieldMap, fieldValidation map[string]any, index int) []any {
	minCount := 1
	if fieldValidation != nil {
		if mc, ok := fieldValidation["minCount"].(float64); ok && mc > 0 {
			minCount = int(mc)
		}
	}

	semanticType, _ := fieldMap["semantic_type"].(string)
	if semanticType == "reference" || strings.HasSuffix(fieldName, "_refs") {
		return generateReferenceList(fieldName, fieldValidation, minCount)
	}

	list := make([]any, minCount)
	for i := range list {
		list[i] = fmt.Sprintf("item_%d_%d", index+1, i+1)
	}
	return list
}

// generateReferenceList generates a list of reference IDs
func generateReferenceList(fieldName string, fieldValidation map[string]any, minCount int) []any {
	refKind := ""
	if fieldValidation != nil {
		if rk := objects.GetString(fieldValidation, ConstMiscReferenceKind); rk != "" {
			refKind = rk
		}
	}
	if refKind == emptyValue {
		refKind = inferReferenceKind(fieldName)
	}

	list := make([]any, minCount)
	for i := range list {
		if refKind != emptyValue {
			if refKind == objects.KindAccount {
				list[i] = ConstMiscAccountTestuser // Use valid username that matches validation pattern
			} else {
				idValidator := validation.GetIDValidator()
				if err := idValidator.LoadPatterns(); err == nil {
					prefixes := idValidator.GetValidPrefixes(refKind)
					if len(prefixes) > 0 {
						prefix := strings.TrimSuffix(prefixes[0], "-")
						list[i] = fmt.Sprintf("%s-%03d", prefix, testReferenceIDSerial+i)
					} else {
						list[i] = fmt.Sprintf("%s-%03d", strings.ToUpper(refKind[:3]), testReferenceIDSerial+i)
					}
				} else {
					list[i] = fmt.Sprintf("%s-%03d", strings.ToUpper(refKind[:3]), testReferenceIDSerial+i)
				}
			}
		} else {
			list[i] = fmt.Sprintf("REF-%03d", testReferenceIDSerial+i)
		}
	}
	return list
}
