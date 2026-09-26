package objects

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// ValidCriteriaCategories lists canonical criteria categories according to criteria.yaml spec.
var ValidCriteriaCategories = []string{
	"functional",
	"non-functional",
	"acceptance",
	"test",
	"performance",
	"security",
	"compliance",
}

// ValidPriorityTiers lists canonical priority tier enum values (P0 through P5).
var ValidPriorityTiers = []string{
	"P0",
	"P1",
	"P2",
	"P3",
	"P4",
	"P5",
}

// FieldValueRestriction describes constraints placed upon a field's value in an object spec.
type FieldValueRestriction struct {
	FieldName     string
	Kind          string
	EnumValues    []string
	Pattern       string
	PatternRegexp *regexp.Regexp
	HasMin        bool
	Min           float64
	HasMax        bool
	Max           float64
}

var checklistEnumRegex = regexp.MustCompile(`(?i)enum\s*\(([^)]+)\)`)

// ExtractFieldRestrictions parses value restrictions (enum, regex pattern, numeric bounds)
// for a given field from the resolved spec definition or canonical fallbacks.
func ExtractFieldRestrictions(spec *Spec, kind, fieldName string) *FieldValueRestriction {
	restr := &FieldValueRestriction{
		FieldName: fieldName,
		Kind:      kind,
	}

	var fieldDef map[string]any
	if spec != nil && spec.ResolvedFields != nil {
		if rawDef, exists := spec.ResolvedFields[fieldName]; exists {
			if m, ok := rawDef.(map[string]any); ok {
				fieldDef = m
			}
		}
	}

	if fieldDef != nil {
		// 1. Validation block
		if valRaw, hasVal := fieldDef["validation"]; hasVal && valRaw != nil {
			if valMap, ok := valRaw.(map[string]any); ok {
				// Enum
				if enumRaw, hasEnum := valMap["enum"]; hasEnum && enumRaw != nil {
					restr.EnumValues = parseEnumSlice(enumRaw)
				}
				// Pattern
				if patRaw, hasPat := valMap["pattern"]; hasPat && patRaw != nil {
					if patStr, ok := patRaw.(string); ok && patStr != "" {
						restr.Pattern = patStr
						if rx, err := regexp.Compile(patStr); err == nil {
							restr.PatternRegexp = rx
						}
					}
				}
				// Numeric min
				if minRaw, hasMin := valMap["min"]; hasMin && minRaw != nil {
					if minVal, ok := ParseNumericValue(minRaw); ok {
						restr.HasMin = true
						restr.Min = minVal
					}
				} else if minRaw, hasMin := valMap["minimum"]; hasMin && minRaw != nil {
					if minVal, ok := ParseNumericValue(minRaw); ok {
						restr.HasMin = true
						restr.Min = minVal
					}
				}
				// Numeric max
				if maxRaw, hasMax := valMap["max"]; hasMax && maxRaw != nil {
					if maxVal, ok := ParseNumericValue(maxRaw); ok {
						restr.HasMax = true
						restr.Max = maxVal
					}
				} else if maxRaw, hasMax := valMap["maximum"]; hasMax && maxRaw != nil {
					if maxVal, ok := ParseNumericValue(maxRaw); ok {
						restr.HasMax = true
						restr.Max = maxVal
					}
				}
			}
		}

		// 2. Checklist validation fallback for enums if not in validation block
		if len(restr.EnumValues) == 0 {
			if checklistRaw, hasCL := fieldDef["checklist"]; hasCL && checklistRaw != nil {
				if clMap, ok := checklistRaw.(map[string]any); ok {
					if valStr, ok := clMap["validation"].(string); ok {
						parsed := parseChecklistEnumString(valStr)
						if len(parsed) > 0 {
							restr.EnumValues = parsed
						}
					}
				}
			}
		}
	}

	// 3. Fallback for canonical fields if spec not found or didn't supply enum
	if len(restr.EnumValues) == 0 {
		if (kind == KindCriteria || strings.EqualFold(kind, "criteria")) && fieldName == FieldKeyCategory {
			restr.EnumValues = ValidCriteriaCategories
		} else if fieldName == FieldKeyPriorityTier {
			restr.EnumValues = ValidPriorityTiers
		}
	}

	return restr
}

