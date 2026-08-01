package objects

const emptyTraitValue = ""

// KindHasTrait returns true if a kind has the given trait after inheritance and
// trait-group expansion (e.g., base_object_traits includes auto_status_transitionable).
func KindHasTrait(kind, trait string) (bool, error) {
	if kind == emptyTraitValue || trait == emptyTraitValue {
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
	reg := NewTraitRegistry()
	expanded, err := reg.ExpandTraits(spec.ResolvedTraits)
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
