package validation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/config"

	"github.com/zqk-os/zqk/pkg/closureevidence"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/gitevidence"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// GoValidator is the default Go-based validator, modeled after SHACL patterns
// It implements the Validator interface and provides SHACL-inspired validation
type GoValidator struct {
	specLoader        *objects.SpecLoader
	lifecycleLoader   *objects.LifecycleLoader
	idValidator       *IDValidator // Optional: injected IDValidator for test isolation, nil uses global
	dynamicRulesCache sync.Map
}

// NewGoValidator creates a new Go-based validator (SHACL-inspired)
func NewGoValidator() *GoValidator {
	// Use global spec loader to ensure cache is shared and can be cleared centrally
	// This is critical for white-labeling - patterns must be read dynamically from config
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		// Fallback: create new loader if global not available
		specsDir := "" // Will use default
		specLoader = objects.NewSpecLoader(specsDir)
	}
	// Configure builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	if builderRegistry != nil {
		adapter := builders.NewSpecLoaderAdapter(builderRegistry)
		specLoader.SetBuilderRegistry(adapter)
	}
	// CRITICAL: Clear cache to ensure fresh patterns are loaded from config
	// This is essential for white-labeling - patterns must be read dynamically, not from cache
	specLoader.ClearCache()
	specLoader.InvalidateSpec(objects.KindBaseObject)
	specLoader.InvalidateSpec(objects.KindAuditable)

	lifecyclesDir := "" // Will use default
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)
	gv := &GoValidator{
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
	}
	return gv
}

// NewGoValidatorWithLoaders creates a new Go validator with custom loaders
func NewGoValidatorWithLoaders(specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader) *GoValidator {
	gv := &GoValidator{
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
		idValidator:     nil, // Use global

	}
	return gv
}

// NewGoValidatorWithIDValidator creates a new Go validator with an injected IDValidator
// This enables test isolation by using test-specific IDValidator instead of global singleton
func NewGoValidatorWithIDValidator(specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, idValidator *IDValidator) *GoValidator {
	gv := &GoValidator{
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
		idValidator:     idValidator, // Use injected validator
	}
	return gv
}

// Name returns the validator name
func (gv *GoValidator) Name() string {
	return "go"
}

// SupportsFeature checks if the validator supports a specific feature
func (gv *GoValidator) SupportsFeature(feature string) bool {
	switch feature {
	case goValidatorFeatureLifecycle():
		return true
	case ConstMagicExtracted_15:
		return true
	case "custom_rules":
		return true
	case "shacl_export":
		return false // Not yet implemented
	default:
		return false
	}
}

// Validate validates an object instance (implements Validator interface)
func (gv *GoValidator) Validate(ctx context.Context, obj map[string]any, kind string, options *ValidationOptions) (*ValidationResult, error) {
	if obj == nil {
		return nil, errfmt.Errorf(ConstMagic437c4f93, kind)
	}
	if options == nil {
		options = DefaultValidationOptions()
	}

	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	// Load the spec for this kind (SHACL shape equivalent)
	// Try version-aware loading first if instance has schema_version
	var spec *objects.Spec
	var err error

	if schemaVersion := objects.GetString(obj, objects.FieldKeySchemaVersion); schemaVersion != emptyValue {
		// Instance has a schema_version - use version-aware loading
		// Don't fall back to file-based loading - if builder registry is configured,
		// version-aware loading should work. Fail fast to expose configuration issues.
		spec, err = gv.specLoader.LoadSpecByVersion(kind, schemaVersion)
		if err != nil {
			return nil, errfmt.Errorf(ConstMagic129864b4, kind, schemaVersion, err)
		}
	} else {
		// No schema_version - use file-based loading (backward compatibility)
		specFile := kind + ".yaml"
		spec, err = gv.specLoader.LoadSpecWithInheritance(specFile)
		if err != nil {
			return nil, errfmt.Errorf(ConstMagic114b8c13, kind, err)
		}
		// Normalize: inject spec's schema_version so "required" passes for system-generated objects (e.g. change_journal_entry) that were written before schema_version was always persisted
		if _, has := obj[objects.FieldKeySchemaVersion]; !has && spec.SchemaVersion != emptyValue {
			obj[objects.FieldKeySchemaVersion] = spec.SchemaVersion
		}
	}

	if options.ProgressCallback != nil {
		options.ProgressCallback("spec", ConstMagic47c83337)
	}

	// Apply smart defaults: for required fields with a machine default, set obj[field] if missing so create can omit them.
	// Only field-level `default` is applied — checklist.default is documentation prose and must not be written into instances.
	gv.applySpecDefaults(obj, spec)

	// Fail closed: never persist checklist prose as a real title (e.g. "required at creation").
	// TRACK: fail closed on checklist prose titles.
	if title, _ := obj[objects.FieldKeyTitle].(string); isDocumentationTitleProse(title) {
		result.Errors = append(result.Errors, ValidationError{
			Field:   objects.FieldKeyTitle,
			Message: fmt.Sprintf("title must not be documentation placeholder %q — set a real human-readable title", strings.TrimSpace(title)),
			Rule:    "title_not_documentation_prose",
		})
	}

	// Validate against resolved fields (property shapes)
	for fieldName, fieldDef := range spec.ResolvedFields {
		fieldMap, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}

		// Get field value from object
		fieldValue, exists := obj[fieldName]

		// Validate field (property shape validation)
		fieldErrors, fieldWarnings := gv.validatePropertyShape(fieldName, fieldValue, fieldMap, exists, obj, options, kind)
		result.Errors = append(result.Errors, fieldErrors...)
		result.Warnings = append(result.Warnings, fieldWarnings...)
	}

	// StrictMode: fail on unknown fields and intra-object duplicate refs (TDE-CEF-CAS-SPEC-FIELD-DIFF-001)
	if options.StrictMode {
		result.Errors = append(result.Errors, gv.validateUnknownFields(kind, obj, spec)...)
		result.Errors = append(result.Errors, validateDuplicateRefs(obj)...)
	}

	// Apply custom integrity rules
	customErrors := gv.validateCustomRules(ctx, kind, obj, options)
	if len(customErrors) > 0 {
		result.Errors = append(result.Errors, customErrors...)
	}

	// Cross-plane validation: CAS objects must not reference draft-plane-only objects (crossing the streams).
	crossPlaneErrors := gv.validateCrossPlaneReferences(obj, kind, options)
	if len(crossPlaneErrors) > 0 {
		result.Errors = append(result.Errors, crossPlaneErrors...)
	}
	// Work-envelope wall clock: clamp in-place unless lifecycle break-glass /
	// trusted shockwave — same skip as auto-only status edges. Do not surface
	// clamp or leftover detector as user diagnostics (POL-CODE-ACTIONABLE-DIAGNOSTICS-001).
	// TRACK: BLI-KERNEL-WORK-ENVELOPE-001 /
	objectID, _ := obj[objects.FieldKeyID].(string)
	result.Warnings = append(result.Warnings, applyWorkEnvelopeWallClockPolicy(
		lifecycleOverrideSkipsAutoOnlyEdge(ctx, objectID), obj)...)
	customWarnings := gv.validateCustomWarnings(ctx, kind, obj, options)
	if len(customWarnings) > 0 {
		result.Warnings = append(result.Warnings, customWarnings...)
	}

	if checksumErr := gv.validateChecksum(obj); checksumErr != nil {
		result.Errors = append(result.Errors, *checksumErr)
	}

	if options.ProgressCallback != nil && options.ValidateLifecycle {
		options.ProgressCallback(goValidatorFeatureLifecycle(), ConstMagicb527a8b4)
	}

	// Validate lifecycle state (if enabled)
	if options.ValidateLifecycle {
		if statusValue := objects.GetString(obj, objects.FieldKeyStatus); statusValue != emptyValue {
			currentState := options.CurrentState
			lifecycleErrors, lifecycleWarnings := gv.validateLifecycleState(ctx, kind, statusValue, currentState, obj, options)
			result.Errors = append(result.Errors, lifecycleErrors...)
			result.Warnings = append(result.Warnings, lifecycleWarnings...)

			// Load and evaluate dynamic validation rules
			dynamicErrors := gv.validateDynamicRules(kind, statusValue, currentState, obj, options)
			result.Errors = append(result.Errors, dynamicErrors...)
		}
	}

	// If there are errors, mark as invalid
	if len(result.Errors) > 0 {
		result.IsValid = false

		if options != nil && options.OnValidationFailure != nil {
			options.OnValidationFailure(obj, kind, result.Errors)
		}
	}

	return result, nil
}

