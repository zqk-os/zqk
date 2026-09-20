package system

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ObjectTraitGroupConfig holds configuration for object trait group replacement
type ObjectTraitGroupConfig struct {
	BaseTraits  []string
	Replacement string
	Extras      []string
}

// getBaseTraitsForExtends gets base traits from registry based on extends value
func getBaseTraitsForExtends(extends string, traitRegistry *objects.TraitRegistry) (*ObjectTraitGroupConfig, error) {
	var baseTraits []string
	var replacement string

	switch extends {
	case objects.KindBaseObject, objects.KindExtensibleObject:
		expanded, err := traitRegistry.ExpandTraitGroup("base_object_traits")
		if err != nil {
			return nil, err
		}
		baseTraits = expanded
		replacement = "base_object_traits"
	case objects.KindAuditable:
		expanded, err := traitRegistry.ExpandTraitGroup("base_auditable_traits")
		if err != nil {
			return nil, err
		}
		baseTraits = expanded
		replacement = "base_auditable_traits"
	default:
		return nil, errfmt.Errorf("unsupported extends value: %s", extends)
	}

	return &ObjectTraitGroupConfig{
		BaseTraits:  baseTraits,
		Replacement: replacement,
	}, nil
}

// checkTraitsContainBase checks if traits contain all base traits and extracts extras
func checkTraitsContainBase(baseTraits, traits []string) (bool, []string) {
	// Sort both for comparison
	sort.Strings(baseTraits)
	sort.Strings(traits)

	// Create map for quick lookup
	baseTraitsMap := make(map[string]bool)
	for _, t := range baseTraits {
		baseTraitsMap[t] = true
	}

	// Extract extras (traits not in base)
	var extras []string
	for _, t := range traits {
		if !baseTraitsMap[t] {
			extras = append(extras, t)
		}
	}

	// Check if we have all base traits
	hasAllBase := true
	for _, t := range baseTraits {
		if !containsStringInSlice(traits, t) {
			hasAllBase = false
			break
		}
	}

	return hasAllBase && len(traits) >= len(baseTraits), extras
}

// replaceObjectTraitsSection replaces the traits section in content
func replaceObjectTraitsSection(content string, replacement string, extras []string) (bool, string) {
	lines := strings.Split(content, "\n")
	var newLines []string
	inTraits := false
	replaced := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "traits:") && !inTraits {
			// Start of object-level traits (not indented or at root level)
			indent := getIndent(line)
			if indent <= 4 { // Object-level traits are at root or minimal indent
				inTraits = true
				newLines = append(newLines, line,
					fmt.Sprintf("%s- %s", strings.Repeat(" ", indent+4), replacement))
				// Add extras
				for _, extra := range extras {
					newLines = append(newLines, fmt.Sprintf("%s- %s", strings.Repeat(" ", indent+4), extra))
				}
				replaced = true
				continue
			}
		}

		if inTraits {
			// Skip trait lines until we hit a non-trait line
			if trimmed == emptyValue || strings.HasPrefix(trimmed, "-") {
				// Still in traits, skip this line
				continue
			}
			// End of traits section
			inTraits = false
		}

		newLines = append(newLines, line)
	}

	if replaced {
		return true, strings.Join(newLines, "\n")
	}

	return false, content
}

// FieldTraitState tracks state while processing field traits
type FieldTraitState struct {
	InField       bool
	CurrentField  string
	FieldIndent   int
	InFieldTraits bool
}

// getFieldTraitGroupPatterns returns field trait group patterns
func getFieldTraitGroupPatterns() map[string]string {
	return map[string]string{
		"field_mutable_group":   "readable,writable,modifiable",
		"field_queryable_group": "readable,listable,filterable,sortable,searchable",
		"field_immutable_group": "readable,listable,filterable,sortable,searchable",
		"field_reference_group": "readable,filterable",
		"field_read_only_group": "readable",
		"field_display_group":   "readable,listable,formatable",
	}
}

// detectFieldStart detects if a line is the start of a field definition
func detectFieldStart(trimmed string, indent int, fields map[string]any) (bool, string) {
	if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") && indent >= 4 {
		fieldName := strings.TrimSuffix(trimmed, ":")
		if _, exists := fields[fieldName]; exists {
			return true, fieldName
		}
	}
	return false, ""
}

// detectFieldEnd detects if we've reached the end of a field
func detectFieldEnd(trimmed string, indent int, fieldIndent int) bool {
	return trimmed != emptyValue && indent <= fieldIndent && !strings.HasPrefix(trimmed, " ")
}

// detectFieldTraitsStart detects if a line is the start of field traits
func detectFieldTraitsStart(trimmed string, indent int, fieldIndent int) bool {
	return strings.HasPrefix(trimmed, "traits:") && indent > fieldIndent
}

// processFieldTraitsEnd processes the end of field traits section
func processFieldTraitsEnd(lines []string, lineIdx, fieldIndent int, currentField string, fieldPatterns map[string]string, traitRegistry *objects.TraitRegistry) ([]string, string) {
	// Read ahead to get all traits
	traits := extractFieldTraits(lines, lineIdx-1, fieldIndent)
	replacement := findFieldTraitGroupReplacement(traits, fieldPatterns, traitRegistry)

	var newLines []string
	var changeMsg string

	if replacement != emptyValue {
		newLines = append(newLines, fmt.Sprintf("%s- %s", strings.Repeat(" ", fieldIndent+8), replacement))
		changeMsg = fmt.Sprintf("Field '%s': replaced traits with %s", currentField, replacement)
	} else {
		// Restore original traits
		for _, trait := range traits {
			newLines = append(newLines, fmt.Sprintf("%s- %s", strings.Repeat(" ", fieldIndent+8), trait))
		}
	}

	return newLines, changeMsg
}
