package objects

import (
	"context"
	"fmt"
)

// CriteriaLookupFunc is a function type for looking up criteria IDs by checklist item name
// This allows SpecValidator to work without importing the storage package
type CriteriaLookupFunc func(ctx context.Context, itemName string) (string, error)

// ValidationError represents a field validation error
type ValidationError struct {
	Field       string // Field name
	MissingItem string // Missing checklist item (e.g., "purpose", "system_usage")
	CriteriaRef string // Reference to criteria object (e.g., "CRIT-8199") - may be empty if criteria not found
	Message     string // Human-readable error message
}

// SpecValidator validates object specifications
type SpecValidator struct {
	loader         *SpecLoader
	criteriaLookup CriteriaLookupFunc // Optional: for criteria traceability
	traitRegistry  *TraitRegistry     // Trait registry for trait validation
}

// NewSpecValidator creates a new spec validator
// If criteriaLookup is nil, criteria references will be empty (no traceability)
// Trait registry is automatically initialized
func NewSpecValidator(loader *SpecLoader, criteriaLookup CriteriaLookupFunc) *SpecValidator {
	return &SpecValidator{
		loader:         loader,
		criteriaLookup: criteriaLookup,
		traitRegistry:  NewTraitRegistry(),
	}
}

// RequiredChecklistItems defines the required checklist items for field vetting
// These are the field vetting questions that must be answered
// Note: display_length is also required in the validation section for fields with
// listable, filterable, or sortable traits (checked separately in ValidateSpec)
var RequiredChecklistItems = []string{
	"purpose",
	"system_usage",
	"criticality",
	"cardinality",
	KindLifecycle,
	"authority",
	"validation",
	"dependencies",
	"default",
	"observability",
	"security",
	"automation_hooks",
	"field_profile_code",
}

// ValidateSpec validates a spec and returns all validation errors
func (sv *SpecValidator) ValidateSpec(ctx context.Context, spec *Spec) []ValidationError {
	var errors []ValidationError

	// Validate object-level traits
	traitErrors := sv.traitRegistry.ValidateTraits(spec.ResolvedTraits, "object")
	for _, traitErr := range traitErrors {
		errors = append(errors, ValidationError{
			Field:       "traits",
			MissingItem: traitErr.Category,
			CriteriaRef: "",
			Message:     fmt.Sprintf("Trait validation error: %s", traitErr.Message),
		})
	}
	// Authored list only: ResolvedTraits merges parent groups (base_object_traits +
	// inherited base_auditable_traits) even though Includes already compose them.
	if len(spec.Traits) > 0 {
		for _, traitErr := range sv.traitRegistry.ValidateRedundantIncludes(spec.Traits) {
			errors = append(errors, ValidationError{
				Field:       "traits",
				MissingItem: traitErr.Category,
				CriteriaRef: "",
				Message:     fmt.Sprintf("Trait validation error: %s", traitErr.Message),
			})
		}
	}

	// Validate each field in the resolved spec
	// ResolvedFields contains the merged field definitions from inheritance chain
	for fieldName, fieldDef := range spec.ResolvedFields {
		fieldMap, ok := fieldDef.(map[string]any)
		if !ok {
			continue // Skip non-map fields (shouldn't happen in proper specs)
		}

		// Validate field-level traits
		fieldTraits := ExtractFieldTraits(fieldMap)
		if len(fieldTraits) > 0 {
			// Expand object traits to get individual traits for dependency checking
			expandedObjectTraits, _ := sv.traitRegistry.ExpandTraits(spec.ResolvedTraits)
			// Validate field traits are valid (pass expanded object traits for dependency checking)
			fieldTraitErrors := sv.traitRegistry.ValidateTraitsWithContext(fieldTraits, "field", expandedObjectTraits)
			for _, traitErr := range fieldTraitErrors {
				errors = append(errors, ValidationError{
					Field:       fieldName,
					MissingItem: traitErr.Category,
					CriteriaRef: "",
					Message:     fmt.Sprintf("Field trait validation error: %s", traitErr.Message),
				})
			}

			// Validate field traits are subset of object traits
			consistencyErrors := sv.traitRegistry.ValidateTraitFieldConsistency(spec.ResolvedTraits, fieldTraits, fieldName)
			for _, traitErr := range consistencyErrors {
				errors = append(errors, ValidationError{
					Field:       fieldName,
					MissingItem: traitErr.Category,
					CriteriaRef: "",
					Message:     traitErr.Message,
				})
			}
		}

		// Get checklist from field definition
		// Field structure: { type: "...", checklist: { purpose: "...", ... } }
		checklist, ok := fieldMap["checklist"].(map[string]any)
		if !ok {
			// Field has no checklist at all - all items missing
			for _, item := range RequiredChecklistItems {
				criteriaRef := sv.getCriteriaRef(ctx, item)
				errors = append(errors, ValidationError{
					Field:       fieldName,
					MissingItem: item,
					CriteriaRef: criteriaRef,
					Message:     fmt.Sprintf("Field %s missing checklist (all items required)", fieldName),
				})
			}
			continue
		}

		// Check each required checklist item
		for _, item := range RequiredChecklistItems {
			// Check if item exists in checklist
			value, exists := checklist[item]
			if !exists || value == nil || value == emptyValue {
				criteriaRef := sv.getCriteriaRef(ctx, item)
				errors = append(errors, ValidationError{
					Field:       fieldName,
					MissingItem: item,
					CriteriaRef: criteriaRef,
					Message:     fmt.Sprintf("Field %s missing required checklist item: %s", fieldName, item),
				})
			}
		}

		// Validate display_length constraint for fields with display-related traits
		// Fields with listable, filterable, or sortable traits should have display_length
		// set for proper table formatting (per CRIT-8202 and REQ-019)
		hasDisplayTrait := false
		displayTraits := []string{"listable", "filterable", "sortable"}
		for _, trait := range fieldTraits {
			for _, displayTrait := range displayTraits {
				if trait == displayTrait {
					hasDisplayTrait = true
					break
				}
			}
			if hasDisplayTrait {
				break
			}
		}

		if hasDisplayTrait {
			validationMap, hasValidation := fieldMap["validation"].(map[string]any)
			if hasValidation {
				// Check if display_length is set
				displayLength, hasDisplayLength := validationMap["display_length"]
				if !hasDisplayLength || displayLength == nil {
					// Warn that fields with display traits should have display_length
					errors = append(errors, ValidationError{
						Field:       fieldName,
						MissingItem: "display_length",
						CriteriaRef: "",
						Message:     fmt.Sprintf("Field %s has display-related trait (listable, filterable, or sortable) but missing display_length constraint in validation - required for proper table formatting", fieldName),
					})
				} else {
					// Validate display_length is a positive integer
					var displayLenInt int
					switch v := displayLength.(type) {
					case int:
						displayLenInt = v
					case float64:
						displayLenInt = int(v)
					default:
						errors = append(errors, ValidationError{
							Field:       fieldName,
							MissingItem: "display_length",
							CriteriaRef: "",
							Message:     fmt.Sprintf("Field %s has invalid display_length type: must be an integer, got %T", fieldName, displayLength),
						})
						continue
					}

					if displayLenInt <= 0 {
						errors = append(errors, ValidationError{
							Field:       fieldName,
							MissingItem: "display_length",
							CriteriaRef: "",
							Message:     fmt.Sprintf("Field %s has invalid display_length value: must be positive, got %d", fieldName, displayLenInt),
						})
					}
				}
			}
		}
	}

	return errors
}