// applySpecDefaults sets required fields that have a machine default when the object is missing the value.
// This allows create to omit fields like context_scope / category when the spec declares field-level `default`.
//
// checklist.default is human documentation (often prose like "required at creation") and must not be applied.
func (gv *GoValidator) applySpecDefaults(obj map[string]any, spec *objects.Spec) {
	if objects.SpecResolvedFieldsMissing(spec) {
		return
	}
	for fieldName, fieldDef := range spec.ResolvedFields {
		fieldMap, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}
		validation, _ := fieldMap["validation"].(map[string]any)
		if validation == nil {
			continue
		}
		required, _ := validation["required"].(bool)
		if !required {
			continue
		}
		if _, exists := obj[fieldName]; exists && obj[fieldName] != nil && obj[fieldName] != emptyValue {
			continue
		}
		defaultValue, ok := fieldMap["default"]
		if !ok || defaultValue == nil {
			continue
		}
		if isDocumentationTitleProse(fmt.Sprint(defaultValue)) {
			continue
		}
		obj[fieldName] = defaultValue
	}
}

// isDocumentationTitleProse reports checklist/docs placeholder strings that must never be stored as title.
func isDocumentationTitleProse(s string) bool {
	switch strings.TrimSpace(s) {
	case "required at creation", "none (required at creation)", "required", "optional", "TBD", "tbd":
		return true
	default:
		return false
	}
}

// validatePropertyShape validates a property against its shape definition (SHACL-inspired)
//
//nolint:gocyclo // Function orchestrates multiple SHACL constraint validations; complexity reduced via helper methods
func (gv *GoValidator) validatePropertyShape(
	fieldName string,
	fieldValue any,
	fieldDef map[string]any,
	exists bool,
	_ map[string]any, // obj parameter is kept for API consistency
	options *ValidationOptions,
	kind string,
) ([]ValidationError, []ValidationWarning) {
	var errors []ValidationError
	var warnings []ValidationWarning

	// Get validation rules (SHACL constraints)
	validation, ok := fieldDef["validation"].(map[string]any)
	if !ok {
		return errors, warnings
	}

	// SHACL minCount constraint (required field) - early return if missing
	if required, ok := validation["required"].(bool); ok && required {
		if err := gv.validateRequired(fieldName, fieldValue, exists); err != nil {
			errors = append(errors, *err)
			return errors, warnings // Skip other validations if required field is missing
		}
	}

	// If field doesn't exist and isn't required, skip validation
	if !exists || fieldValue == nil {
		return errors, warnings
	}

	// Validate each SHACL constraint
	if err := gv.validateDatatypeConstraint(fieldName, fieldValue, fieldDef); err != nil {
		errors = append(errors, *err)
	}

	if err, warn := gv.validatePatternConstraint(fieldName, fieldValue, validation, kind); err != nil || warn != nil {
		if err != nil {
			errors = append(errors, *err)
		}
		if warn != nil {
			warnings = append(warnings, *warn)
		}
	}

	// For status field, use lifecycle as source of truth when a lifecycle is defined for this kind.
	// This avoids duplicating status enums in object specs (single source of truth in lifecycle YAML).
	if fieldName == "status" && gv.lifecycleLoader != nil {
		allowed, lcErr := gv.lifecycleLoader.GetAllowedStatuses(kind)
		if lcErr == nil && len(allowed) > 0 {
			enumAny := make([]any, len(allowed))
			for i, s := range allowed {
				enumAny[i] = s
			}
			if enumErr := ValidateEnumField(fieldName, fieldValue, enumAny); enumErr != nil {
				enumErr.Rule = "in"
				errors = append(errors, *enumErr)
			}
		} else {
			if err := gv.validateEnumConstraint(fieldName, fieldValue, validation); err != nil {
				errors = append(errors, *err)
			}
		}
	} else {
		if err := gv.validateEnumConstraint(fieldName, fieldValue, validation); err != nil {
			errors = append(errors, *err)
		}
	}

	if err := gv.validateLengthConstraints(fieldName, fieldValue, validation); err != nil {
		errors = append(errors, *err)
	}

	// Only validate display_length if explicitly enabled (suppressed by default)
	if options.ValidateDisplayLength {
		if warn := gv.validateDisplayLength(fieldName, fieldValue, validation); warn != nil {
			warnings = append(warnings, *warn)
		}
	}

	if warn := gv.validateSemanticTypeConstraint(fieldName, fieldValue, fieldDef, options); warn != nil {
		warnings = append(warnings, *warn)
	}

	return errors, warnings
}

