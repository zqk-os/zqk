package objects

import (
	"context"
	"maps"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/kindsynonyms"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/logging"
)

// SynonymLoader is an interface for loading synonyms from storage
// This allows the resolver to work without directly importing storage
type SynonymLoader interface {
	LoadSynonyms() ([]SynonymData, error)
}

// SynonymData represents a single synonym definition
type SynonymData struct {
	Kind     string
	Synonym  string
	Priority int
}

// SynonymProvider is an interface for providing synonym mappings from config
// This allows KindSynonymResolver to use config-based synonyms without importing validation package
type SynonymProvider interface {
	GetSynonymsForKind(kind string) []string
}

// KindSynonymResolver manages synonyms/aliases for object kinds.
// Synonyms are loaded from kind_synonym objects stored in the system.
// Initialize uses the component loader pattern (pkg/loader) for wait-for-completion and timeouts.
type KindSynonymResolver struct {
	synonymToKind     map[string]string
	synonymToPriority map[string]int
	kindToSynonyms    map[string][]string
	synonymLoader     SynonymLoader
	synonymProvider   SynonymProvider
	mu                sync.RWMutex
	initialized       bool
	runner            *loader.Runner
	runnerOnce        sync.Once
}

var (
	globalSynonymResolver     *KindSynonymResolver
	globalSynonymResolverOnce sync.Once
)

// GetGlobalSynonymResolver returns the singleton instance of KindSynonymResolver
// The resolver is automatically configured with a SynonymProvider if available
func GetGlobalSynonymResolver() *KindSynonymResolver {
	globalSynonymResolverOnce.Do(func() {
		globalSynonymResolver = NewKindSynonymResolver()
		// Note: SynonymProvider is set lazily via SetSynonymProvider
		// This avoids import cycles - callers in pkg/validation can set it up
	})
	return globalSynonymResolver
}

// NewKindSynonymResolver creates a new synonym resolver
func NewKindSynonymResolver() *KindSynonymResolver {
	return &KindSynonymResolver{
		synonymToKind:     make(map[string]string),
		synonymToPriority: make(map[string]int),
		kindToSynonyms:    make(map[string][]string),
		initialized:       false,
	}
}

// SetSynonymProvider sets the provider for config-based synonyms
// This allows the resolver to use standardized synonym mappings from IDPrefixesConfig
func (ksr *KindSynonymResolver) SetSynonymProvider(provider SynonymProvider) {
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.RunInLockWithLogger(
		&ksr.mu,
		LockNameSynonymResolverSetProvider,
		logger,
		func() error {
			ksr.synonymProvider = provider
			return nil
		},
	)
}

// SetSynonymLoader sets the loader for loading synonyms from storage
// If the resolver was already initialized with generated synonyms, this will
// reset the state so that synonyms can be reloaded from storage on next Initialize() call.
func (ksr *KindSynonymResolver) SetSynonymLoader(loader SynonymLoader) {
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.RunInLockWithLogger(
		&ksr.mu,
		LockNameSynonymResolverSetLoader,
		logger,
		func() error {
			// If we're setting a loader and were previously initialized without one,
			// reset the state so synonyms can be reloaded from storage
			if loader != nil && ksr.initialized && ksr.synonymLoader == nil {
				// Clear existing synonym data to allow reload from storage
				ksr.synonymToKind = make(map[string]string)
				ksr.synonymToPriority = make(map[string]int)
				ksr.kindToSynonyms = make(map[string][]string)
				ksr.initialized = false
				ksr.getRunner().ResetLoaded()
			}

			ksr.synonymLoader = loader
			return nil
		},
	)
}

// getRunner returns the shared loader.Runner for this resolver (lazily created).
func (ksr *KindSynonymResolver) getRunner() *loader.Runner {
	ksr.runnerOnce.Do(func() {
		ksr.runner = loader.NewRunner("kind_synonym_resolver", func(ctx context.Context) error {
			return ksr.doInitialize(ctx)
		})
	})
	return ksr.runner
}

