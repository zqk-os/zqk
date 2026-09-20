package validation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
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
		return nil, errfmt.Errorf(ConstMagic0a39dd3c)
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
			return nil, errfmt.Errorf(ConstMagic123ebc4c, kind, schemaVersion, err)
		}
	} else {
		// No schema_version - use file-based loading (backward compatibility)
		spec, err = iv.specLoader.LoadSpecWithInheritance(kind + ".yaml")
		if err != nil {
			return nil, errfmt.Errorf(ConstMagic73de3a61, kind, err)
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

	// Get validation rules from field definition
	validation, ok := fieldDef["validation"].(map[string]any)
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
		Message: fmt.Sprintf(ConstMagicd5e5bce7, fieldName, fieldType, fieldValue),
		Rule:    "type",
	}
}

// validateFieldPattern validates field pattern constraint
// For the 'id' field, uses IDValidator which leverages IDPrefixesConfig
// This allows configurable ID validation strategies per kind (e.g., account:username format)
func (iv *InstanceValidator) validateFieldPattern(fieldName string, fieldValue any, validation map[string]any, kind string) (*ValidationError, *ValidationWarning) {
	pattern, ok := validation["pattern"].(string)
	if !ok {
		return nil, nil
	}

	strValue, ok := ResolveStringForPatternValidationWithField(fieldName, fieldValue)
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
				Message: fmt.Sprintf(ConstMagic43850ada, err),
				Rule:    "pattern",
			}, nil
		}
		if !valid {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagicd9b56a88, fieldName, kind),
				Rule:    "pattern",
			}, nil
		}
		// Check if IDValidator actually validated (has patterns for this kind) or was permissive
		// If IDValidator was permissive (returned true for unknown kind), also validate against spec pattern
		var hasPatterns bool
		if err := concurrency.RunInRLockWithLogger(
			&idValidator.mu,
			LockNameInstanceValidatorCheckPatterns,
			lockLoggerSystem(),
			func() error {
				hasPatterns = idValidator.patterns[kind] != nil
				return nil
			},
		); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(

				// IDValidator has patterns for this kind and validated - skip spec pattern validation
				string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if hasPatterns {

			return nil, nil
		}
		// IDValidator was permissive (no patterns for this kind) - fall through to spec pattern validation
	}

	// Standard pattern validation for non-id fields
	re, err := GetCachedRegexp(pattern)
	if err != nil {
		return nil, &ValidationWarning{
			Field:   fieldName,
			Message: fmt.Sprintf(ConstMagic1e0005e2, err),
			Rule:    "pattern",
		}
	}

	if re.MatchString(strValue) {
		return nil, nil
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf(ConstMagic3bddafa1, fieldName, pattern),
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
			Message: fmt.Sprintf(ConstMagic8d32cf06, err),
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
		return errfmt.Newf(ConstMagic83f17821).Wrap(err)
	}

	return nil
}