// ValidateFieldValue validates a single field value against the restriction.
func (r *FieldValueRestriction) ValidateFieldValue(value any) error {
	if r == nil || value == nil {
		return nil
	}

	// If EnumValues are defined, enforce strict enum membership
	if len(r.EnumValues) > 0 {
		switch v := value.(type) {
		case []string:
			for _, item := range v {
				if !matchesAnyEnum(item, r.EnumValues) {
					return fmt.Errorf("field %q has invalid value %q; must be one of: [%s]", r.FieldName, item, strings.Join(r.EnumValues, ", "))
				}
			}
		case []any:
			for _, item := range v {
				itemStr := fmt.Sprintf("%v", item)
				if !matchesAnyEnum(itemStr, r.EnumValues) {
					return fmt.Errorf("field %q has invalid value %q; must be one of: [%s]", r.FieldName, itemStr, strings.Join(r.EnumValues, ", "))
				}
			}
		default:
			valStr := fmt.Sprintf("%v", value)
			if !matchesAnyEnum(valStr, r.EnumValues) {
				return fmt.Errorf("field %q has invalid value %q; must be one of: [%s]", r.FieldName, valStr, strings.Join(r.EnumValues, ", "))
			}
		}
	}

	// Pattern check
	if r.PatternRegexp != nil {
		valStr := fmt.Sprintf("%v", value)
		if !r.PatternRegexp.MatchString(valStr) {
			return fmt.Errorf("field %q value %q does not match required pattern %q", r.FieldName, valStr, r.Pattern)
		}
	}

	// Numeric range bounds check
	if r.HasMin || r.HasMax {
		if num, ok := ParseNumericValue(value); ok {
			if r.HasMin && num < r.Min {
				return fmt.Errorf("field %q value %v is less than minimum allowed %v", r.FieldName, num, r.Min)
			}
			if r.HasMax && num > r.Max {
				return fmt.Errorf("field %q value %v is greater than maximum allowed %v", r.FieldName, num, r.Max)
			}
		}
	}

	return nil
}

// ValidateObjectFieldRestrictions validates all supplied update fields against the spec for the kind.
// Any field failing value-restricted validation causes an error to be returned immediately (fail-closed).
func ValidateObjectFieldRestrictions(spec *Spec, kind string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	if spec == nil {
		if loader := GetGlobalSpecLoader(); loader != nil {
			spec, _ = loader.LoadSpecWithInheritance(kind + ".yaml")
		}
	}

	for k, v := range updates {
		// Skip control, metadata, and lifecycle status keys (status is governed by lifecycle engine)
		if k == FieldKeyID || k == FieldKeyKind || k == FieldKeyStatus || k == "expected_updated_at" || k == "_dry_run" {
			continue
		}
		// Skip unset markers
		if v == nil || isFieldUnsetMarker(v) {
			continue
		}

		restr := ExtractFieldRestrictions(spec, kind, k)
		if restr != nil {
			if err := restr.ValidateFieldValue(v); err != nil {
				return err
			}
		}
	}

	return nil
}

func matchesAnyEnum(candidate string, allowed []string) bool {
	for _, a := range allowed {
		if candidate == a {
			return true
		}
	}
	return false
}

func parseEnumSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			} else if item != nil {
				res = append(res, fmt.Sprintf("%v", item))
			}
		}
		return res
	default:
		return nil
	}
}

func parseChecklistEnumString(s string) []string {
	matches := checklistEnumRegex.FindStringSubmatch(s)
	if len(matches) < 2 {
		return nil
	}
	content := strings.TrimSpace(matches[1])
	if strings.Contains(strings.ToLower(content), "must be") ||
		strings.Contains(strings.ToLower(content), "defined categories") ||
		strings.Contains(strings.ToLower(content), "controlled vocabulary") {
		return nil
	}
	parts := strings.Split(content, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		cleaned := strings.Trim(strings.TrimSpace(p), `"' \`)
		// Also clean escaped quotes like \"
		cleaned = strings.ReplaceAll(cleaned, `\"`, "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" && !strings.Contains(cleaned, " ") {
			out = append(out, cleaned)
		}
	}
	return out
}

// ParseNumericValue converts a numeric or numeric-string value to float64.
func ParseNumericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func isFieldUnsetMarker(v any) bool {
	if v == nil {
		return false
	}
	// Support both string marker and any type with FieldUnset tag
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.String && (v == "__UNSET__" || v == "UNSET") {
		return true
	}
	return false
}
