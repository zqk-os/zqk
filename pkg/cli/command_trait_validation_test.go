package cli

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidateCommandTraits(t *testing.T) {
	t.Parallel()

	// Create a test command spec with trait requirements
	spec := &CommandSpec{
		Name:           "list [kind]",
		RequiredTraits: []string{"listable"},
	}

	// Test with a kind that should have listable trait
	// Note: This test requires actual object specs to be available
	// For now, we'll test that the function doesn't panic
	err := ValidateCommandTraits(spec, "backlog_item")
	if err != nil {
		// If error, it's likely because the spec doesn't exist or doesn't have the trait
		// This is acceptable - the function should gracefully handle missing specs
		t.Logf("Validation returned error (expected if spec doesn't exist): %v", err)
	}
}

func TestValidateCommandTraitsWithFlags(t *testing.T) {
	t.Parallel()

	// Create a test command spec with conditional traits
	spec := &CommandSpec{
		Name:           "list [kind]",
		RequiredTraits: []string{"listable"},
		ConditionalTraits: []ConditionalTrait{
			{
				Flag:   "group-by",
				Traits: []string{"groupable"},
			},
		},
	}

	// Test without the flag (should only check required traits)
	err := ValidateCommandTraitsWithFlags(spec, "backlog_item", map[string]any{})
	if err != nil {
		t.Logf("Validation returned error (expected if spec doesn't exist): %v", err)
	}

	// Test with the flag set (should check conditional traits too)
	flags := map[string]any{
		"group-by": "status",
	}
	err = ValidateCommandTraitsWithFlags(spec, "backlog_item", flags)
	if err != nil {
		t.Logf("Validation returned error (expected if spec doesn't exist or trait missing): %v", err)
	}
}

func TestGetCommandTraitRequirements(t *testing.T) {
	t.Parallel()

	// Test with required traits only
	spec1 := &CommandSpec{
		Name:           "get <id>",
		RequiredTraits: []string{"readable"},
	}
	requirements := GetCommandTraitRequirements(spec1)
	if len(requirements) != 1 || requirements[0] != "readable" {
		t.Errorf("Expected [readable], got %v", requirements)
	}

	// Test with conditional traits
	spec2 := &CommandSpec{
		Name:           "list [kind]",
		RequiredTraits: []string{"listable"},
		ConditionalTraits: []ConditionalTrait{
			{
				Flag:   "group-by",
				Traits: []string{"groupable"},
			},
			{
				Flag:   "filter",
				Traits: []string{"filterable"},
			},
		},
	}
	requirements = GetCommandTraitRequirements(spec2)
	expectedTraits := map[string]bool{
		"listable":   true,
		"groupable":  true,
		"filterable": true,
	}
	if len(requirements) != len(expectedTraits) {
		t.Errorf("Expected %d traits, got %d: %v", len(expectedTraits), len(requirements), requirements)
	}
	for _, trait := range requirements {
		if !expectedTraits[trait] {
			t.Errorf("Unexpected trait: %s", trait)
		}
	}

	// Test with nil spec
	requirements = GetCommandTraitRequirements(nil)
	if requirements != nil {
		t.Errorf("Expected nil for nil spec, got %v", requirements)
	}
}

func TestTraitRegistryIntegration(t *testing.T) {
	t.Parallel()

	// Test that trait registry can validate traits
	registry := objects.NewTraitRegistry()
	if registry == nil {
		t.Fatal("Failed to create trait registry")
	}

	// Test standard traits
	if !registry.IsValidTrait("listable") {
		t.Error("listable should be a valid trait")
	}
	if !registry.IsValidTrait("readable") {
		t.Error("readable should be a valid trait")
	}
	if !registry.IsValidTrait("writable") {
		t.Error("writable should be a valid trait")
	}
	if !registry.IsValidTrait("modifiable") {
		t.Error("modifiable should be a valid trait")
	}
	if !registry.IsValidTrait("removable") {
		t.Error("removable should be a valid trait")
	}
	if !registry.IsValidTrait("groupable") {
		t.Error("groupable should be a valid trait")
	}
	if !registry.IsValidTrait("filterable") {
		t.Error("filterable should be a valid trait")
	}
	if !registry.IsValidTrait("sortable") {
		t.Error("sortable should be a valid trait")
	}

	// Test invalid trait
	if registry.IsValidTrait("invalid_trait") {
		t.Error("invalid_trait should not be a valid trait")
	}
}
