// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package aliases

import "strings"

// Table represents an immutable, domain-scoped alias resolution table.
// It encapsulates a domain identifier and normalized alias-to-canonical mapping,
// preventing cross-domain alias contamination and ensuring thread-safe concurrent lookups.
type Table struct {
	domain  string
	mapping map[string]string
}

// NormalizeMap creates a new map where all alias keys are trimmed and lowercased,
// and canonical values are trimmed. Entries with empty keys after trimming are omitted.
// If the input map is nil or empty, NormalizeMap returns an initialized empty map.
func NormalizeMap(aliases map[string]string) map[string]string {
	normalized := make(map[string]string, len(aliases))
	for k, v := range aliases {
		cleanKey := strings.ToLower(strings.TrimSpace(k))
		if cleanKey != "" {
			normalized[cleanKey] = strings.TrimSpace(v)
		}
	}
	return normalized
}

// NewTable constructs an immutable domain-specific alias table.
// Keys in aliases are automatically normalized via NormalizeMap.
func NewTable(domain string, aliases map[string]string) *Table {
	return &Table{
		domain:  strings.TrimSpace(domain),
		mapping: NormalizeMap(aliases),
	}
}

// Domain returns the domain identifier associated with this table.
// If the receiver is nil, it returns an empty string.
func (t *Table) Domain() string {
	if t == nil {
		return ""
	}
	return t.domain
}

// Resolve resolves the input string through the table's normalized mapping.
// If the table is nil or the input is empty or unknown, it returns the trimmed,
// lowercased input string.
func (t *Table) Resolve(s string) string {
	if t == nil {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ResolveStatic(s, t.mapping)
}

// HasAlias reports whether the normalized string is an explicitly registered alias in this table.
// If the receiver is nil or the input is empty/unknown, it returns false.
func (t *Table) HasAlias(s string) bool {
	if t == nil {
		return false
	}
	clean := strings.ToLower(strings.TrimSpace(s))
	if clean == "" {
		return false
	}
	_, ok := t.mapping[clean]
	return ok
}

// Len returns the count of registered aliases in the table.
// If the receiver is nil, it returns 0.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.mapping)
}

// Mappings returns a defensive copy of the underlying normalized alias map.
// If the receiver is nil, it returns an empty map.
func (t *Table) Mappings() map[string]string {
	if t == nil {
		return make(map[string]string)
	}
	clone := make(map[string]string, len(t.mapping))
	for k, v := range t.mapping {
		clone[k] = v
	}
	return clone
}