// Initialize loads synonyms from kind_synonym objects in storage via the component loader pattern.
func (ksr *KindSynonymResolver) Initialize() error {
	return ksr.getRunner().Load(pkgctx.NewSystemContext())
}

// doInitialize performs the actual load; called by loader.Runner.
func (ksr *KindSynonymResolver) doInitialize(ctx context.Context) error { //nolint:unparam // ctx for LoadFn signature; future timeout use
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	var loader SynonymLoader
	_ = concurrency.RunInRLockWithLogger(
		&ksr.mu, LockNameSynonymResolverGetLoader, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			loader = ksr.synonymLoader
			return nil
		},
	)

	if loader != nil {
		synonymData, err := loader.LoadSynonyms()
		if err == nil && len(synonymData) > 0 {
			// Track synonyms per target_kind to ensure uniqueness per target_kind
			// Map: target_kind -> synonym -> priority
			kindSynonymMap := make(map[string]map[string]int)
			// Map: synonym -> kind -> priority (for cross-kind conflict resolution)
			synonymMap := make(map[string]map[string]int)

			for _, data := range synonymData {
				if data.Kind == emptyValue || data.Synonym == emptyValue {
					continue
				}

				// Normalize synonym (lowercase)
				normalizedSynonym := strings.ToLower(data.Synonym)

				// Initialize maps if needed
				if kindSynonymMap[data.Kind] == nil {
					kindSynonymMap[data.Kind] = make(map[string]int)
				}
				if synonymMap[normalizedSynonym] == nil {
					synonymMap[normalizedSynonym] = make(map[string]int)
				}

				// For duplicates within the same target_kind, keep the highest priority
				if existingPriority, exists := kindSynonymMap[data.Kind][normalizedSynonym]; exists {
					if data.Priority > existingPriority {
						kindSynonymMap[data.Kind][normalizedSynonym] = data.Priority
						synonymMap[normalizedSynonym][data.Kind] = data.Priority
					}
					// Skip adding duplicate to list (already exists with same or higher priority)
					continue
				}

				// First occurrence of this synonym for this target_kind
				kindSynonymMap[data.Kind][normalizedSynonym] = data.Priority
				synonymMap[normalizedSynonym][data.Kind] = data.Priority
			}

			// Build mappings outside lock (copy-out/process pattern)
			synonymToKind := make(map[string]string)
			synonymToPriority := make(map[string]int)
			kindToSynonyms := make(map[string][]string)

			// Resolve cross-kind conflicts by priority (higher priority wins)
			// If the same synonym maps to multiple kinds, the highest priority wins
			for synonym, kindPriorities := range synonymMap {
				bestKind := ""
				bestPriority := -1

				for kind, priority := range kindPriorities {
					if priority > bestPriority {
						bestPriority = priority
						bestKind = kind
					}
				}

				if bestKind != emptyValue {
					synonymToKind[synonym] = bestKind
					synonymToPriority[synonym] = bestPriority
				}
			}

			// Build kindToSynonyms from kindSynonymMap
			for kind, synonyms := range kindSynonymMap {
				synonymList := make([]string, 0, len(synonyms))
				for synonym := range synonyms {
					synonymList = append(synonymList, synonym)
				}
				kindToSynonyms[kind] = synonymList
			}

			// Copy-in: Update cache with lock
			err = concurrency.RunInLockWithLogger(
				&ksr.mu,
				LockNameSynonymResolverInitUpdate,
				logger,
				func() error {
					// Double-check after re-acquiring lock
					if ksr.initialized {
						return nil
					}
					// Update mappings
					maps.Copy(ksr.synonymToKind, synonymToKind)
					maps.Copy(ksr.synonymToPriority, synonymToPriority)
					maps.Copy(ksr.kindToSynonyms, kindToSynonyms)
					ksr.initialized = true
					return nil
				},
			)
			if err != nil {
				return err
			}
			return nil
		}
	}

	// Fall back to generated synonyms if no loader or loading failed
	return ksr.initializeWithGeneratedSynonyms()
}

