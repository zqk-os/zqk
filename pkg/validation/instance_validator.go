package validation

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// InstanceValidator validates object instances against their specifications.
//
// Phase 1 (Complete):
// - Type validation (string, int, list, object, date, datetime, etc.)
// - Pattern validation (regex)
// - Enum validation
// - Required field validation
// - Basic semantic type validation (placeholder)
//
// Phase 2 (Current):
// - Lifecycle state transition validation
// - Required fields per lifecycle state
// - Precondition checking
//
// Phase 3 (Future):
// - Formal ontology integration (BFO, ISO 11179, Schema.org)
// - Custom validation rules from specs
type InstanceValidator struct {
	specLoader      *objects.SpecLoader
	lifecycleLoader *objects.LifecycleLoader
	gv              *GoValidator
	mu              sync.RWMutex
}

// NewInstanceValidator creates a new instance validator
func NewInstanceValidator(specLoader *objects.SpecLoader) *InstanceValidator {
	lifecyclesDir := "" // Will use default
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)
	return &InstanceValidator{
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
	}
}

// NewInstanceValidatorWithLifecycle creates a new instance validator with a custom lifecycle loader
func NewInstanceValidatorWithLifecycle(specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader) *InstanceValidator {
	return &InstanceValidator{
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
	}
}

func (iv *InstanceValidator) getGoValidator() *GoValidator {
	if iv == nil {
		return NewGoValidator()
	}
	iv.mu.RLock()
	if iv.gv != nil {
		defer iv.mu.RUnlock()
		return iv.gv
	}
	iv.mu.RUnlock()

	iv.mu.Lock()
	defer iv.mu.Unlock()
	if iv.gv != nil {
		return iv.gv
	}
	if iv.specLoader != nil || iv.lifecycleLoader != nil {
		iv.gv = NewGoValidatorWithLoaders(iv.specLoader, iv.lifecycleLoader)
	} else {
		iv.gv = NewGoValidator()
	}
	return iv.gv
}

// ValidationResult represents the result of validating an instance
type ValidationResult struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Rule    string // e.g., "required", "type", "pattern", "enum"
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Field   string
	Message string
	Rule    string
}

// ValidateInstance validates an object instance against its spec
// This is a convenience method that uses the default validator from the registry
func (iv *InstanceValidator) ValidateInstance(obj map[string]any, kind string) (*ValidationResult, error) {
	return iv.ValidateInstanceWithState(obj, kind, "")
}

// ValidateInstanceWithState validates an object instance against its spec and lifecycle
// currentState is the current lifecycle state (if known)
// This is a convenience method that uses the default validator from the registry
func (iv *InstanceValidator) ValidateInstanceWithState(obj map[string]any, kind, currentState string) (*ValidationResult, error) {
	// Use the default Go validator for backward compatibility
	validator := GetGlobalRegistry().Get("go")
	if validator == nil {
		return nil, errfmt.Errorf("default validator not available")
	}

	options := &ValidationOptions{
		CurrentState:          currentState,
		ValidateLifecycle:     true,
		ValidateSemanticTypes: true,
		StrictMode:            true,
	}

	return validator.Validate(pkgctx.NewSystemContext(), obj, kind, options)
}

// Legacy method - kept for backward compatibility
// New code should use ValidatorRegistry and Validator interface
func (iv *InstanceValidator) validateInstanceLegacy(obj map[string]any, kind, currentState string) (*ValidationResult, error) {
	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	// Load the spec for this kind
	// Try version-aware loading first if instance has schema_version
	var spec *objects.Spec
	var err error

	if schemaVersion := objects.GetString(obj, objects.FieldKeySchemaVersion); schemaVersion != emptyValue {
		// Instance has a schema_version - use version-aware loading
		// Don't fall back to file-based loading - if builder registry is configured,
		// version-aware loading should work. Fail fast to expose configuration issues.
		spec, err = iv.specLoader.LoadSpecByVersion(kind, schemaVersion)
		if err != nil {
			return nil, errfmt.Errorf("failed to load spec for kind %s version %s (builder registry may not be configured or builder not found): %w", kind, schemaVersion, err)
		}
	} else {
		// No schema_version - use file-based loading (backward compatibility)
		spec, err = iv.specLoader.LoadSpecWithInheritance(kind + ".yaml")
		if err != nil {
			return nil, errfmt.Errorf("failed to load spec for kind %s: %w", kind, err)
		}
	}

	// Validate against resolved fields (includes inherited fields)
	for fieldName, fieldDef := range spec.ResolvedFields {
		fieldMap, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}

		// Get field value from object
		fieldValue, exists := obj[fieldName]

		// Validate field
		fieldErrors, fieldWarnings := iv.validateField(fieldName, fieldValue, fieldMap, exists, obj, kind)
		result.Errors = append(result.Errors, fieldErrors...)
		result.Warnings = append(result.Warnings, fieldWarnings...)
	}

	// Phase 2: Validate lifecycle state if status field exists
	if statusValue := objects.GetString(obj, objects.FieldKeyStatus); statusValue != emptyValue {
		lifecycleErrors, lifecycleWarnings := iv.validateLifecycleState(kind, statusValue, currentState, obj)
		result.Errors = append(result.Errors, lifecycleErrors...)
		result.Warnings = append(result.Warnings, lifecycleWarnings...)
	}

	// If there are errors, mark as invalid
	if len(result.Errors) > 0 {
		result.IsValid = false
	}

	return result, nil
}

