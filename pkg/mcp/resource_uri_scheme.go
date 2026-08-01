package mcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
)

// ResourceURISchemeResolver determines the appropriate URI scheme for a resource based on its location
type ResourceURISchemeResolver struct {
	schemeRules []ResourceURISchemeRule
}

// ResourceURISchemeRule defines a rule for determining URI scheme based on resource location
type ResourceURISchemeRule struct {
	Pattern     string // Path pattern to match (supports glob patterns)
	Scheme      string // URI scheme to use (e.g., "file://", "docs://", "internal://")
	Category    string // Resource category (e.g., "guide", "architecture", "internal")
	Description string // Description of this rule
}

// NewResourceURISchemeResolver creates a new URI scheme resolver with default rules
func NewResourceURISchemeResolver() *ResourceURISchemeResolver {
	return &ResourceURISchemeResolver{
		schemeRules: getDefaultResourceURISchemeRules(),
	}
}

// NewResourceURISchemeResolverFromConfig creates a new URI scheme resolver from ServerConfig
// Uses configured rules if available, otherwise falls back to defaults
func NewResourceURISchemeResolverFromConfig(config *ServerConfig) *ResourceURISchemeResolver {
	return &ResourceURISchemeResolver{
		schemeRules: LoadResourceURISchemeRulesFromConfig(config),
	}
}

// NewResourceURISchemeResolverWithRules creates a new URI scheme resolver with custom rules
func NewResourceURISchemeResolverWithRules(rules []ResourceURISchemeRule) *ResourceURISchemeResolver {
	return &ResourceURISchemeResolver{
		schemeRules: rules,
	}
}

// ResolveURI determines the appropriate URI for a resource path
// Returns the full URI with scheme and the category
func (r *ResourceURISchemeResolver) ResolveURI(relPath string) (uri string, category string) {
	// Normalize path for matching
	normalizedPath := filepath.ToSlash(relPath)

	// Try each rule in order (first match wins)
	for _, rule := range r.schemeRules {
		if r.matchesPattern(normalizedPath, rule.Pattern) {
			// Build URI with the determined scheme
			uri = fmt.Sprintf("%s%s", rule.Scheme, normalizedPath)
			return uri, rule.Category
		}
	}

	// Default fallback: use file:// scheme
	return fmt.Sprintf("file://%s", normalizedPath), "documentation"
}

// matchesPattern checks if a path matches a pattern
// Supports glob patterns and simple prefix/suffix matching
func (r *ResourceURISchemeResolver) matchesPattern(path, pattern string) bool {
	// Simple prefix match
	if strings.HasPrefix(pattern, "**/") {
		// Match anywhere in path
		suffix := strings.TrimPrefix(pattern, "**/")
		return strings.Contains(path, suffix)
	}

	// Simple suffix match
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(path, prefix)
	}

	// Exact match
	if pattern == path {
		return true
	}

	// Prefix match
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}

	// Simple glob matching (basic implementation)
	if strings.Contains(pattern, "*") {
		// Convert glob pattern to simple matching
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(path, parts[0]) && strings.HasSuffix(path, parts[1])
		}
	}

	return false
}

// getDefaultResourceURISchemeRules returns default rules for resource URI schemes
func getDefaultResourceURISchemeRules() []ResourceURISchemeRule {
	return []ResourceURISchemeRule{
		{
			Pattern:     filepath.ToSlash(paths.ProcessInternalDir) + "/**",
			Scheme:      "internal://",
			Category:    "internal",
			Description: "Internal documentation (lifecycles, specs, etc.)",
		},
		{
			Pattern:     filepath.ToSlash(paths.ProcessArchitectureDir) + "/**",
			Scheme:      "docs://",
			Category:    "architecture",
			Description: "Architecture documentation",
		},
		{
			Pattern:     filepath.ToSlash(paths.ProcessWorkstreamsDir) + "/**",
			Scheme:      "docs://",
			Category:    "workstreams",
			Description: "Workstream documentation",
		},
		{
			Pattern:     filepath.ToSlash(paths.ProcessPoliciesDir) + "/**",
			Scheme:      "docs://",
			Category:    string([]byte{'p', 'o', 'l', 'i', 'c', 'y'}),
			Description: "Policy documentation",
		},
		{
			Pattern:     "docs/onboarding/**",
			Scheme:      "docs://",
			Category:    "onboarding",
			Description: "Onboarding documentation",
		},
		{
			Pattern:     "docs/**",
			Scheme:      "file://",
			Category:    "documentation",
			Description: "General documentation",
		},
		// Default fallback is handled in ResolveURI
	}
}

// AddRule adds a custom rule to the resolver
func (r *ResourceURISchemeResolver) AddRule(rule ResourceURISchemeRule) {
	r.schemeRules = append(r.schemeRules, rule)
}

// SetRules replaces all rules with new ones
func (r *ResourceURISchemeResolver) SetRules(rules []ResourceURISchemeRule) {
	r.schemeRules = rules
}

// GetRules returns all current rules
func (r *ResourceURISchemeResolver) GetRules() []ResourceURISchemeRule {
	return r.schemeRules
}
