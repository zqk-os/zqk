package cli

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ValidateCommandTraits validates that an object kind has the required traits for a command
// This enables early validation before attempting operations
func ValidateCommandTraits(spec *CommandSpec, kind string) error {
	if spec == nil {
		return nil // No spec, no validation needed
	}

	// Get trait registry (create new instance)
	registry := objects.NewTraitRegistry()
	if registry == nil {
		return nil // No registry available, skip validation
	}

	// Get object spec to check traits
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		return nil // No spec loader, skip validation
	}

	// Load object spec (use LoadSpecWithInheritance to get resolved traits)
	objSpec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		// Object spec not found - this is handled elsewhere, just skip trait validation
		return nil
	}

	// Expand object traits (handles trait groups)
	expandedTraits, err := registry.ExpandTraits(objSpec.ResolvedTraits)
	if err != nil {
		return errfmt.Errorf("failed to expand traits for kind %s: %w", kind, err)
	}

	// Build trait map for quick lookup
	traitMap := make(map[string]bool)
	for _, trait := range expandedTraits {
		traitMap[trait] = true
	}

	// Check required traits
	for _, requiredTrait := range spec.RequiredTraits {
		if !traitMap[requiredTrait] {
			return errfmt.Errorf("object kind '%s' does not have required trait '%s' for command '%s'", kind, requiredTrait, spec.Name)
		}
	}

	// Check required trait groups
	for _, traitGroup := range spec.RequiredTraitGroups {
		// Expand trait group
		groupTraits, err := registry.ExpandTraitGroup(traitGroup)
		if err != nil {
			return errfmt.Errorf("failed to expand trait group '%s': %w", traitGroup, err)
		}

		// Check if all traits in the group are present
		for _, groupTrait := range groupTraits {
			if !traitMap[groupTrait] {
				return errfmt.Errorf("object kind '%s' does not have trait '%s' from required trait group '%s' for command '%s'", kind, groupTrait, traitGroup, spec.Name)
			}
		}
	}

	return nil
}

// ValidateCommandTraitsWithFlags validates traits including conditional traits based on flags
func ValidateCommandTraitsWithFlags(spec *CommandSpec, kind string, flags map[string]any) error {
	// First validate base required traits
	if err := ValidateCommandTraits(spec, kind); err != nil {
		return err
	}

	if spec == nil || len(spec.ConditionalTraits) == 0 {
		return nil // No conditional traits to check
	}

	// Get trait registry (create new instance)
	registry := objects.NewTraitRegistry()
	if registry == nil {
		return nil // No registry available, skip validation
	}

	// Get object spec to check traits
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		return nil // No spec loader, skip validation
	}

	// Load object spec (use LoadSpecWithInheritance to get resolved traits)
	objSpec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		// Object spec not found - this is handled elsewhere, just skip trait validation
		return nil
	}

	// Expand object traits
	expandedTraits, err := registry.ExpandTraits(objSpec.ResolvedTraits)
	if err != nil {
		return errfmt.Errorf("failed to expand traits for kind %s: %w", kind, err)
	}

	// Build trait map for quick lookup
	traitMap := make(map[string]bool)
	for _, trait := range expandedTraits {
		traitMap[trait] = true
	}

	// Check conditional traits
	for _, condTrait := range spec.ConditionalTraits {
		// Check if the flag is set (non-empty/non-zero value)
		flagValue, flagSet := flags[condTrait.Flag]
		if !flagSet {
			continue // Flag not set, skip this conditional trait
		}

		// Check if flag has a meaningful value
		hasValue := false
		switch v := flagValue.(type) {
		case string:
			hasValue = v != emptyValue
		case []string:
			hasValue = len(v) > 0
		case bool:
			hasValue = v
		case int:
			hasValue = v != 0
		default:
			hasValue = flagValue != nil
		}

		if !hasValue {
			continue // Flag set but empty/zero, skip
		}

		// Flag is set and has value, check required traits
		for _, requiredTrait := range condTrait.Traits {
			if !traitMap[requiredTrait] {
				return errfmt.Errorf("object kind '%s' does not have required trait '%s' for command '%s' with flag '--%s'", kind, requiredTrait, spec.Name, condTrait.Flag)
			}
		}
	}

	return nil
}

// GetCommandTraitRequirements returns all trait requirements for a command (including conditional)
func GetCommandTraitRequirements(spec *CommandSpec) []string {
	if spec == nil {
		return nil
	}

	traits := make(map[string]bool)

	// Add required traits
	for _, trait := range spec.RequiredTraits {
		traits[trait] = true
	}

	// Add conditional traits
	for _, condTrait := range spec.ConditionalTraits {
		for _, trait := range condTrait.Traits {
			traits[trait] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(traits))
	for trait := range traits {
		result = append(result, trait)
	}

	return result
}
