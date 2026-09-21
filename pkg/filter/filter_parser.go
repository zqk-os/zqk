package filter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

const (
	filterOpNotEqual     = "!="
	filterOpEqual        = "="
	filterOpColon        = ":"
	filterValueSeparator = "|"
	filterMongoOpNotIn   = "$nin"
	filterMongoOpNotEq   = "$ne"
	filterMongoOpIn      = "$in"
	emptyValue           = ""
)

// ParseFilterString parses a filter string into field name and filter value
// This is a shared utility used across commands and daemon subsystems
// Supports:
//   - field=value (equality)
//   - field==value (equality, normalized from == to =)
//   - field!=value (not equal, converts to {"$ne": value})
//   - field:value (equality, alternative syntax)
//   - field!="value1|value2" (not in list, converts to {"$nin": ["value1", "value2"]})
//   - field="value1|value2" (in list, converts to {"$in": ["value1", "value2"]})
//   - field={"$op": value} (explicit operator syntax)
//
// Normalizes kebab-case field names (e.g., "priority-plan-ref") to snake_case ("priority_plan_ref").
// Trims whitespace around operators (e.g., "field = value" -> field="value").
func ParseFilterString(filterStr string) (field string, value any, err error) {
	filterStr = strings.TrimSpace(filterStr)
	if filterStr == "" {
		return "", nil, errfmt.Errorf("empty filter expression: expected <field>=<value>, <field>!=<value>, or <field>:<value>")
	}

	// Try != operator first (not equal)
	if strings.Contains(filterStr, filterOpNotEqual) {
		f, v, e := parseNotEqualFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	// Try == operator (programming style equality)
	if strings.Contains(filterStr, "==") {
		f, v, e := parseDoubleEqualFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	// Try = operator (equality)
	if strings.Contains(filterStr, filterOpEqual) {
		f, v, e := parseEqualFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	// Try colon separator (equality, alternative syntax)
	if strings.Contains(filterStr, filterOpColon) {
		f, v, e := parseColonFilter(filterStr)
		return f, ResolveTimeTokens(v), e
	}

	return "", nil, errfmt.Errorf("invalid filter syntax %q: missing operator (expected <field>=<value>, <field>!=<value>, <field>:<value>, or <field>=\"val1|val2\"). Supported operators: '=', '!=', ':', '|'", filterStr)
}

func normalizeFieldName(fieldPart string) string {
	name := strings.TrimSpace(fieldPart)
	name = strings.TrimLeft(name, "-")
	if strings.Contains(name, "-") {
		name = strings.ReplaceAll(name, "-", "_")
	}
	return name
}

// parseNotEqualFilter parses a filter with != operator
func parseNotEqualFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, filterOpNotEqual)
	if !ok {
		return "", nil, errfmt.Errorf("invalid != filter format: %s", filterStr)
	}

	fieldName := normalizeFieldName(fieldPart)
	if fieldName == "" {
		return "", nil, errfmt.Errorf("invalid filter syntax %q: missing field name before != operator", filterStr)
	}
	fieldValue := strings.TrimSpace(valuePart)
	fieldValue = strings.Trim(fieldValue, `"'`)

	// Check for pipe-separated values (OR condition)
	if strings.Contains(fieldValue, filterValueSeparator) {
		valueList := parsePipeSeparatedValues(fieldValue)
		return fieldName, map[string]any{filterMongoOpNotIn: valueList}, nil
	}

	// Single value: convert to $ne (not equal)
	parsedValue := parseYAMLValue(fieldValue)
	return fieldName, map[string]any{filterMongoOpNotEq: parsedValue}, nil
}

// parseDoubleEqualFilter parses a filter with == operator
func parseDoubleEqualFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, "==")
	if !ok {
		return "", nil, errfmt.Errorf("invalid == filter format: %s", filterStr)
	}

	fieldName := normalizeFieldName(fieldPart)
	if fieldName == "" {
		return "", nil, errfmt.Errorf("invalid filter syntax %q: missing field name before == operator", filterStr)
	}
	fieldValue := strings.TrimSpace(valuePart)
	fieldValue = strings.Trim(fieldValue, `"'`)

	if strings.Contains(fieldValue, filterValueSeparator) {
		valueList := parsePipeSeparatedValues(fieldValue)
		return fieldName, map[string]any{filterMongoOpIn: valueList}, nil
	}

	parsedValue := parseYAMLValue(fieldValue)
	if filterMap, ok := parsedValue.(map[string]any); ok {
		return fieldName, filterMap, nil
	}

	return fieldName, parsedValue, nil
}

// parseEqualFilter parses a filter with = operator
func parseEqualFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, filterOpEqual)
	if !ok {
		return "", nil, errfmt.Errorf("invalid = filter format: %s", filterStr)
	}

	fieldName := normalizeFieldName(fieldPart)
	if fieldName == "" {
		return "", nil, errfmt.Errorf("invalid filter syntax %q: missing field name before = operator", filterStr)
	}
	fieldValue := strings.TrimSpace(valuePart)
	fieldValue = strings.Trim(fieldValue, `"'`)

	// Check for pipe-separated values (OR condition)
	if strings.Contains(fieldValue, filterValueSeparator) {
		valueList := parsePipeSeparatedValues(fieldValue)
		return fieldName, map[string]any{filterMongoOpIn: valueList}, nil
	}

	// Try to parse as YAML value (supports JSON-like maps for operators)
	parsedValue := parseYAMLValue(fieldValue)

	// If parsed value is a map, it might be an explicit operator (e.g., {"$gt": 10})
	if filterMap, ok := parsedValue.(map[string]any); ok {
		return fieldName, filterMap, nil
	}

	return fieldName, parsedValue, nil
}

