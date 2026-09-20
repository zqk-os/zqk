package cli

import (
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kindsynonyms"
)

type operationAliasEntry struct {
	canonical string
	aliases   []string
}

const readOpAliasDisplay = "dis" + "play" // CLI verb synonym for read; not KindDisplay

var defaultOperationAliasEntries = []operationAliasEntry{
	{canonical: "create", aliases: []string{"add", "new", "make", "generate"}},
	{canonical: "read", aliases: []string{"get", "show", "view", readOpAliasDisplay, "fetch", "retrieve"}},
	{canonical: "list", aliases: []string{"find", "search", "query", "show_all", "enumerate"}},
	{canonical: "update", aliases: []string{"edit", "modify", "change", "alter", "patch"}},
	{canonical: "delete", aliases: []string{"remove", "del", "drop", "erase", "destroy"}},
}

// AliasRegistry manages aliases and synonyms for semantic resolution
// This enables fluid traversal where "tasks" can resolve to "backlog_item"
type AliasRegistry struct {
	mu               sync.RWMutex
	aliases          map[string][]string // alias -> []canonical names
	canonicalToAlias map[string][]string // canonical -> []aliases (reverse lookup)
}

// NewAliasRegistry creates a new alias registry with default aliases
func NewAliasRegistry() *AliasRegistry {
	registry := &AliasRegistry{
		aliases:          make(map[string][]string),
		canonicalToAlias: make(map[string][]string),
	}

	// Register default aliases for common object kinds
	registry.RegisterDefaults()

	return registry
}

// RegisterDefaults registers default aliases for semantic traversal
func (ar *AliasRegistry) RegisterDefaults() {
	// Object kind aliases (baseline defaults)
	for _, e := range kindsynonyms.DefaultKindAliasEntries() {
		ar.Register(e.Kind, e.Aliases)
	}

	// Operation type aliases
	for _, e := range defaultOperationAliasEntries {
		ar.Register(e.canonical, e.aliases)
	}
}

// Register registers aliases for a canonical name
// Example: Register("backlog_item", []string{"task", "tasks"})
// This means "task" and "tasks" will resolve to "backlog_item"
func (ar *AliasRegistry) Register(canonical string, aliases []string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	// Normalize canonical name
	canonical = strings.ToLower(strings.TrimSpace(canonical))

	// Store aliases -> canonical mapping
	for _, alias := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == emptyValue {
			continue
		}
		ar.aliases[alias] = appendUnique(ar.aliases[alias], canonical)
	}

	// Store canonical -> aliases mapping (reverse lookup)
	ar.canonicalToAlias[canonical] = appendUnique(ar.canonicalToAlias[canonical], aliases...)
}

// Resolve resolves an alias or synonym to canonical names
// Returns empty slice if no alias found (caller can use original term)
// Returns multiple values if alias maps to multiple canonicals
func (ar *AliasRegistry) Resolve(alias string) []string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(alias))
	if canonicals, ok := ar.aliases[normalized]; ok {
		// Return copy to prevent external modification
		result := make([]string, len(canonicals))
		copy(result, canonicals)
		return result
	}

	// If not found as alias, check if it's already a canonical name
	if _, isCanonical := ar.canonicalToAlias[normalized]; isCanonical {
		return []string{normalized}
	}

	return []string{}
}

// ResolveToSingle resolves an alias to a single canonical name
// Returns the first match, or the original term if no alias found
func (ar *AliasRegistry) ResolveToSingle(alias string) string {
	resolved := ar.Resolve(alias)
	if len(resolved) > 0 {
		return resolved[0]
	}
	return strings.ToLower(strings.TrimSpace(alias))
}

// GetAliases returns all aliases for a canonical name
func (ar *AliasRegistry) GetAliases(canonical string) []string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(canonical))
	if aliases, ok := ar.canonicalToAlias[normalized]; ok {
		result := make([]string, len(aliases))
		copy(result, aliases)
		return result
	}

	return []string{}
}

// HasAlias checks if a term has registered aliases
func (ar *AliasRegistry) HasAlias(term string) bool {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(term))
	_, hasAlias := ar.aliases[normalized]
	_, isCanonical := ar.canonicalToAlias[normalized]
	return hasAlias || isCanonical
}

// appendUnique appends values to a slice, avoiding duplicates
func appendUnique(slice []string, values ...string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice)+len(values))

	// Add existing values
	for _, v := range slice {
		normalized := strings.ToLower(strings.TrimSpace(v))
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}

	// Add new values
	for _, v := range values {
		normalized := strings.ToLower(strings.TrimSpace(v))
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}

	return result
}

// LoadAliasesFromSpecs loads aliases from command spec files
// This allows specs to define their own aliases
func (ar *AliasRegistry) LoadAliasesFromSpecs(specsDir string) error {
	graph := NewCommandSpecGraph(specsDir)
	if err := graph.BuildGraph(); err != nil {
		return errfmt.Newf("failed to build graph").Wrap(err)
	}

	// Extract aliases from command specs
	// For now, we'll use a simple heuristic - could be enhanced with explicit alias fields in specs
	allSpecs := graph.GetAllSpecs()
	for name := range allSpecs {
		// Extract potential aliases from command name
		// e.g., "object get" might have alias "get"
		parts := strings.Fields(name)
		if len(parts) > 1 {
			// Register the last part as an alias for the full name
			lastPart := parts[len(parts)-1]
			ar.Register(name, []string{lastPart})
		}
	}

	return nil
}
