package objects

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// GenerateFieldHelp generates help text for fields of a specific object kind
// Groups fields into common (inherited) and specialized sections
func GenerateFieldHelp(kind string) (string, error) {
	registry := GetGlobalFieldRegistry()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return "", errfmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}

	var help strings.Builder
	fmt.Fprintf(&help, "Available fields for '%s':\n\n", kind)

	// Common fields (inherited from base_object/auditable)
	if len(kindFields.CommonFields) > 0 {
		help.WriteString("Common Fields (inherited by all objects):\n")
		for i := range kindFields.CommonFields {
			field := &kindFields.CommonFields[i]
			fmt.Fprintf(&help, "  • %s", field.Name)
			if field.Type != emptyValue {
				fmt.Fprintf(&help, " (%s)", field.Type)
			}
			if field.Required {
				help.WriteString(" [required]")
			}
			help.WriteString("\n")
			if field.Description != emptyValue {
				fmt.Fprintf(&help, "    %s\n", field.Description)
			}
			if len(field.EnumValues) > 0 {
				fmt.Fprintf(&help, "    Valid values: %s\n", strings.Join(field.EnumValues, ", "))
			}
			if len(field.Traits) > 0 {
				fmt.Fprintf(&help, "    Traits: %s\n", strings.Join(field.Traits, ", "))
			}
			help.WriteString("\n")
		}
	}

	// Specialized fields
	if len(kindFields.SpecializedFields) > 0 {
		help.WriteString("Specialized Fields (specific to this kind):\n")
		for i := range kindFields.SpecializedFields {
			field := &kindFields.SpecializedFields[i]
			fmt.Fprintf(&help, "  • %s", field.Name)
			if field.Type != emptyValue {
				fmt.Fprintf(&help, " (%s)", field.Type)
			}
			if field.Required {
				help.WriteString(" [required]")
			}
			help.WriteString("\n")
			if field.Description != emptyValue {
				fmt.Fprintf(&help, "    %s\n", field.Description)
			}
			if len(field.EnumValues) > 0 {
				fmt.Fprintf(&help, "    Valid values: %s\n", strings.Join(field.EnumValues, ", "))
			}
			if len(field.Traits) > 0 {
				fmt.Fprintf(&help, "    Traits: %s\n", strings.Join(field.Traits, ", "))
			}
			help.WriteString("\n")
		}
	} else {
		help.WriteString("No specialized fields (only common fields)\n")
	}

	return help.String(), nil
}

// GenerateFieldList generates a simple list of field names for a kind
// Useful for auto-completion or quick reference
func GenerateFieldList(kind string, includeCommon bool) ([]string, error) {
	registry := GetGlobalFieldRegistry()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return nil, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}

	var fields []string
	if includeCommon {
		// Include all fields
		for i := range kindFields.AllFields {
			fields = append(fields, kindFields.AllFields[i].Name)
		}
	} else {
		// Only specialized fields
		for i := range kindFields.SpecializedFields {
			fields = append(fields, kindFields.SpecializedFields[i].Name)
		}
	}

	return fields, nil
}

func fieldHasNamedTrait(traits []string, want string) bool {
	for _, t := range traits {
		if t == want {
			return true
		}
	}
	expanded, err := lookupTraitRegistry().ExpandTraits(traits)
	if err != nil {
		return false
	}
	for _, t := range expanded {
		if t == want {
			return true
		}
	}
	return false
}

func fieldsWithTrait(kindFields *KindFields, want string) []string {
	var out []string
	if kindFields == nil {
		return out
	}
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		if fieldHasNamedTrait(field.Traits, want) {
			out = append(out, field.Name)
		}
	}
	return out
}

// GenerateFilterableFields returns fields that can be used for filtering
func GenerateFilterableFields(kind string) ([]string, error) {
	registry := GetGlobalFieldRegistry()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return nil, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}
	return fieldsWithTrait(kindFields, "filterable"), nil
}

// GenerateSortableFields returns fields that can be used for sorting
func GenerateSortableFields(kind string) ([]string, error) {
	registry := GetGlobalFieldRegistry()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return nil, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}
	return fieldsWithTrait(kindFields, "sortable"), nil
}

// GenerateGroupableFields returns fields that can be used for grouping
func GenerateGroupableFields(kind string) ([]string, error) {
	registry := GetGlobalFieldRegistry()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return nil, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}
	return fieldsWithTrait(kindFields, "groupable"), nil
}