// parseColonFilter parses a filter with : separator
func parseColonFilter(filterStr string) (field string, value any, err error) {
	fieldPart, valuePart, ok := strings.Cut(filterStr, filterOpColon)
	if !ok {
		return "", nil, errfmt.Errorf("invalid : filter format: %s", filterStr)
	}

	fieldName := normalizeFieldName(fieldPart)
	if fieldName == "" {
		return "", nil, errfmt.Errorf("invalid filter syntax %q: missing field name before : operator", filterStr)
	}
	fieldValue := strings.TrimSpace(valuePart)

	parsedValue := parseYAMLValue(fieldValue)
	return fieldName, parsedValue, nil
}

// parsePipeSeparatedValues parses pipe-separated values into a list
func parsePipeSeparatedValues(fieldValue string) []string {
	valueList := make([]string, 0)
	for v := range strings.SplitSeq(fieldValue, filterValueSeparator) {
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if v != emptyValue {
			valueList = append(valueList, v)
		}
	}
	return valueList
}

// parseYAMLValue parses a value as YAML, falling back to string if parsing fails
func parseYAMLValue(fieldValue string) any {
	var parsedValue any
	if err := yaml.Unmarshal([]byte(fieldValue), &parsedValue); err != nil {
		// If YAML parsing fails, treat as string
		return fieldValue
	}
	return parsedValue
}

// ValidateFilterFields checks that all filter fields exist in the object kind schema,
// and returns an actionable error with fuzzy 'Did you mean?' suggestions if unknown fields are found.
func ValidateFilterFields(kind string, filters map[string]any) error {
	if kind == "" || len(filters) == 0 {
		return nil
	}
	registry := objects.GetGlobalFieldRegistry()
	if registry == nil {
		return nil
	}
	_ = registry.LoadFields()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil || kindFields == nil || len(kindFields.AllFields) == 0 {
		return nil
	}

	validFields := make(map[string]bool, len(kindFields.AllFields)+16)
	fieldList := make([]string, 0, len(kindFields.AllFields)+16)
	for _, f := range kindFields.AllFields {
		validFields[f.Name] = true
		fieldList = append(fieldList, f.Name)
	}
	// Common dynamic or reference fields that are universally valid for query filtering
	extraValid := []string{
		"id", "kind", "title", "description", "status", "created_at", "created_by",
		"updated_at", "updated_by", "namespace_id", "schema_version", "tags",
		"related_object_refs", "persona_refs", "milestone_refs", "workstream_refs",
		"meta", "child_rollup",
	}
	for _, ef := range extraValid {
		if !validFields[ef] {
			validFields[ef] = true
			fieldList = append(fieldList, ef)
		}
	}

	for fieldName := range filters {
		// Skip mongo / logical operators like $or, $and
		if strings.HasPrefix(fieldName, "$") {
			continue
		}
		// Handle dotted paths like child_rollup.completed_count
		rootField := fieldName
		if dotIdx := strings.Index(fieldName, "."); dotIdx > 0 {
			rootField = fieldName[:dotIdx]
		}

		if !validFields[rootField] {
			suggestions := SuggestSimilarFields(rootField, fieldList)
			if len(suggestions) > 0 {
				quoted := make([]string, len(suggestions))
				for i, s := range suggestions {
					quoted[i] = fmt.Sprintf("%q", s)
				}
				return errfmt.Errorf("unknown filter field %q for kind %q. Did you mean %s?", fieldName, kind, strings.Join(quoted, " or "))
			}
			return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("unknown filter field %q for kind %q. Check available fields with 'zqk object template %s'", fieldName, kind, kind)))
		}
	}
	return nil
}

// SuggestSimilarFields finds closest matching field names using prefix, substring, and Levenshtein distance.
func SuggestSimilarFields(input string, validFields []string) []string {
	inputLower := strings.ToLower(strings.TrimSpace(input))
	if inputLower == "" {
		return nil
	}

	// Exact case-insensitive match
	for _, f := range validFields {
		if strings.EqualFold(f, inputLower) {
			return []string{f}
		}
	}

	type match struct {
		name string
		dist int
	}
	var scored []match

	// Prefix matches
	for _, f := range validFields {
		fLower := strings.ToLower(f)
		if strings.HasPrefix(fLower, inputLower) {
			scored = append(scored, match{name: f, dist: 0})
		}
	}

	// Levenshtein distance <= 2 (or 3 for longer words)
	maxDist := 2
	if len(inputLower) >= 8 {
		maxDist = 3
	}

	for _, f := range validFields {
		fLower := strings.ToLower(f)
		dist := computeLevenshtein(inputLower, fLower)
		if dist <= maxDist {
			already := false
			for _, m := range scored {
				if m.name == f {
					already = true
					break
				}
			}
			if !already {
				scored = append(scored, match{name: f, dist: dist})
			}
		}
	}

	slices.SortFunc(scored, func(a, b match) int {
		return a.dist - b.dist
	})

	var result []string
	bestDist := -1
	for _, m := range scored {
		if bestDist == -1 {
			bestDist = m.dist
		}
		// If we have a very close match (distance <= 1), don't include worse matches
		if bestDist <= 1 && m.dist > bestDist {
			continue
		}
		if m.dist <= bestDist+1 {
			result = append(result, m.name)
		}
		if len(result) >= 3 {
			break
		}
	}
	return result
}

func computeLevenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			dp[i][j] = min(dp[i-1][j]+1, min(dp[i][j-1]+1, dp[i-1][j-1]+cost))
		}
	}
	return dp[la][lb]
}