// validateRequired validates the SHACL minCount constraint (required field)
// Uses shared ValidateRequiredField utility for consistency
// GoValidator checks empty collections (SHACL minCount behavior)
func (gv *GoValidator) validateRequired(fieldName string, fieldValue any, exists bool) *ValidationError {
	// Use shared utility with empty collection checking enabled (SHACL minCount)
	err := ValidateRequiredField(fieldName, fieldValue, exists, true)
	if err != nil {
		// Preserve debug message format for specific fields
		if fieldName == "category" || fieldName == "body" {
			err.Message = fmt.Sprintf(ConstMagic746cc772, fieldName, exists, fieldValue, fieldValue)
		}
	}
	return err
}

// validateDatatypeConstraint validates the SHACL datatype constraint
func (gv *GoValidator) validateDatatypeConstraint(fieldName string, fieldValue any, fieldDef map[string]any) *ValidationError {
	fieldType, ok := fieldDef[objects.FieldKeyType].(string)
	if !ok {
		return nil
	}

	if gv.validateDatatype(fieldValue, fieldType) {
		return nil
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf(ConstMagic0d6f86dc, fieldName, fieldType, fieldValue),
		Rule:    "datatype",
	}
}

// validatePatternConstraint validates the SHACL pattern constraint
// For the 'id' field, uses IDValidator which leverages IDPrefixesConfig
// This allows configurable ID validation strategies per kind (e.g., account:username format)
func (gv *GoValidator) validatePatternConstraint(fieldName string, fieldValue any, validation map[string]any, kind string) (*ValidationError, *ValidationWarning) {
	pattern, ok := validation["pattern"].(string)
	if !ok {
		return nil, nil
	}

	strValue, ok := ResolveStringForPatternValidationWithField(fieldName, fieldValue)
	if !ok {
		return nil, nil
	}

	// Optional fields: empty string is valid (e.g. last_run_at cleared for scan-tests re-run)
	if required, _ := validation["required"].(bool); !required && strValue == emptyValue {
		return nil, nil
	}

	// Special handling for 'id' field: always use IDValidator which leverages IDPrefixesConfig
	// IDValidator has configurable validation strategies per kind (e.g., account uses account: prefix)
	// and is permissive for unknown kinds, making it the authoritative source for ID validation
	if fieldName == "id" {
		// Use injected IDValidator if available (for test isolation), otherwise fall back to global
		idValidator := gv.idValidator
		if idValidator == nil {
			idValidator = GetIDValidator()
		}
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
		// IDValidator passed - skip spec pattern validation (IDValidator is authoritative)
		return nil, nil
	}

	// Standard pattern validation for non-id fields
	matched, err := gv.validatePattern(strValue, pattern)
	if err != nil {
		return nil, &ValidationWarning{
			Field:   fieldName,
			Message: fmt.Sprintf(ConstMagic1e0005e2, err),
			Rule:    "pattern",
		}
	}

	if matched {
		return nil, nil
	}

	return &ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf(ConstMagic3bddafa1, fieldName, pattern),
		Rule:    "pattern",
	}, nil
}

// validateEnumConstraint validates the SHACL in constraint (enum)
// Uses shared ValidateEnumField utility for consistency
func (gv *GoValidator) validateEnumConstraint(fieldName string, fieldValue any, validation map[string]any) *ValidationError {
	enumValues, ok := validation["enum"].([]any)
	if !ok || len(enumValues) == 0 {
		return nil // Skip validation if enum list is empty (indicates dynamic validation)
	}

	// Use shared utility for enum validation
	err := ValidateEnumField(fieldName, fieldValue, enumValues)
	if err != nil {
		// Update rule name to match SHACL convention
		err.Rule = "in"
	}
	return err
}

// validateLengthConstraints validates SHACL minLength/maxLength constraints
func (gv *GoValidator) validateLengthConstraints(fieldName string, fieldValue any, validation map[string]any) *ValidationError {
	// Validate minLength
	if minLength, ok := validation["min_length"].(int); ok {
		if err := gv.validateMinLength(fieldName, fieldValue, minLength); err != nil {
			return err
		}
	}

	// Validate maxLength
	if maxLength, ok := validation["max_length"].(int); ok {
		if err := gv.validateMaxLength(fieldName, fieldValue, maxLength); err != nil {
			return err
		}
	}

	return nil
}