// validateLifecycleState validates the lifecycle state and transitions (Phase 2)
func (iv *InstanceValidator) validateLifecycleState(kind, status, currentState string, obj map[string]any) ([]ValidationError, []ValidationWarning) {
	var errors []ValidationError
	var warnings []ValidationWarning

	if iv.lifecycleLoader == nil {
		// Lifecycle loader not available - skip lifecycle validation
		return errors, warnings
	}

	// Validate that the status is valid for this kind
	valid, err := iv.lifecycleLoader.IsValidStatus(kind, status)
	if err != nil {
		// If lifecycle file doesn't exist, that's OK - just warn
		warnings = append(warnings, ValidationWarning{
			Field:   objects.FieldKeyStatus,
			Message: fmt.Sprintf(ConstMagic55f76aa6, err),
			Rule:    validationRuleLifecycle(),
		})
		return errors, warnings
	}

	if !valid {
		errors = append(errors, ValidationError{
			Field:   objects.FieldKeyStatus,
			Message: fmt.Sprintf(ConstMagicc9d3d02d, status, kind),
			Rule:    validationRuleLifecycle(),
		})
		return errors, warnings
	}

	// If we have a current state, validate the transition
	if currentState != emptyValue && currentState != status {
		validTransition, err := iv.lifecycleLoader.IsValidTransition(kind, currentState, status)
		if err != nil {
			// Transition validation error
			errors = append(errors, ValidationError{
				Field:   objects.FieldKeyStatus,
				Message: fmt.Sprintf(ConstMagic655b306b, err),
				Rule:    validationRuleLifecycle(),
			})
			return errors, warnings
		}

		if !validTransition {
			errors = append(errors, ValidationError{
				Field:   objects.FieldKeyStatus,
				Message: fmt.Sprintf(ConstMagicd7b84ef1, currentState, status, kind),
				Rule:    validationRuleLifecycle(),
			})
			return errors, warnings
		}

		// Check preconditions for the transition
		preconditions, err := iv.lifecycleLoader.GetTransitionPreconditions(kind, currentState, status)
		if err == nil && len(preconditions) > 0 {
			// Validate preconditions (basic check - can be enhanced)
			for _, precondition := range preconditions {
				// Preconditions are typically field checks like "priority_plan_ref is set"
				// This is a simplified check - full implementation would parse and validate
				if !iv.checkPrecondition(precondition, obj) {
					errors = append(errors, ValidationError{
						Field:   objects.FieldKeyStatus,
						Message: fmt.Sprintf(ConstMagica2dc9a5c, status, precondition),
						Rule:    validationRuleLifecycle(),
					})
				}
			}
		}
	}

	// Check preconditions for the target status
	statusPreconditions, err := iv.lifecycleLoader.GetStatusPreconditions(kind, status)
	if err == nil && len(statusPreconditions) > 0 {
		for _, precondition := range statusPreconditions {
			if !iv.checkPrecondition(precondition, obj) {
				errors = append(errors, ValidationError{
					Field:   objects.FieldKeyStatus,
					Message: fmt.Sprintf(ConstMagica962242b, status, precondition),
					Rule:    validationRuleLifecycle(),
				})
			}
		}
	}

	return errors, warnings
}

// checkPrecondition checks if a precondition is met (simplified implementation)
// Preconditions are strings like "priority_plan_ref is set" or "milestone_refs is not empty"
func (iv *InstanceValidator) checkPrecondition(precondition string, obj map[string]any) bool {
	// Simple precondition checking - can be enhanced with a proper parser
	precondition = strings.ToLower(strings.TrimSpace(precondition))

	if strings.Contains(precondition, strings.ToLower(PrecondCRIShovelReady)) {
		return EvaluateShovelReady(obj).Ready
	}

	// Check for "is set" or "is not empty"
	if isFieldCheckPrecondition(precondition, "is set") {
		// Extract field name (e.g., "priority_plan_ref is set" -> "priority_plan_ref")
		fieldName := strings.TrimSpace(strings.Split(precondition, "is set")[0])
		value, exists := obj[fieldName]
		return exists && value != nil && value != emptyValue
	}

	if isFieldCheckPrecondition(precondition, "is not empty") {
		fieldName := strings.TrimSpace(strings.Split(precondition, "is not empty")[0])
		value, exists := obj[fieldName]
		if !exists {
			return false
		}
		// Check if it's a list/array
		val := reflect.ValueOf(value)
		if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
			return val.Len() > 0
		}
		return value != nil && value != emptyValue
	}

	// Check for "at least" (e.g., "at least one milestone_ref linked")
	if strings.Contains(precondition, "at least") {
		// Extract field name and count
		// This is a simplified parser - full implementation would be more robust
		var fieldName string
		for _, word := range strings.Fields(precondition) {
			w := strings.Trim(word, ",.()[]{}'")
			if strings.HasSuffix(w, "_ref") || strings.HasSuffix(w, "_refs") {
				fieldName = w
				break
			}
		}
		if fieldName != "" {
			if !strings.HasSuffix(fieldName, "s") {
				val, exists := obj[fieldName+"s"]
				if !exists || val == nil {
					val, exists = obj["resolved_"+fieldName+"s"]
				}
				if exists && val != nil {
					valReflect := reflect.ValueOf(val)
					if (valReflect.Kind() == reflect.Slice || valReflect.Kind() == reflect.Array) && valReflect.Len() > 0 {
						return true
					}
				}
			}
			val, exists := obj[fieldName]
			if !exists || val == nil {
				val, exists = obj["resolved_"+fieldName]
			}
			if exists && val != nil {
				valReflect := reflect.ValueOf(val)
				if valReflect.Kind() == reflect.Slice || valReflect.Kind() == reflect.Array {
					return valReflect.Len() > 0
				}
				return val != emptyValue
			}
			return false
		}
	}

	// Default: assume precondition is met if we can't parse it
	// This is permissive - full implementation would fail on unknown preconditions
	return true
}
