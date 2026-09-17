package validator

import (
	"fmt"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validator validates migration data
type Validator struct{}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateReferences validates that all references are resolved
func (v *Validator) ValidateReferences(resolvedRefs map[string]bool) []*ValidationError {
	var errors []*ValidationError

	for refID, exists := range resolvedRefs {
		if !exists {
			errors = append(errors, &ValidationError{
				Field:   "reference",
				Message: fmt.Sprintf("unresolved reference: %s", refID),
			})
		}
	}

	return errors
}

// ValidateRequiredFields validates that all required fields are present
func (v *Validator) ValidateRequiredFields(properties map[string]any, requiredFields []string) []*ValidationError {
	var errors []*ValidationError

	for _, field := range requiredFields {
		if _, exists := properties[field]; !exists {
			errors = append(errors, &ValidationError{
				Field:   field,
				Message: "required field is missing",
			})
		}
	}

	return errors
}

// DetectOrphans detects entities with no relationships
func (v *Validator) DetectOrphans(entityIDs []string, relationships map[string][]string) []string {
	var orphans []string

	for _, entityID := range entityIDs {
		if refs, exists := relationships[entityID]; !exists || len(refs) == 0 {
			orphans = append(orphans, entityID)
		}
	}

	return orphans
}

// ValidateEdgeTypes validates that edge types are in the allowed list
func (v *Validator) ValidateEdgeTypes(edgeTypes, validEdgeTypes []string) []*ValidationError {
	var errors []*ValidationError

	validMap := make(map[string]bool)
	for _, validType := range validEdgeTypes {
		validMap[validType] = true
	}

	for _, edgeType := range edgeTypes {
		if !validMap[edgeType] {
			errors = append(errors, &ValidationError{
				Field:   "edge_type",
				Message: fmt.Sprintf("invalid edge type: %s", edgeType),
			})
		}
	}

	return errors
}

// CheckDuplicateIDs checks for duplicate entity IDs
func (v *Validator) CheckDuplicateIDs(ids []string) []string {
	seen := make(map[string]int)
	var duplicates []string

	for _, id := range ids {
		seen[id]++
		if seen[id] == 2 {
			// First time we see a duplicate
			duplicates = append(duplicates, id)
		}
	}

	return duplicates
}
