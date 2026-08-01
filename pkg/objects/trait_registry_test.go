package objects

import (
	"testing"
)

func TestTraitRegistry_IsValidTrait(t *testing.T) {
	t.Parallel()
	registry := NewTraitRegistry()

	tests := []struct {
		name     string
		trait    string
		expected bool
	}{
		{"standard trait", "listable", true},
		{"standard trait", "readable", true},
		{"standard trait", "writable", true},
		{"domain-specific trait", "constrainable", true},
		{"invalid trait", "invalid_trait", false},
		{"empty trait", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.IsValidTrait(tt.trait)
			if result != tt.expected {
				t.Errorf("IsValidTrait(%q) = %v, want %v", tt.trait, result, tt.expected)
			}
		})
	}
}

func TestTraitRegistry_ValidateTraits(t *testing.T) {
	t.Parallel()
	registry := NewTraitRegistry()

	tests := []struct {
		name          string
		traits        []string
		level         string
		expectedError bool
		errorCategory string
	}{
		{"valid standard traits", []string{"listable", "readable"}, "object", false, ""},
		{"valid with dependencies", []string{"writable", "readable"}, "object", false, ""},
		{"missing dependency", []string{"writable"}, "object", true, "missing_dependency"},
		{"invalid trait", []string{"invalid_trait"}, "object", true, "invalid"},
		{"duplicate trait", []string{"listable", "listable"}, "object", true, "duplicate"},
		{"field level trait", []string{"listable"}, "field", false, ""},
		{"field level with object-only trait", []string{"removable"}, "field", true, "level_mismatch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := registry.ValidateTraits(tt.traits, tt.level)
			hasError := len(errors) > 0

			if hasError != tt.expectedError {
				t.Errorf("ValidateTraits(%v, %q) returned errors = %v, want %v", tt.traits, tt.level, hasError, tt.expectedError)
				if hasError {
					for _, err := range errors {
						t.Logf("  Error: %s", err.Message)
					}
				}
			}

			if tt.expectedError && tt.errorCategory != emptyValue {
				found := false
				for _, err := range errors {
					if err.Category == tt.errorCategory {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error category %q, got errors: %v", tt.errorCategory, errors)
				}
			}
		})
	}
}

func TestTraitRegistry_ValidateTraitFieldConsistency(t *testing.T) {
	t.Parallel()
	registry := NewTraitRegistry()

	tests := []struct {
		name          string
		objectTraits  []string
		fieldTraits   []string
		fieldName     string
		expectedError bool
	}{
		{"consistent traits", []string{"listable", "readable"}, []string{"listable"}, "test_field", false},
		{"field trait not in object", []string{"readable"}, []string{"listable"}, "test_field", true},
		{"empty field traits", []string{"listable", "readable"}, []string{}, "test_field", false},
		{"multiple field traits", []string{"listable", "readable", "filterable"}, []string{"listable", "filterable"}, "test_field", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := registry.ValidateTraitFieldConsistency(tt.objectTraits, tt.fieldTraits, tt.fieldName)
			hasError := len(errors) > 0

			if hasError != tt.expectedError {
				t.Errorf("ValidateTraitFieldConsistency(%v, %v, %q) returned errors = %v, want %v",
					tt.objectTraits, tt.fieldTraits, tt.fieldName, hasError, tt.expectedError)
				if hasError {
					for _, err := range errors {
						t.Logf("  Error: %s", err.Message)
					}
				}
			}
		})
	}
}

func TestTraitRegistry_LoadTraitsFromDirectory(t *testing.T) {
	t.Parallel()
	registry := NewTraitRegistry()

	// Verify that traits were loaded (either from files or hardcoded)
	// Check for standard traits
	standardTraits := []string{"listable", "readable", "writable", "modifiable", "filterable", "sortable", "searchable"}
	for _, trait := range standardTraits {
		if !registry.IsValidTrait(trait) {
			t.Errorf("Expected trait %q to be loaded, but it's not valid", trait)
		}
	}

	// Check for domain-specific traits
	if !registry.IsValidTrait("constrainable") {
		t.Error("Expected trait 'constrainable' to be loaded, but it's not valid")
	}

	// Check for system traits (snapable)
	if !registry.IsValidTrait("snapable") {
		t.Error("Expected trait 'snapable' to be loaded, but it's not valid")
	}

	// Verify snapable has correct dependencies
	snapable, err := registry.GetTrait("snapable")
	if err != nil {
		t.Fatalf("Failed to get snapable trait: %v", err)
	}
	if !snapable.ObjectLevel || !snapable.FieldLevel {
		t.Error("snapable trait should be usable at both object and field levels")
	}
	if len(snapable.Requires) == 0 || snapable.Requires[0] != "readable" {
		t.Error("snapable trait should require 'readable' trait")
	}

	// Verify standard traits list
	standardList := registry.GetStandardTraits()
	if len(standardList) == 0 {
		t.Error("Expected standard traits list to be populated")
	}
}

