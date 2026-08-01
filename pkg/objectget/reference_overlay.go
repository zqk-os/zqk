package objectget

// ReferenceResolverOverlayConfig controls how object get (and other surfaces) resolve *_ref / *_refs
// fields into resolved_<field> embeds. Shared by CLI, MCP, hooks, and scripts so hydration policy
// stays consistent.
type ReferenceResolverOverlayConfig struct {
	// ResolveFieldKeys restricts which reference fields (e.g. criteria_refs, goal_refs)
	// should be resolved into resolved_<field> slices.
	// If nil/empty, the overlay resolves all *_ref / *_refs fields present on the object.
	ResolveFieldKeys []string

	// MaxDepth is the maximum recursion depth for resolving references found on referenced objects.
	// depth=1 means: resolve only references on the root object.
	MaxDepth int

	// Caps for worst-case safety.
	MaxTotalUniqueIDs     int
	MaxTotalResolvedEntry int
}

const (
	// DefaultReferenceResolverMaxDepth keeps object get responsive on branching reference graphs.
	DefaultReferenceResolverMaxDepth = 2
	// DefaultReferenceResolverMaxTotalUniqueIDs caps unique objects touched in one request.
	DefaultReferenceResolverMaxTotalUniqueIDs = 300
	// DefaultReferenceResolverMaxTotalResolvedEntry caps total resolved_* entries injected.
	DefaultReferenceResolverMaxTotalResolvedEntry = 2000
)

// NormalizeReferenceOverlayConfig applies defaults for zero fields (snappy but safe).
func NormalizeReferenceOverlayConfig(cfg ReferenceResolverOverlayConfig) ReferenceResolverOverlayConfig {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = DefaultReferenceResolverMaxDepth
	}
	if cfg.MaxTotalUniqueIDs <= 0 {
		cfg.MaxTotalUniqueIDs = DefaultReferenceResolverMaxTotalUniqueIDs
	}
	if cfg.MaxTotalResolvedEntry <= 0 {
		cfg.MaxTotalResolvedEntry = DefaultReferenceResolverMaxTotalResolvedEntry
	}
	return cfg
}