// validateField validates a single field against its spec definition
// validateField validates a single field against its definition
//
//nolint:gocyclo // Function orchestrates multiple validation checks; complexity reduced via helper methods
func (iv *InstanceValidator) validateField(fieldName string, fieldValue any, fieldDef map[string]any, exists bool, obj map[string]any, kind string) ([]ValidationError, []ValidationWarning) {
	_ = obj // Reserved for future use (e.g., cross-field validation)
	var errors []ValidationError
	var warnings []ValidationWarning

	validation, ok := ExtractValidationRules(fieldDef)
	if !ok {
		return errors, warnings
	}

	// Check required - early return if missing
	if err := iv.validateRequired(fieldName, fieldValue, exists, validation); err != nil {
		errors = append(errors, *err)
		return errors, warnings // Skip other validations if required field is missing
	}

	// If field doesn't exist and isn't required, skip validation
	if !exists || fieldValue == nil {
		return errors, warnings
	}

	// Validate each aspect
	if err := iv.validateFieldType(fieldName, fieldValue, fieldDef); err != nil {
		errors = append(errors, *err)
	}

	if err, warn := iv.validateFieldPattern(fieldName, fieldValue, validation, kind); err != nil || warn != nil {
		if err != nil {
			errors = append(errors, *err)
		}
		if warn != nil {
			warnings = append(warnings, *warn)
		}
	}

	if err := iv.validateFieldEnum(fieldName, fieldValue, validation); err != nil {
		errors = append(errors, *err)
	}

	if warn := iv.validateFieldSemanticType(fieldName, fieldValue, fieldDef); warn != nil {
		warnings = append(warnings, *warn)
	}

	return errors, warnings
}

// validateRequired validates required field constraint
// Uses shared ValidateRequiredField utility for consistency
func (iv *InstanceValidator) validateRequired(fieldName string, fieldValue any, exists bool, validation map[string]any) *ValidationError {
	required, ok := validation["required"].(bool)
	if !ok || !required {
		return nil
	}

	// Use shared utility (InstanceValidator doesn't check empty collections)
	return ValidateRequiredField(fieldName, fieldValue, exists, false)
}

// validateFieldType validates field type
func (iv *InstanceValidator) validateFieldType(fieldName string, fieldValue any, fieldDef map[string]any) *ValidationError {
	fieldType, ok := fieldDef[objects.FieldKeyType].(string)
	if !ok {
		return nil
	}

	if iv.validateType(fieldValue, fieldType) {
		return nil
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf("Field %s has invalid type: expected %s, got %T", fieldName, fieldType, fieldValue),
		Rule:    "type",
	}
}