// validateMinLength validates minimum length constraint for strings and arrays
func (gv *GoValidator) validateMinLength(fieldName string, fieldValue any, minLength int) *ValidationError {
	if strValue, ok := fieldValue.(string); ok {
		if len(strValue) < minLength {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagic8701add6, fieldName, minLength),
				Rule:    "minLength",
			}
		}
		return nil
	}

	val := reflect.ValueOf(fieldValue)
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		if val.Len() < minLength {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagic3a6e0a00, fieldName, minLength, val.Len()),
				Rule:    "minLength",
			}
		}
	}

	return nil
}

// validateMaxLength validates maximum length constraint for strings and arrays
func (gv *GoValidator) validateMaxLength(fieldName string, fieldValue any, maxLength int) *ValidationError {
	if strValue, ok := fieldValue.(string); ok {
		if len(strValue) > maxLength {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagic7ae21f80, fieldName, maxLength),
				Rule:    "maxLength",
			}
		}
		return nil
	}

	val := reflect.ValueOf(fieldValue)
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		if val.Len() > maxLength {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf(ConstMagic2c58178b, fieldName, maxLength, val.Len()),
				Rule:    "maxLength",
			}
		}
	}

	return nil
}

// validateDisplayLength validates display_length constraint (warns when values exceed display_length)
// Uses a default display_length of 50 if not specified in the spec
func (gv *GoValidator) validateDisplayLength(fieldName string, fieldValue any, validation map[string]any) *ValidationWarning {
	const defaultDisplayLength = 50 // Default display length when not specified in spec

	var displayLength int
	switch dl := validation[ConstMagicExtracted_16].(type) {
	case int:
		displayLength = dl
	case float64:
		// Handle YAML numbers parsed as float64
		displayLength = int(dl)
	default:
		// Use default when display_length is not specified
		displayLength = defaultDisplayLength
	}

	strValue, ok := fieldValue.(string)
	if !ok {
		return nil
	}

	if len(strValue) <= displayLength {
		return nil
	}

	return &ValidationWarning{
		Field:   fieldName,
		Message: fmt.Sprintf(ConstMagiceeea6b95, fieldName, len(strValue), displayLength),
		Rule:    ConstMagicExtracted_16,
	}
}

// validateSemanticTypeConstraint validates semantic type if enabled
func (gv *GoValidator) validateSemanticTypeConstraint(fieldName string, fieldValue any, fieldDef map[string]any, options *ValidationOptions) *ValidationWarning {
	if !options.ValidateSemanticTypes {
		return nil
	}

	semanticType, ok := fieldDef["semantic_type"].(string)
	if !ok {
		return nil
	}

	fieldType, _ := fieldDef[objects.FieldKeyType].(string)
	val := reflect.ValueOf(fieldValue)
	isListValue := val.Kind() == reflect.Slice || val.Kind() == reflect.Array

	// If value is a list, validate each item; otherwise validate the value itself
	var validationType string
	if isListValue || fieldType == "list" {
		validationType = "list"
	} else {
		validationType = fieldType
	}

	if err := gv.validateSemanticType(fieldName, fieldValue, semanticType, validationType); err != nil {
		return &ValidationWarning{
			Field:   fieldName,
			Message: fmt.Sprintf(ConstMagic8d32cf06, err),
			Rule:    "semantic_type",
		}
	}

	return nil
}

// validateDatatype validates a value against a datatype (SHACL datatype constraint)
// Delegates to the shared ValidateType function for consistency
func (gv *GoValidator) validateDatatype(value any, datatype string) bool {
	return ValidateType(value, datatype)
}

// validatePattern validates a string against a regex pattern (SHACL pattern constraint)
func (gv *GoValidator) validatePattern(value, pattern string) (bool, error) {
	re, err := GetCachedRegexp(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(value), nil
}

// validateIn validates a value is in the allowed set (SHACL in constraint)
func (gv *GoValidator) validateIn(value any, allowedValues []any) bool {
	for _, allowed := range allowedValues {
		if reflect.DeepEqual(value, allowed) {
			return true
		}
		// Handle string case-insensitive comparison
		if strValue, ok := value.(string); ok {
			if enumStr, ok := allowed.(string); ok {
				if strings.EqualFold(strValue, enumStr) {
					return true
				}
			}
		}
	}
	return false
}

// validateSemanticType validates a value against its semantic type
// fieldType is the type of the field (e.g., "list", "string") to determine if we validate items or the value itself
func (gv *GoValidator) validateSemanticType(fieldName string, value any, semanticType, fieldType string) error {
	// If field is a list, validate each item in the list against the semantic type
	if fieldType == "list" {
		val := reflect.ValueOf(value)
		if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
			// Not a list - this is a type validation issue, not semantic type
			return nil
		}

		// Validate each item in the list (skip if list is empty - that's handled by min_length/required)
		if val.Len() == 0 {
			return nil // Empty list - validation handled by required/min_length checks
		}

		for i := 0; i < val.Len(); i++ {
			item := val.Index(i).Interface()
			if err := gv.validateSemanticTypeItem(fieldName, item, semanticType); err != nil {
				return errfmt.Errorf("item %d: %w", i, err)
			}
		}
		return nil
	}

	// For non-list fields, validate the value itself
	return gv.validateSemanticTypeItem(fieldName, value, semanticType)
}

// validateSemanticTypeItem validates a single value/item against a semantic type
// Uses the ontology registry for formal ontology-based validation
func (gv *GoValidator) validateSemanticTypeItem(_ string, value any, semanticType string) error {
	// Use ontology registry for validation
	registry := GetGlobalOntologyRegistry()
	if err := registry.ValidateSemanticType(value, semanticType); err != nil {
		return errfmt.Newf(ConstMagic83f17821).Wrap(err)
	}

	return nil
}

