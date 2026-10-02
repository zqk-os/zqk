package provider

import "strings"

// QueryBuilder provides a fluent API for constructing queries across backends
type QueryBuilder interface {
	Match(pattern string) QueryBuilder
	Where(condition string) QueryBuilder
	Return(fields ...string) QueryBuilder
	Limit(n int) QueryBuilder
	OrderBy(field string, direction string) QueryBuilder
	Build() Query
}

// QueryTranslator translates queries between different query languages
type QueryTranslator interface {
	Translate(query Query, targetLanguage QueryLanguage) (Query, error)
}

// ToLabel converts a snake_case kind to PascalCase label (e.g. "backlog_item" -> "BacklogItem").
// Returns empty string if the kind contains invalid characters to prevent Cypher injection.
func ToLabel(kind string) string {
	parts := strings.Split(kind, "_")
	var labelParts []string
	for _, part := range parts {
		if part != "" {
			labelParts = append(labelParts, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		}
	}
	label := strings.Join(labelParts, "")
	for _, r := range label {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return "" // Reject unsafe label
		}
	}
	return label
}