func TestTraitRegistry_ExpandTraitGroup(t *testing.T) {
	t.Parallel()
	registry := NewTraitRegistry()

	tests := []struct {
		name           string
		traitName      string
		expectedTraits []string
		expectError    bool
	}{
		{
			name:      "expand base_auditable_traits",
			traitName: "base_auditable_traits",
			expectedTraits: []string{
				"listable", "readable", "writable", "modifiable",
				"removable", "formatable", "filterable", "sortable", "searchable",
			},
			expectError: false,
		},
		{
			name:      "expand base_object_traits (recursive)",
			traitName: "base_object_traits",
			expectedTraits: []string{
				"listable", "readable", "writable", "modifiable",
				"removable", "formatable", "filterable", "sortable", "searchable",
				"groupable",
				"auto_status_transitionable",
			},
			expectError: false,
		},
		{
			name:           "non-group trait returns itself",
			traitName:      "readable",
			expectedTraits: []string{"readable"},
			expectError:    false,
		},
		{
			name:        "invalid trait",
			traitName:   "invalid_trait",
			expectError: true,
		},
		{
			name:      "expand read_only_group",
			traitName: "read_only_group",
			expectedTraits: []string{
				"listable", "readable", "formatable", "groupable",
				"filterable", "sortable", "searchable",
				// Should NOT include: writable, modifiable, removable
			},
			expectError: false,
		},
		{
			name:      "expand confidential_group",
			traitName: "confidential_group",
			expectedTraits: []string{
				"listable", "readable", "writable", "modifiable",
				"removable", "formatable", "filterable", "sortable", "searchable",
				"groupable",
				"auto_status_transitionable",
			},
			expectError: false,
		},
		{
			name:      "expand field_immutable_group",
			traitName: "field_immutable_group",
			expectedTraits: []string{
				"readable", "listable", "filterable", "sortable", "searchable",
			},
			expectError: false,
		},
		{
			name:      "expand field_queryable_group",
			traitName: "field_queryable_group",
			expectedTraits: []string{
				"readable", "listable", "filterable", "sortable", "searchable",
			},
			expectError: false,
		},
		{
			name:      "expand field_mutable_group",
			traitName: "field_mutable_group",
			expectedTraits: []string{
				"readable", "writable", "modifiable",
			},
			expectError: false,
		},
		{
			name:      "expand field_reference_group",
			traitName: "field_reference_group",
			expectedTraits: []string{
				"readable", "filterable",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := registry.ExpandTraitGroup(tt.traitName)

			if tt.expectError {
				if err == nil {
					t.Errorf("ExpandTraitGroup(%q) expected error, got nil", tt.traitName)
				}
				return
			}

			if err != nil {
				t.Errorf("ExpandTraitGroup(%q) error = %v", tt.traitName, err)
				return
			}

			// Check that all expected traits are present
			expandedMap := make(map[string]bool)
			for _, trait := range expanded {
				expandedMap[trait] = true
			}

			for _, expected := range tt.expectedTraits {
				if !expandedMap[expected] {
					t.Errorf("ExpandTraitGroup(%q) missing expected trait %q. Got: %v",
						tt.traitName, expected, expanded)
				}
			}

			// Check that we have the right number of traits
			if len(expanded) != len(tt.expectedTraits) {
				t.Errorf("ExpandTraitGroup(%q) returned %d traits, want %d. Got: %v",
					tt.traitName, len(expanded), len(tt.expectedTraits), expanded)
			}
		})
	}
}

func TestTraitRegistry_ExpandTraits(t *testing.T) {
	t.Parallel()
	registry := NewTraitRegistry()

	tests := []struct {
		name           string
		traits         []string
		expectedTraits []string
		expectError    bool
	}{
		{
			name:   "expand trait group with specific traits",
			traits: []string{"base_object_traits", "snapable"},
			expectedTraits: []string{
				"listable", "readable", "writable", "modifiable",
				"removable", "formatable", "filterable", "sortable", "searchable",
				"groupable", "snapable",
			},
			expectError: false,
		},
		{
			name:           "regular traits unchanged",
			traits:         []string{"readable", "writable"},
			expectedTraits: []string{"readable", "writable"},
			expectError:    false,
		},
		{
			name:        "invalid trait in list",
			traits:      []string{"base_object_traits", "invalid_trait"},
			expectError: false, // Invalid traits are kept as-is, validation catches them later
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := registry.ExpandTraits(tt.traits)

			if tt.expectError {
				if err == nil {
					t.Errorf("ExpandTraits(%v) expected error, got nil", tt.traits)
				}
				return
			}

			if err != nil {
				t.Errorf("ExpandTraits(%v) error = %v", tt.traits, err)
				return
			}

			// Check that all expected traits are present
			expandedMap := make(map[string]bool)
			for _, trait := range expanded {
				expandedMap[trait] = true
			}

			for _, expected := range tt.expectedTraits {
				if !expandedMap[expected] {
					t.Errorf("ExpandTraits(%v) missing expected trait %q. Got: %v",
						tt.traits, expected, expanded)
				}
			}

			// Check that we have the right number of traits (allowing for invalid traits)
			if len(expanded) < len(tt.expectedTraits) {
				t.Errorf("ExpandTraits(%v) returned %d traits, want at least %d. Got: %v",
					tt.traits, len(expanded), len(tt.expectedTraits), expanded)
			}
		})
	}
}

func TestExtractFieldTraits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		fieldDef    map[string]any
		expected    []string
		expectedLen int
	}{
		{"traits as list", map[string]any{"traits": []any{"listable", "readable"}}, []string{"listable", "readable"}, 2},
		{"no traits", map[string]any{FieldKeyType: "string"}, []string{}, 0},
		{"traits as strings", map[string]any{"traits": []any{"writable", "modifiable"}}, []string{"writable", "modifiable"}, 2},
		{"empty traits", map[string]any{"traits": []any{}}, []string{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFieldTraits(tt.fieldDef)
			if len(result) != tt.expectedLen {
				t.Errorf("ExtractFieldTraits() returned %d traits, want %d", len(result), tt.expectedLen)
			}
			if len(result) > 0 {
				for i, expected := range tt.expected {
					if i < len(result) && result[i] != expected {
						t.Errorf("ExtractFieldTraits()[%d] = %q, want %q", i, result[i], expected)
					}
				}
			}
		})
	}
}