// initializeWithGeneratedSynonyms generates synonyms using conventions when storage is unavailable
func (ksr *KindSynonymResolver) initializeWithGeneratedSynonyms() error {
	// Get all known kinds from the kind mapper (I/O outside lock)
	mapper := GetGlobalKindMapper()
	if err := mapper.Initialize(); err != nil {
		return errfmt.Newf("failed to initialize kind mapper").Wrap(err)
	}

	allKinds := mapper.GetAllKinds()

	// Generate synonyms for each kind (process outside lock)
	kindToSynonyms := make(map[string][]string)
	synonymToKind := make(map[string]string)
	synonymToPriority := make(map[string]int)

	for _, kind := range allKinds {
		synonyms := ksr.generateSynonyms(kind)
		kindToSynonyms[kind] = synonyms

		// Map each synonym to the canonical kind (first one wins for conflicts)
		for _, synonym := range synonyms {
			normalized := strings.ToLower(synonym)
			if _, exists := synonymToKind[normalized]; !exists {
				synonymToKind[normalized] = kind
				synonymToPriority[normalized] = 0 // Default priority
			}
		}
	}

	// Copy-in: Update cache with lock
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	err := concurrency.RunInLockWithLogger(
		&ksr.mu,
		LockNameSynonymResolverInitGenerated,
		logger,
		func() error {
			// Double-check after re-acquiring lock
			if ksr.initialized {
				return nil
			}
			// Update mappings
			maps.Copy(ksr.kindToSynonyms, kindToSynonyms)
			maps.Copy(ksr.synonymToKind, synonymToKind)
			maps.Copy(ksr.synonymToPriority, synonymToPriority)
			ksr.initialized = true
			return nil
		},
	)
	return err
}