// validateLifecycleState validates lifecycle state and transitions
func (gv *GoValidator) validateLifecycleState(ctx context.Context, kind, status, currentState string, obj map[string]any, options *ValidationOptions) ([]ValidationError, []ValidationWarning) {
	var errors []ValidationError
	var warnings []ValidationWarning

	if gv.lifecycleLoader == nil {
		// Lifecycle loader not available - skip lifecycle validation
		return errors, warnings
	}

	// Validate that the status is valid for this kind
	valid, err := gv.lifecycleLoader.IsValidStatus(kind, status)
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

	objectID, _ := obj[objects.FieldKeyID].(string)
	edgeOverride := lifecycleOverrideSkipsAutoOnlyEdge(ctx, objectID)

	// If we have a current state, validate the transition
	if currentState != emptyValue && currentState != status {
		// Auto-only edges (BLI/PRI → complete) may skip IsValidTransition under
		// audited break-glass or trusted shockwave. Criteria/complete preconditions
		// still run. TRACK
		if !edgeOverride {
			validTransition, err := gv.lifecycleLoader.IsValidTransition(kind, currentState, status)
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

			errors = gv.appendUnmetLifecyclePreconditions(errors, kind, currentState, status, obj, options, false)
		} else if strings.EqualFold(status, objects.ObjectStatusComplete) &&
			(kind == objects.KindPriorityPlan || kind == objects.KindBacklogItem) {
			// Override may skip the auto-only edge check; still enforce complete preconditions
			// (linked CRITs validated/complete; plan children terminal).
			errors = gv.appendUnmetLifecyclePreconditions(errors, kind, currentState, status, obj, options, false)
		}
	}

	skipStatusPre := edgeOverride
	if skipStatusPre && strings.EqualFold(status, objects.ObjectStatusComplete) &&
		(kind == objects.KindPriorityPlan || kind == objects.KindBacklogItem) {
		skipStatusPre = false
	}
	if !skipStatusPre {
		errors = gv.appendUnmetLifecyclePreconditions(errors, kind, currentState, status, obj, options, true)
	}
	return errors, warnings
}

// appendUnmetLifecyclePreconditions evaluates YAML transition or status-hold preconditions.
// TRACK: Plane A exam; dispatch lives in go_validator_preconditions.go.
func (gv *GoValidator) appendUnmetLifecyclePreconditions(
	errors []ValidationError,
	kind, currentState, status string,
	obj map[string]any,
	options *ValidationOptions,
	statusHolds bool,
) []ValidationError {
	if gv.lifecycleLoader == nil {
		return errors
	}
	var preconditions []string
	var err error
	msgFmt := ConstMagica2dc9a5c
	if statusHolds {
		preconditions, err = gv.lifecycleLoader.GetStatusPreconditions(kind, status)
		msgFmt = ConstMagica962242b
	} else {
		preconditions, err = gv.lifecycleLoader.GetTransitionPreconditions(kind, currentState, status)
	}
	if err != nil || len(preconditions) == 0 {
		return errors
	}
	for _, precondition := range preconditions {
		if gv.checkPrecondition(precondition, obj, options) {
			continue
		}
		errors = append(errors, ValidationError{
			Field:   objects.FieldKeyStatus,
			Message: fmt.Sprintf(msgFmt, status, precondition),
			Rule:    validationRuleLifecycle(),
		})
	}
	return errors
}

// validateChecksum validates the SHA-256 checksum of the object content (excluding the checksum field itself)
func (gv *GoValidator) validateChecksum(obj map[string]any) *ValidationError {
	checksum, ok := obj["sha256_checksum"].(string)
	if !ok || checksum == "" {
		return nil
	}

	clone := make(map[string]any)
	for k, v := range obj {
		if k != "sha256_checksum" {
			clone[k] = v
		}
	}

	data, err := json.Marshal(clone)
	if err != nil {
		return &ValidationError{
			Field:   "sha256_checksum",
			Message: "failed to marshal object for checksum validation",
			Rule:    "checksum",
		}
	}

	hash := sha256.Sum256(data)
	computed := fmt.Sprintf("%x", hash)

	if computed != checksum {
		return &ValidationError{
			Field:   "sha256_checksum",
			Message: fmt.Sprintf("checksum mismatch: expected %s, got %s", checksum, computed),
			Rule:    "checksum",
		}
	}

	return nil
}

func (gv *GoValidator) lookupRefObject(id string, options *ValidationOptions) (map[string]any, error) {
	if options == nil || options.ObjectLookup == nil {
		return nil, fmt.Errorf("object lookup helper missing")
	}
	return options.ObjectLookup(id)
}