// ValidateLoadedSpec runs field-vetting, trait, and display constraint checks on a resolved spec.
// Use this as the single validation entry point after LoadSpecWithInheritance, after spec file writes,
// and for batch checks so new and existing object_specs share one code path (no parallel rules).
func ValidateLoadedSpec(ctx context.Context, loader *SpecLoader, spec *Spec, criteriaLookup CriteriaLookupFunc) []ValidationError {
	if spec == nil {
		return nil
	}
	sv := NewSpecValidator(loader, criteriaLookup)
	return sv.ValidateSpec(ctx, spec)
}

// LoadSpecAndValidate loads a spec by file name (e.g. "backlog_item.yaml") with inheritance and runs
// the same validation as [ValidateLoadedSpec]. Single load — use this in CLI batch paths.
func LoadSpecAndValidate(ctx context.Context, loader *SpecLoader, specFileName string, criteriaLookup CriteriaLookupFunc) (*Spec, []ValidationError, error) {
	spec, err := loader.LoadSpecWithInheritance(specFileName)
	if err != nil {
		return nil, nil, err
	}
	return spec, ValidateLoadedSpec(ctx, loader, spec, criteriaLookup), nil
}

// ValidateSpecFile loads and validates; returns only validation errors (load failure returns a non-nil error).
func ValidateSpecFile(ctx context.Context, loader *SpecLoader, specFileName string, criteriaLookup CriteriaLookupFunc) ([]ValidationError, error) {
	_, errs, err := LoadSpecAndValidate(ctx, loader, specFileName, criteriaLookup)
	return errs, err
}

// getCriteriaRef loads the criteria reference for a checklist item
// Returns empty string if criteria lookup function is not available or criteria not found
func (sv *SpecValidator) getCriteriaRef(ctx context.Context, itemName string) string {
	if sv.criteriaLookup == nil {
		return "" // No criteria lookup function, no traceability
	}

	criteriaID, err := sv.criteriaLookup(ctx, itemName)
	if err != nil || criteriaID == emptyValue {
		return "" // Criteria not found, but validation continues
	}

	return criteriaID
}