// validateFieldPattern validates field pattern constraint
// For the 'id' field, uses IDValidator which leverages IDPrefixesConfig
// This allows configurable ID validation strategies per kind (e.g., account:username format)
func (iv *InstanceValidator) validateFieldPattern(fieldName string, fieldValue any, validation map[string]any, kind string) (*ValidationError, *ValidationWarning) {
	pattern, strValue, ok := ExtractPatternAndString(fieldName, fieldValue, validation)
	if !ok {
		return nil, nil
	}

	// Special handling for 'id' field: use IDValidator if patterns are loaded for this kind,
	// otherwise fall back to spec pattern validation (for test-only kinds or when IDValidator is permissive)
	if fieldName == "id" {
		idValidator := GetIDValidator()
		valid, err := idValidator.ValidateID(strValue, kind)
		if err != nil {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("ID validation failed: %v", err),
				Rule:    "pattern",
			}, nil
		}
		if !valid {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("Field %s does not match ID pattern for kind %s", fieldName, kind),
				Rule:    "pattern",
			}, nil
		}
		// Check if IDValidator actually validated (has patterns for this kind) or was permissive.
		// If IDValidator was permissive (returned true for unknown kind), also validate against spec pattern.
		// IDValidator has patterns for this kind and validated - skip spec pattern validation.
		if idValidator.HasLoadedPattern(kind) {

			return nil, nil
		}
		// IDValidator was permissive (no patterns for this kind) - fall through to spec pattern validation
	}

	// Standard pattern validation for non-id fields
	re, err := GetCachedRegexp(pattern)
	if err != nil {
		return nil, &ValidationWarning{
			Field:   fieldName,
			Message: fmt.Sprintf("Invalid pattern in spec: %v", err),
			Rule:    "pattern",
		}
	}

	if re.MatchString(strValue) {
		return nil, nil
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf("Field %s does not match pattern %s", fieldName, pattern),
		Rule:    "pattern",
	}, nil
}

// validateFieldEnum validates field enum constraint
// Uses shared ValidateEnumField utility for consistency
func (iv *InstanceValidator) validateFieldEnum(fieldName string, fieldValue any, validation map[string]any) *ValidationError {
	enumValues, ok := validation["enum"].([]any)
	if !ok || len(enumValues) == 0 {
		return nil // Skip validation if enum list is empty (indicates dynamic validation)
	}

	// Use shared utility for enum validation
	return ValidateEnumField(fieldName, fieldValue, enumValues)
}

// validateFieldSemanticType validates field semantic type
func (iv *InstanceValidator) validateFieldSemanticType(fieldName string, fieldValue any, fieldDef map[string]any) *ValidationWarning {
	semanticType, ok := fieldDef["semantic_type"].(string)
	if !ok {
		return nil
	}

	if err := iv.validateSemanticType(fieldName, fieldValue, semanticType); err != nil {
		return &ValidationWarning{
			Field:   fieldName,
			Message: fmt.Sprintf("Semantic type validation: %v", err),
			Rule:    "semantic_type",
		}
	}

	return nil
}

// validateType checks if a value matches the expected type
// Delegates to the shared ValidateType function for consistency
func (iv *InstanceValidator) validateType(value any, expectedType string) bool {
	return ValidateType(value, expectedType)
}

// validateEnum checks if a value is in the allowed enum values
func (iv *InstanceValidator) validateEnum(value any, enumValues []any) bool {
	for _, enumValue := range enumValues {
		if reflect.DeepEqual(value, enumValue) {
			return true
		}
		// Handle string case-insensitive comparison
		if strValue, ok := value.(string); ok {
			if enumStr, ok := enumValue.(string); ok {
				if strings.EqualFold(strValue, enumStr) {
					return true
				}
			}
		}
	}
	return false
}

// validateSemanticType validates a value against its semantic type.
// Uses the ontology registry for formal ontology-based validation.
func (iv *InstanceValidator) validateSemanticType(_ string, value any, semanticType string) error {
	// Use ontology registry for validation
	registry := GetGlobalOntologyRegistry()
	if err := registry.ValidateSemanticType(value, semanticType); err != nil {
		return errfmt.Newf("ontology validation failed").Wrap(err)
	}

	return nil
}

// validateLifecycleState validates the lifecycle state and transitions (Phase 2)
// Delegates directly to GoValidator to ensure uniform lifecycle invariants and eliminate duplicated logic.
func (iv *InstanceValidator) validateLifecycleState(kind, status, currentState string, obj map[string]any) ([]ValidationError, []ValidationWarning) {
	if iv == nil || iv.lifecycleLoader == nil {
		return nil, nil
	}
	gv := iv.getGoValidator()
	if gv == nil {
		return nil, nil
	}
	return gv.validateLifecycleState(context.Background(), kind, status, currentState, obj, nil)
}

// checkPrecondition checks if a precondition or postcondition is met using the Unified Kernel Predicate DSL.
func (iv *InstanceValidator) checkPrecondition(precondition string, obj map[string]any) bool {
	gv := iv.getGoValidator()
	if gv != nil {
		met, recognized := gv.evaluatePrecondition(precondition, obj, nil)
		if recognized {
			return met
		}
	} else {
		handled, met := evalOverlayDSLStage(nil, precondition, obj, nil)
		if handled {
			return met
		}
	}

	// Fail closed unless explicit fail-open is enabled
	return zqkenv.PreconditionsFailOpen().Get() == "1"
}