// generateSynonyms generates synonyms for a given kind based on conventions
// Uses SynonymProvider for standardized mappings (config-based), with minimal algorithmic fallback
// All explicit mappings should be in config - algorithmic generation is only for truly generic patterns
func (ksr *KindSynonymResolver) generateSynonyms(kind string) []string {
	synonyms := []string{}

	// Always include the canonical name
	synonyms = append(synonyms, kind)

	// Baseline aliases shared with AliasRegistry so common CLI workflows match.
	synonyms = append(synonyms, kindsynonyms.DefaultKindAliasesForKind(kind)...)

	// Get synonyms from config provider (standardized approach)
	// This includes both explicit mappings and pattern-based inference
	if ksr.synonymProvider != nil {
		if configSynonyms := ksr.synonymProvider.GetSynonymsForKind(kind); len(configSynonyms) > 0 {
			synonyms = append(synonyms, configSynonyms...)
		}
	}

	// Minimal algorithmic fallback - only if config provided no synonyms
	// This ensures new kinds get basic synonyms even without explicit config
	if len(synonyms) == 1 { // Only canonical name, no config synonyms
		// Generic algorithmic patterns (no hardcoded cases)
		if strings.Contains(kind, "_") {
			parts := strings.Split(kind, "_")
			nonEmptyParts := make([]string, 0, len(parts))
			for _, part := range parts {
				if part != emptyValue {
					nonEmptyParts = append(nonEmptyParts, part)
				}
			}

			if len(nonEmptyParts) == 2 && len(nonEmptyParts[0]) > 0 && len(nonEmptyParts[1]) > 0 {
				// Two-word: first letter of each word
				abbrev := string(nonEmptyParts[0][0]) + string(nonEmptyParts[1][0])
				synonyms = append(synonyms, abbrev)
			} else if len(nonEmptyParts) > 2 {
				// Multi-word: first letter of each word
				abbrev := ""
				for _, part := range nonEmptyParts {
					if len(part) > 0 {
						abbrev += string(part[0])
					}
				}
				if abbrev != emptyValue {
					synonyms = append(synonyms, abbrev)
				}
			}
		} else {
			// Single-word: first 4 letters
			if len(kind) >= 4 {
				synonyms = append(synonyms, kind[:4])
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := []string{}
	for _, s := range synonyms {
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}

	return unique
}

// ResolveKind resolves a synonym/alias to the canonical kind name
// Returns the canonical kind if found, or the input string if not found
func (ksr *KindSynonymResolver) ResolveKind(input string) string {
	// Ensure initialized
	if !ksr.initialized {
		if err := ksr.Initialize(); err != nil {
			// If initialization fails, return input as-is
			return input
		}
	}

	var result string
	_ = concurrency.RunInRLockWithLogger(
		&ksr.mu, LockNameKindSynonymsResolve, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Normalize input (lowercase, trim)
			normalized := strings.ToLower(strings.TrimSpace(input))

			// Check if it's a synonym
			if kind, exists := ksr.synonymToKind[normalized]; exists {
				result = kind
				return nil
			}

			// Check if it's already a canonical kind (case-insensitive)
			for kind := range ksr.kindToSynonyms {
				if strings.EqualFold(kind, normalized) {
					result = kind
					return nil
				}
			}

			// Not found - return input as-is (let validation handle it)
			result = input
			return nil
		},
	)
	return result
}

// GetSynonyms returns all synonyms for a given canonical kind
func (ksr *KindSynonymResolver) GetSynonyms(kind string) []string {
	var synonyms []string
	_ = concurrency.RunInRLockWithLogger(
		&ksr.mu, LockNameSynonymResolverGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			synonyms = ksr.kindToSynonyms[kind]
			return nil
		},
	)

	return synonyms
}

// GetAllSynonyms returns a map of all canonical kinds to their synonyms
func (ksr *KindSynonymResolver) GetAllSynonyms() map[string][]string {
	var result map[string][]string
	_ = concurrency.RunInRLockWithLogger(
		&ksr.mu, LockNameSynonymResolverGetAll, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			result = make(map[string][]string)
			for kind, synonyms := range ksr.kindToSynonyms {
				result[kind] = make([]string, len(synonyms))
				copy(result[kind], synonyms)
			}
			return nil
		},
	)
	return result
}

// ValidateUniqueness checks that all synonyms are unique across all kinds
// Returns a map of conflicting synonyms to their conflicting kinds
func (ksr *KindSynonymResolver) ValidateUniqueness() map[string][]string {
	var kindToSynonymsCopy map[string][]string
	_ = concurrency.RunInRLockWithLogger(
		&ksr.mu, LockNameSynonymResolverValidateCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			kindToSynonymsCopy = make(map[string][]string)
			for k, v := range ksr.kindToSynonyms {
				kindToSynonymsCopy[k] = v
			}
			return nil
		},
	)

	conflicts := make(map[string][]string)
	synonymToKinds := make(map[string][]string)

	// Build reverse map: synonym -> []kinds
	for kind, synonyms := range kindToSynonymsCopy {
		for _, synonym := range synonyms {
			normalized := strings.ToLower(synonym)
			synonymToKinds[normalized] = append(synonymToKinds[normalized], kind)
		}
	}

	// Find conflicts
	for synonym, kinds := range synonymToKinds {
		if len(kinds) > 1 {
			conflicts[synonym] = kinds
		}
	}

	return conflicts
}

// GetCanonicalKind returns the canonical kind name for a given input
// This is an alias for ResolveKind for clarity
func GetCanonicalKind(input string) string {
	resolver := GetGlobalSynonymResolver()
	if err := resolver.Initialize(); err != nil {
		// If initialization fails, return input as-is
		return input
	}
	return resolver.ResolveKind(input)
}
