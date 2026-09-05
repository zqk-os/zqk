package objects

import "sync"

// lookupTraitRegistry returns the process-wide registry backing trait lookups.
//
// Constructing a registry per lookup made each call re-read every file in the traits
// directory (and spawn a goroutine plus a 5s timer to do it): a `system check` over ~8k
// objects spent 29s of its 73s inside loadTraitFromFile. Trait definitions are static for
// the life of the process and nothing on this path mutates the registry, so one instance
// serves every reader. Sharing is safe without a mutex because sync.Once establishes
// happens-before between the single initialization and every later read — callers that
// need a mutable registry must keep using NewTraitRegistry.
func lookupTraitRegistry() *TraitRegistry {
	lookupTraitRegistryOnce.Do(func() { lookupTraitRegistryShared = NewTraitRegistry() })
	return lookupTraitRegistryShared
}

var (
	lookupTraitRegistryShared *TraitRegistry
	lookupTraitRegistryOnce   sync.Once
)

// KindHasTrait returns true if a kind has the given trait after inheritance,
// trait-group expansion, and Includes composition (effort_aware confers completable;
// satisfiable does not).
func KindHasTrait(kind, trait string) (bool, error) {
	if kind == emptyValue || trait == emptyValue {
		return false, nil
	}
	specLoader := GetGlobalSpecLoader()
	spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		return false, err
	}
	for _, excluded := range spec.ExcludeTraits {
		if excluded == trait {
			return false, nil
		}
	}
	expanded, err := lookupTraitRegistry().ExpandTraits(spec.ResolvedTraits)
	if err != nil {
		return false, err
	}
	for _, t := range expanded {
		if t == trait {
			return true, nil
		}
	}
	return false, nil
}

// KindHasNamedTrait is KindHasTrait with errors treated as false (unknown kind,
// missing spec). Use on hot paths that must not fail closed into a kind switch.
func KindHasNamedTrait(kind, trait string) bool {
	ok, err := KindHasTrait(kind, trait)
	return err == nil && ok
}
