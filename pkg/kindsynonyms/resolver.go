// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package kindsynonyms

import (
	"sort"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NormalizeKind normalizes raw kind strings by trimming whitespace and lowercasing.
func NormalizeKind(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// Resolver provides concurrency-safe kind synonym resolution and canonical kind mapping.
type Resolver struct {
	mu            sync.RWMutex
	synonymToKind map[string]string
	canonicalMap  map[string]bool
	kindToAliases map[string][]string
}

// NewResolver constructs a Resolver pre-populated with deterministic default aliases.
func NewResolver() *Resolver {
	r := &Resolver{
		synonymToKind: make(map[string]string),
		canonicalMap:  make(map[string]bool),
		kindToAliases: make(map[string][]string),
	}
	r.registerDefaults()
	return r
}

func (r *Resolver) registerDefaults() {
	for _, entry := range DefaultKindAliasEntries() {
		normKind := NormalizeKind(entry.Kind)
		r.canonicalMap[normKind] = true
		for _, alias := range entry.Aliases {
			normAlias := NormalizeKind(alias)
			r.synonymToKind[normAlias] = normKind
			r.kindToAliases[normKind] = append(r.kindToAliases[normKind], normAlias)
		}
	}
}

// Register registers a custom synonym alias for a canonical kind.
// Returns an error if canonicalKind or synonym is empty, or if synonym is already mapped to another kind.
func (r *Resolver) Register(canonicalKind, synonym string) error {
	normKind := NormalizeKind(canonicalKind)
	if normKind == "" {
		return errfmt.Errorf("canonical kind cannot be empty")
	}
	normSynonym := NormalizeKind(synonym)
	if normSynonym == "" {
		return errfmt.Errorf("synonym cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.canonicalMap[normKind] = true
	if normSynonym == normKind {
		return nil
	}

	if existing, found := r.synonymToKind[normSynonym]; found && existing != normKind {
		return errfmt.Errorf("synonym %q is already mapped to %q", synonym, existing)
	}

	r.synonymToKind[normSynonym] = normKind
	alreadyInList := false
	for _, a := range r.kindToAliases[normKind] {
		if a == normSynonym {
			alreadyInList = true
			break
		}
	}
	if !alreadyInList {
		r.kindToAliases[normKind] = append(r.kindToAliases[normKind], normSynonym)
	}
	return nil
}

// ResolveKind resolves raw input to its canonical kind.
// Empty input returns an empty string.
// If input is unknown, it returns the normalized input (pass-through fallback).
func (r *Resolver) ResolveKind(raw string) string {
	normalized := NormalizeKind(raw)
	if normalized == "" {
		return ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.canonicalMap[normalized] {
		return normalized
	}
	if canonical, ok := r.synonymToKind[normalized]; ok {
		return canonical
	}

	if strings.HasSuffix(normalized, "s") {
		singular := strings.TrimSuffix(normalized, "s")
		if r.canonicalMap[singular] {
			return singular
		}
		if canonical, ok := r.synonymToKind[singular]; ok {
			return canonical
		}
	}

	return normalized
}

// ResolveKindStrict resolves raw input to its canonical kind.
// Returns an error if input is empty or resolves to an unknown kind.
func (r *Resolver) ResolveKindStrict(raw string) (string, error) {
	normalized := NormalizeKind(raw)
	if normalized == "" {
		return "", errfmt.Errorf("kind argument cannot be empty")
	}
	resolved := r.ResolveKind(raw)
	if r.IsCanonical(resolved) {
		return resolved, nil
	}
	return "", errfmt.Errorf("unknown kind %q", raw)
}

// IsCanonical reports whether the kind is a known canonical kind.
func (r *Resolver) IsCanonical(kind string) bool {
	normalized := NormalizeKind(kind)
	if normalized == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.canonicalMap[normalized]
}

// AliasesForKind returns all registered aliases for a canonical kind.
func (r *Resolver) AliasesForKind(kind string) []string {
	normalized := NormalizeKind(kind)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if aliases, ok := r.kindToAliases[normalized]; ok {
		return append([]string(nil), aliases...)
	}
	return []string{}
}

// AllCanonicalKinds returns a sorted slice of all registered canonical kinds.
func (r *Resolver) AllCanonicalKinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]string, 0, len(r.canonicalMap))
	for k := range r.canonicalMap {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

var (
	defaultResolver     *Resolver
	defaultResolverOnce sync.Once
)

// DefaultResolver returns the singleton default resolver.
func DefaultResolver() *Resolver {
	defaultResolverOnce.Do(func() {
		defaultResolver = NewResolver()
	})
	return defaultResolver
}

// ResolveKind resolves raw input using the package-level default resolver.
func ResolveKind(raw string) string {
	return DefaultResolver().ResolveKind(raw)
}

// ResolveKindStrict resolves raw input strictly using the package-level default resolver.
func ResolveKindStrict(raw string) (string, error) {
	return DefaultResolver().ResolveKindStrict(raw)
}