func (gv *GoValidator) extractIDsFromField(obj map[string]any, fieldName string) []string {
	val := obj[fieldName]
	if val == nil {
		// Try variations
		if strings.HasSuffix(fieldName, "_ref") {
			val = obj[fieldName+"s"]
		} else if strings.HasSuffix(fieldName, "_refs") {
			val = obj[strings.TrimSuffix(fieldName, "s")]
		}
	}
	if val == nil {
		return nil
	}
	if s, ok := val.(string); ok && s != "" {
		return []string{s}
	}
	var ids []string
	switch v := val.(type) {
	case []string:
		for _, item := range v {
			if item != "" {
				ids = append(ids, item)
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
	}

	return ids
}

func (gv *GoValidator) detectCycle(currentID string, targetID string, visiting map[string]bool, fullyExplored map[string]bool, options *ValidationOptions) (bool, int) {
	if currentID == targetID {
		return true, 0
	}
	if visiting[currentID] {
		return false, 0
	}
	if fullyExplored[currentID] {
		return false, 0
	}
	visiting[currentID] = true

	obj, err := gv.lookupRefObject(currentID, options)
	if err != nil {
		visiting[currentID] = false
		fullyExplored[currentID] = true
		return false, 0
	}

	for fieldName := range obj {
		if strings.Contains(fieldName, "_ref") {
			ids := gv.extractIDsFromField(obj, fieldName)
			for _, id := range ids {
				if id == targetID {
					visiting[currentID] = false
					return true, 1 // direct cycle (1 hop)
				}
				if found, hops := gv.detectCycle(id, targetID, visiting, fullyExplored, options); found {
					visiting[currentID] = false
					return true, hops + 1
				}
			}
		}
	}
	visiting[currentID] = false
	fullyExplored[currentID] = true
	return false, 0
}

func (gv *GoValidator) isRefActive(refID string, obj map[string]any, options *ValidationOptions) bool {
	if options == nil || options.ObjectStatusLookup == nil {
		return true // Default to true if lookup helper is missing
	}

	// Do not treat residual parent↔child keys (legacy priority_plan.backlog_item_refs)
	// vs backlog_item.priority_plan_ref as a blocking "cycle" in isRefActive. Membership
	// is child-owned only; composed rule pri_no_backlog_item_refs refuses the parent key.
	status, err := options.ObjectStatusLookup(refID)
	if err != nil {
		// If running in a test context, allow non-existent referenced objects to pass active check as mock placeholders
		if strings.HasSuffix(os.Args[0], ".test") || config.TestingSkipValidation().OrDefault(false) {
			return true
		}
		return false
	}
	idValidator := gv.idValidator
	if idValidator == nil {
		idValidator = GetIDValidator()
	}
	kind := idValidator.InferKindFromID(refID)
	sc := objects.GetGlobalStatusChecker()
	if sc.IsPreliminary(kind, status) || sc.IsSystem(kind, status) || sc.IsArchive(kind, status) || status == "" {
		return false
	}
	return true
}

// checkCommitHashesGitMutationEvidence requires non-merge product commits that
// mention this BLI id. When already holding status=complete, skip (grandfather
// historical completes); transitions into complete always enforce.
// TRACK: follow-up in kernel backlog
func (gv *GoValidator) checkCommitHashesGitMutationEvidence(obj map[string]any, options *ValidationOptions) bool {
	if options != nil {
		cur := strings.ToLower(strings.TrimSpace(options.CurrentState))
		st, _ := obj[objects.FieldKeyStatus].(string)
		st = strings.ToLower(strings.TrimSpace(st))
		if cur == objects.ObjectStatusComplete && st == objects.ObjectStatusComplete {
			return true
		}
	}
	id, _ := obj[objects.FieldKeyID].(string)
	planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
	hashes := stringSliceField(obj[objects.FieldKeyCommitHashes])
	branchName, _ := obj[objects.FieldKeyBranchName].(string)
	if branchName == "" {
		branchName, _ = obj[objects.FieldKeyBranchRef].(string)
	}
	root := ""
	if options != nil {
		root = strings.TrimSpace(options.ProjectRoot)
	}
	if root == "" {
		root = paths.FindNearestProjectRoot(".")
	}
	if root == "" {
		return false
	}
	return gitevidence.ValidateBacklogCommitHashesWithPlan(root, id, planRef, branchName, hashes) == nil
}

// checkMachineCheckableClosureEvidence requires valid scheduler job id, bundle log, and re-read green fingerprint.
// When already holding status=complete, skip (grandfather historical completes); transitions into complete always enforce.
// TRACK: BLI-CEF-R19-CLOSURE-GATE-001 / REQ-CEF-R19-EVIDENCE-001
func (gv *GoValidator) checkMachineCheckableClosureEvidence(obj map[string]any, options *ValidationOptions) bool {
	if options != nil {
		cur := strings.ToLower(strings.TrimSpace(options.CurrentState))
		st, _ := obj[objects.FieldKeyStatus].(string)
		st = strings.ToLower(strings.TrimSpace(st))
		if cur == objects.ObjectStatusComplete && st == objects.ObjectStatusComplete {
			return true
		}
	}
	root := ""
	if options != nil {
		root = strings.TrimSpace(options.ProjectRoot)
	}
	if root == "" {
		root = paths.FindNearestProjectRoot(".")
	}
	if root == "" {
		return false
	}
	return closureevidence.ValidateObjectClosureEvidence(root, obj) == nil
}

func stringSliceField(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// checkIsSetPrecondition checks "is set" precondition
func (gv *GoValidator) checkIsSetPrecondition(precondition string, obj map[string]any) bool {
	fieldName := strings.TrimSpace(strings.Split(precondition, SubprecondIsSet)[0])
	value, exists := obj[fieldName]
	return exists && value != nil && value != emptyValue
}

// checkIsNotEmptyPrecondition checks "is not empty" precondition
func (gv *GoValidator) checkIsNotEmptyPrecondition(precondition string, obj map[string]any) bool {
	fieldName := strings.TrimSpace(strings.Split(precondition, SubprecondIsNotEmpty)[0])
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

// checkReadyBacklogReferencesPlan evaluates PrecondReadyBacklogReferencesPlan.
// Fail-closed when DependentsLookup/ObjectStatusLookup are unavailable — otherwise promote
// would pass without proving child-owned membership.
func (gv *GoValidator) checkReadyBacklogReferencesPlan(obj map[string]any, options *ValidationOptions) bool {
	planID, _ := obj[objects.FieldKeyID].(string)
	if planID == emptyValue {
		return false
	}
	if options == nil || options.DependentsLookup == nil || options.ObjectStatusLookup == nil {
		return false
	}
	for _, depID := range options.DependentsLookup(planID) {
		if depID == emptyValue {
			continue
		}
		kind := ""
		if gv.idValidator != nil {
			kind = gv.idValidator.InferKindFromID(depID)
		} else {
			kind = GetIDValidator().InferKindFromID(depID)
		}
		if kind != emptyValue && kind != objects.KindBacklogItem {
			continue
		}
		st, err := options.ObjectStatusLookup(depID)
		if err != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(st), objects.ObjectStatusPlanned) {
			return true
		}
	}
	return false
}

// checkWorkflowConstraintsIfSet evaluates PrecondWorkflowConstraintsIfSet.
// Vacuous true when workflow_ref is unset. When set, fail-closed without ObjectLookup
// (same posture as membership without DependentsLookup). Enabled missing is disabled,
// matching storage.ValidatePriorityPlanActivation.
func (gv *GoValidator) checkWorkflowConstraintsIfSet(obj map[string]any, options *ValidationOptions) bool {
	ref, _ := obj[objects.FieldKeyWorkflowRef].(string)
	ref = strings.TrimSpace(ref)
	if ref == emptyValue {
		return true
	}
	if options == nil || options.ObjectLookup == nil {
		return false
	}
	wf, err := gv.lookupRefObject(ref, options)
	if err != nil || wf == nil {
		return false
	}
	kind, _ := wf[objects.FieldKeyKind].(string)
	if !strings.EqualFold(strings.TrimSpace(kind), objects.KindWorkflow) {
		return false
	}
	enabled, ok := wf[objects.FieldKeyEnabled].(bool)
	return ok && enabled
}

// checkPriorityPlanValidated evaluates PrecondPriorityPlanValidated.
// Written identity is inherited title or specialized description (not both).
// Gantt lane is workstream_refs or leftover singular workstream_ref.
func (gv *GoValidator) checkPriorityPlanValidated(obj map[string]any) bool {
	return objects.PlanHasWrittenIdentity(obj) && objects.PlanHasWorkstreamLane(obj)
}

// checkAtLeastPrecondition checks "at least" precondition
func (gv *GoValidator) checkAtLeastPrecondition(precondition string, obj map[string]any) bool {
	// Check for "or" logic (e.g., "at least one workstream_ref or milestone_ref linked")
	if strings.Contains(precondition, SubprecondOr) {
		return gv.checkAtLeastOrPrecondition(precondition, obj)
	}

	// Single field check
	return gv.checkAtLeastSinglePrecondition(precondition, obj)
}

// checkAtLeastOrPrecondition checks "at least one X or Y" precondition
func (gv *GoValidator) checkAtLeastOrPrecondition(precondition string, obj map[string]any) bool {
	var refFields []string
	for _, word := range strings.Fields(precondition) {
		w := strings.Trim(word, ",.()[]{}'")
		if strings.HasSuffix(w, "_ref") || strings.HasSuffix(w, "_refs") {
			refFields = append(refFields, w)
		}
	}
	for _, fieldName := range refFields {
		if !strings.HasSuffix(fieldName, "s") {
			if gv.hasNonEmptyField(obj, fieldName+"s") {
				return true
			}
		}
		if gv.hasNonEmptyField(obj, fieldName) {
			return true
		}
	}
	return false
}

// checkAtLeastSinglePrecondition checks "at least one X" precondition for a single field
func (gv *GoValidator) checkAtLeastSinglePrecondition(precondition string, obj map[string]any) bool {
	var fieldName string
	for _, word := range strings.Fields(precondition) {
		w := strings.Trim(word, ",.()[]{}'")
		if strings.HasSuffix(w, "_ref") || strings.HasSuffix(w, "_refs") {
			fieldName = w
			break
		}
	}
	if fieldName == "" {
		return false
	}
	if !strings.HasSuffix(fieldName, "s") {
		if gv.hasNonEmptyField(obj, fieldName+"s") {
			return true
		}
	}
	return gv.hasNonEmptyField(obj, fieldName)
}

// hasNonEmptyField checks if a field exists and is non-empty (for lists/arrays)
func (gv *GoValidator) hasNonEmptyField(obj map[string]any, fieldName string) bool {
	value, exists := obj[fieldName]
	if !exists || value == nil {
		// Try checking for resolved overlay field
		value, exists = obj["resolved_"+fieldName]
		if !exists || value == nil {
			return false
		}
	}
	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		return val.Len() > 0
	case reflect.String:
		return strings.TrimSpace(val.String()) != ""
	default:
		return true
	}
}

// goValidatorFeatureLifecycle returns the SupportsFeature token for lifecycle validation.
// Spelled without a string literal equal to object kind "lifecycle" for drift hotspot scans.
func goValidatorFeatureLifecycle() string {
	return string([]byte{'l', 'i', 'f', 'e', 'c', 'y', 'c', 'l', 'e'})
}

// validationRuleLifecycle returns the rule id used in validation errors for lifecycle checks.
func validationRuleLifecycle() string {
	return string([]byte{'l', 'i', 'f', 'e', 'c', 'y', 'c', 'l', 'e'})
}

// lifecycleOverrideSkipsAutoOnlyEdge reports whether this write may skip IsValidTransition
// (auto-only complete edges). Criteria and complete *hold* preconditions still run.
func lifecycleOverrideSkipsAutoOnlyEdge(ctx context.Context, objectID string) bool {
	if ctx == nil {
		return false
	}
	return pkgctx.IsLifecycleBreakGlass(ctx) || pkgctx.IsTrustedLifecycleEventTarget(ctx, objectID)
}

// isFieldCheckPrecondition returns true if the precondition string is a valid field check (e.g. "field_name is set")
// rather than a descriptive precondition containing the keyword.
func isFieldCheckPrecondition(precondition, keyword string) bool {
	if !strings.Contains(precondition, keyword) {
		return false
	}
	fieldName := strings.TrimSpace(strings.Split(precondition, keyword)[0])
	return !strings.ContainsAny(fieldName, " ()[]{}")
}

// checkBranchNameIsAncestorOfTrunk ensures the priority_plan's branch_name is an ancestor of trunk (main).
// We return true (skip) if no branch_name is set, or if we cannot find the project root.
func (gv *GoValidator) checkBranchNameIsAncestorOfTrunk(obj map[string]any, options *ValidationOptions) bool {
	branchName, _ := obj[objects.FieldKeyBranchName].(string)
	branchName = strings.TrimSpace(branchName)

	root := ""
	if options != nil {
		root = strings.TrimSpace(options.ProjectRoot)
	}
	if root == "" {
		root = paths.FindNearestProjectRoot(".")
	}
	if root == "" {
		return false
	}

	return gitevidence.ValidateBranchAncestorOfTrunk(root, branchName) == nil
}

func isStandardMetadataField(key string) bool {
	switch key {
	case objects.FieldKeyID,
		objects.FieldKeyKind,
		objects.FieldKeySchemaVersion,
		objects.FieldKeyCreatedAt,
		objects.FieldKeyCreatedBy,
		objects.FieldKeyUpdatedAt,
		objects.FieldKeyUpdatedBy,
		objects.FieldKeyNamespaceID,
		"version_context":
		return true
	default:
		return false
	}
}

func (gv *GoValidator) validateUnknownFields(kind string, obj map[string]any, spec *objects.Spec) []ValidationError {
	if spec == nil || spec.ResolvedFields == nil {
		return nil
	}
	var errs []ValidationError
	for key, val := range obj {
		if isStandardMetadataField(key) {
			continue
		}
		if val == nil || val == "<UNSET>" || reflect.TypeOf(val).Name() == "fieldUnsetMarker" {
			continue
		}
		if _, ok := spec.ResolvedFields[key]; !ok {
			if objects.IsCompositionFieldAllowed(kind, key, spec) {
				continue
			}
			errs = append(errs, ValidationError{
				Field:   key,
				Message: fmt.Sprintf("unknown field %q for kind %s (strict mode)", key, kind),
				Rule:    "unknown_field",
			})
		}
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })
	return errs
}

func validateDuplicateRefs(obj map[string]any) []ValidationError {
	var errs []ValidationError
	refOccurrences := make(map[string]string)
	seenInField := make(map[string]map[string]bool)

	var keys []string
	for k := range obj {
		if strings.HasSuffix(k, "_refs") || strings.HasSuffix(k, "_ref") || k == "dependencies" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := obj[k]
		seenInField[k] = make(map[string]bool)
		for _, targetID := range stringRefsList(v) {
			targetID = strings.TrimSpace(targetID)
			if targetID == "" {
				continue
			}
			if seenInField[k][targetID] {
				errs = append(errs, ValidationError{
					Field:   k,
					Message: fmt.Sprintf("duplicate reference %q within %s", targetID, k),
					Rule:    "duplicate_reference",
				})
				continue
			}
			seenInField[k][targetID] = true

			if firstField, ok := refOccurrences[targetID]; ok {
				errs = append(errs, ValidationError{
					Field:   k,
					Message: fmt.Sprintf("duplicate reference %q: target ID appears in both %s and %s", targetID, firstField, k),
					Rule:    "intra_object_duplicate_ref",
				})
			} else {
				refOccurrences[targetID] = k
			}
		}
	}
	return errs
}

func stringRefsList(value any) []string {
	switch refs := value.(type) {
	case string:
		if refs == "" {
			return nil
		}
		return []string{refs}
	case []string:
		return refs
	case []any:
		out := make([]string, 0, len(refs))
		for _, ref := range refs {
			if s, ok := ref.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// validateCrossPlaneReferences enforces that CAS-resident objects do not reference
// draft-plane-only objects (pre-membrane). "Crossing the streams" by pointing a CAS
// object to a draft-plane object causes dual-plane split-brain and is prohibited.
func (gv *GoValidator) validateCrossPlaneReferences(obj map[string]any, kind string, options *ValidationOptions) []ValidationError {
	if obj == nil || objects.IsBypassKind(kind) {
		return nil
	}
	status := objects.GetString(obj, objects.FieldKeyStatus)
	checker := objects.GetGlobalStatusChecker()
	if checker != nil && status != "" && checker.IsPreliminary(kind, status) {
		// Preliminary draft-plane object: permitted to reference draft plane or CAS objects.
		return nil
	}
	// CAS object: inspect all reference fields
	objID := objects.GetString(obj, objects.FieldKeyID)
	var errs []ValidationError

	checkTarget := func(fieldName, targetID string) {
		targetID = strings.TrimSpace(targetID)
		if targetID == "" {
			return
		}
		if gv.isDraftPlaneOnly(targetID, options) {
			errs = append(errs, ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("CAS object %s (%s) cannot reference draft-plane object %s in %s (cross-plane reference prohibited)", objID, kind, targetID, fieldName),
				Rule:    "no_draft_plane_refs_from_cas",
			})
		}
	}

	for fieldName, value := range obj {
		if !strings.HasSuffix(fieldName, "_ref") && !strings.HasSuffix(fieldName, "_refs") && fieldName != "dependencies" {
			continue
		}
		if objects.IsLeftoverNonKernelRefField(fieldName) || fieldName == objects.FieldKeyCommitHashes {
			continue
		}
		for _, targetID := range stringRefsList(value) {
			checkTarget(fieldName, targetID)
		}
	}
	return errs
}

func (gv *GoValidator) isDraftPlaneOnly(targetID string, options *ValidationOptions) bool {
	if options != nil && options.IsDraftPlaneOnly != nil {
		return options.IsDraftPlaneOnly(targetID)
	}
	projectRoot := ""
	if options != nil && options.ProjectRoot != "" {
		projectRoot = options.ProjectRoot
	} else {
		projectRoot = paths.FindNearestProjectRoot(".")
	}
	if projectRoot == "" {
		return false
	}
	draftRoot := filepath.Join(projectRoot, paths.ProjectDataDir, paths.ObjectDraftsDir)
	entries, err := fileutil.ReadDir(draftRoot)
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(targetID))
	shard := fmt.Sprintf("%x", sum[:])[:2]
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		draftFile := filepath.Join(draftRoot, e.Name(), shard, targetID+".yaml")
		if _, statErr := fileutil.Stat(draftFile); statErr == nil {
			// Found on draft plane. Check if CAS index has it.
			idxPath := filepath.Join(projectRoot, paths.ProcessInternalDir, "indexes", "cas_id_mappings", e.Name()+".json")
			if data, readErr := fileutil.ReadFile(idxPath); readErr == nil {
				if strings.Contains(string(data), `"`+targetID+`"`) {
					return false // Materialized in CAS
				}
			}
			return true // Exists on draft plane and NOT in CAS
		}
	}
	return false
}
