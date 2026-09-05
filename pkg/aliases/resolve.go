// Package aliases provides small helpers for domain-specific string alias tables.
//
// Use this for static map[string]string alias → canonical normalization at command
// boundaries. Do not use pkg/cli.AliasRegistry default operation aliases for unrelated
// domains: e.g. that table maps "modify" → "update" (CRUD), which conflicts with spec
// field lifecycle verbs where "modify" must stay "modify".
//
// Related: objects.RegisterStatusAlias / statusAliases for lifecycle status variants.
package aliases

import "strings"

// ResolveStatic maps trimmed, lowercased s through aliasToCanonical when s is a key.
// Otherwise it returns trimmed, lowercased s (canonical spellings and unknown input pass through).
func ResolveStatic(s string, aliasToCanonical map[string]string) string {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return t
	}
	if c, ok := aliasToCanonical[t]; ok {
		return c
	}
	return t
}
