package builders

import "github.com/lanceman/zqk/pkg/objects"

// fbSpecKey* are object-spec field-definition YAML keys for FieldBuilder and nested builders.
const (
	fbSpecKeyAccess           = "access"
	fbSpecKeyCardinality      = "cardinality"
	fbSpecKeyChecklist        = "checklist"
	fbSpecKeyCriticality      = "criticality"
	fbSpecKeyDefault          = "default"
	fbSpecKeyDisplayLength    = "display_length"
	fbSpecKeyEnum             = "enum"
	fbSpecKeyFieldProfileCode = "field_profile_code"
	fbSpecKeyMaxCount         = "maxCount"
	fbSpecKeyMaxLength        = "max_length"
	fbSpecKeyMinCount         = "minCount"
	fbSpecKeyMinLength        = "min_length"
	fbSpecKeyObservability    = "observability"
	fbSpecKeyPattern          = "pattern"
	fbSpecKeyPurpose          = "purpose"
	fbSpecKeyRequires         = "requires"
	fbSpecKeySecurity         = "security"
	fbSpecKeySemanticType     = "semantic_type"
	fbSpecKeySystemUsage      = "system_usage"
	fbSpecKeyTraits           = "traits"
	fbSpecKeyValidation       = "validation"
	fbSpecKeyRequired         = "required"
)

// FieldBuilder provides a fluent API for building field definitions
// This makes generated code more maintainable and easier to read
type FieldBuilder struct {
	name         string
	fieldType    string
	fieldDef     map[string]any
	checklist    map[string]any
	access       map[string]any
	validation   map[string]any
	defaultValue any
	traits       []string
	permissions  string
	semanticType string
	profileCode  string
}

// NewFieldBuilder creates a new field builder
func NewFieldBuilder(name, fieldType string) *FieldBuilder {
	return &FieldBuilder{
		name:      name,
		fieldType: fieldType,
		fieldDef:  make(map[string]any),
		traits:    []string{},
	}
}

// WithChecklist sets the checklist section (accepts ChecklistBuilder or map)
func (fb *FieldBuilder) WithChecklist(checklist any) *FieldBuilder {
	switch v := checklist.(type) {
	case *ChecklistBuilder:
		fb.checklist = v.Build()
	case map[string]any:
		fb.checklist = v
	default:
		// Try to build if it has a Build() method
		if builder, ok := checklist.(interface{ Build() map[string]any }); ok {
			fb.checklist = builder.Build()
		}
	}
	return fb
}

// WithAccess sets the access section (accepts AccessBuilder or map)
func (fb *FieldBuilder) WithAccess(access any) *FieldBuilder {
	switch v := access.(type) {
	case *AccessBuilder:
		fb.access = v.Build()
	case map[string]any:
		fb.access = v
	default:
		// Try to build if it has a Build() method
		if builder, ok := access.(interface{ Build() map[string]any }); ok {
			fb.access = builder.Build()
		}
	}
	return fb
}

// WithDefault sets the default value for the field (auto-resolved at create when omitted)
func (fb *FieldBuilder) WithDefault(defaultValue any) *FieldBuilder {
	fb.defaultValue = defaultValue
	return fb
}

// WithValidation sets the validation section (accepts ValidationBuilder or map)
func (fb *FieldBuilder) WithValidation(validation any) *FieldBuilder {
	switch v := validation.(type) {
	case *ValidationBuilder:
		fb.validation = v.Build()
	case map[string]any:
		fb.validation = v
	default:
		// Try to build if it has a Build() method
		if builder, ok := validation.(interface{ Build() map[string]any }); ok {
			fb.validation = builder.Build()
		}
	}
	return fb
}

// Description sets the description for the field
func (fb *FieldBuilder) Description(description string) *FieldBuilder {
	fb.fieldDef[objects.FieldKeyDescription] = description
	return fb
}

// Purpose sets the purpose for the field (alias for description)
func (fb *FieldBuilder) Purpose(description string) *FieldBuilder {
	return fb.Description(description)
}

// SetDescription is a compatibility alias for Description
func (fb *FieldBuilder) SetDescription(description string) *FieldBuilder {
	return fb.Description(description)
}

// WithTraits adds traits to the field
func (fb *FieldBuilder) WithTraits(traits ...string) *FieldBuilder {
	fb.traits = append(fb.traits, traits...)
	return fb
}

// WithPermissions sets the permissions string (e.g., "rwx")
func (fb *FieldBuilder) WithPermissions(permissions string) *FieldBuilder {
	fb.permissions = permissions
	return fb
}

// WithSemanticType sets the semantic type
func (fb *FieldBuilder) WithSemanticType(semanticType string) *FieldBuilder {
	fb.semanticType = semanticType
	return fb
}

// WithProfileCode sets the field profile code (e.g., "BLI-016")
func (fb *FieldBuilder) WithProfileCode(profileCode string) *FieldBuilder {
	fb.profileCode = profileCode
	return fb
}

// Build builds the field definition map
func (fb *FieldBuilder) Build() map[string]any {
	fieldDef := make(map[string]any)
	for k, v := range fb.fieldDef {
		fieldDef[k] = v
	}

	// Set type (required)
	fieldDef[objects.FieldKeyType] = fb.fieldType

	// Set checklist if provided
	if len(fb.checklist) > 0 {
		fieldDef[fbSpecKeyChecklist] = fb.checklist
	}

	// Set access if provided
	if len(fb.access) > 0 {
		fieldDef[fbSpecKeyAccess] = fb.access
	}

	// Set validation if provided
	if len(fb.validation) > 0 {
		fieldDef[fbSpecKeyValidation] = fb.validation
	}

	// Set top-level default if provided (so validator can apply when field is missing)
	if fb.defaultValue != nil {
		fieldDef[fbSpecKeyDefault] = fb.defaultValue
	}

	// Set traits if provided
	if len(fb.traits) > 0 {
		traits := make([]any, len(fb.traits))
		for i, trait := range fb.traits {
			traits[i] = trait
		}
		fieldDef[fbSpecKeyTraits] = traits
	}

	// Set permissions if provided
	if fb.permissions != emptyValue {
		fieldDef[objects.FieldKeyPermissions] = fb.permissions
	}

	// Set semantic type if provided
	if fb.semanticType != emptyValue {
		fieldDef[fbSpecKeySemanticType] = fb.semanticType
	}

	// Set profile code if provided
	if fb.profileCode != emptyValue {
		fieldDef[fbSpecKeyFieldProfileCode] = fb.profileCode
	}

	return fieldDef
}

// ChecklistBuilder provides a fluent API for building checklist sections
type ChecklistBuilder struct {
	checklist map[string]any
}

// NewChecklistBuilder creates a new checklist builder
func NewChecklistBuilder() *ChecklistBuilder {
	return &ChecklistBuilder{
		checklist: make(map[string]any),
	}
}

// Authority sets the authority field
func (cb *ChecklistBuilder) Authority(authority string) *ChecklistBuilder {
	cb.checklist[objects.FieldKeyAuthority] = authority
	return cb
}

// AutomationHooks sets the automation_hooks field
func (cb *ChecklistBuilder) AutomationHooks(hooks string) *ChecklistBuilder {
	cb.checklist[objects.FieldKeyAutomationHooks] = hooks
	return cb
}

// Cardinality sets the cardinality field
func (cb *ChecklistBuilder) Cardinality(cardinality string) *ChecklistBuilder {
	cb.checklist[fbSpecKeyCardinality] = cardinality
	return cb
}

// Criticality sets the criticality field
func (cb *ChecklistBuilder) Criticality(criticality string) *ChecklistBuilder {
	cb.checklist[fbSpecKeyCriticality] = criticality
	return cb
}

// Default sets the default value
func (cb *ChecklistBuilder) Default(defaultValue any) *ChecklistBuilder {
	cb.checklist[fbSpecKeyDefault] = defaultValue
	return cb
}

// Dependencies sets the dependencies field
func (cb *ChecklistBuilder) Dependencies(dependencies string) *ChecklistBuilder {
	cb.checklist[objects.FieldKeyDependencies] = dependencies
	return cb
}

// Lifecycle sets the lifecycle field
func (cb *ChecklistBuilder) Lifecycle(lifecycle string) *ChecklistBuilder {
	cb.checklist[objects.FieldKeyOriginLifecycle] = NormalizeChecklistLifecycle(lifecycle)
	return cb
}

// Observability sets the observability field
func (cb *ChecklistBuilder) Observability(observability string) *ChecklistBuilder {
	cb.checklist[fbSpecKeyObservability] = NormalizeChecklistObservability(observability)
	return cb
}

// Purpose sets the purpose field
func (cb *ChecklistBuilder) Purpose(purpose string) *ChecklistBuilder {
	cb.checklist[fbSpecKeyPurpose] = purpose
	return cb
}

// Security sets the security field
func (cb *ChecklistBuilder) Security(security string) *ChecklistBuilder {
	cb.checklist[fbSpecKeySecurity] = NormalizeChecklistSecurity(security)
	return cb
}

// SystemUsage sets the system_usage field (can be string or []any)
func (cb *ChecklistBuilder) SystemUsage(usage any) *ChecklistBuilder {
	cb.checklist[fbSpecKeySystemUsage] = usage
	return cb
}

// Validation sets the validation field (in checklist)
func (cb *ChecklistBuilder) Validation(validation string) *ChecklistBuilder {
	cb.checklist[fbSpecKeyValidation] = validation
	return cb
}

// Build builds the checklist map
func (cb *ChecklistBuilder) Build() map[string]any {
	return cb.checklist
}

// AccessBuilder provides a fluent API for building access sections
type AccessBuilder struct {
	access map[string]any
}

// NewAccessBuilder creates a new access builder
func NewAccessBuilder() *AccessBuilder {
	return &AccessBuilder{
		access: make(map[string]any),
	}
}

// Requires sets the requires field
func (ab *AccessBuilder) Requires(requires ...string) *AccessBuilder {
	if len(requires) > 0 {
		req := make([]any, len(requires))
		for i, r := range requires {
			req[i] = r
		}
		ab.access[fbSpecKeyRequires] = req
	}
	return ab
}

// Build builds the access map
func (ab *AccessBuilder) Build() map[string]any {
	return ab.access
}

// ValidationBuilder provides a fluent API for building validation sections
type ValidationBuilder struct {
	validation map[string]any
}

// NewValidationBuilder creates a new validation builder
func NewValidationBuilder() *ValidationBuilder {
	return &ValidationBuilder{
		validation: make(map[string]any),
	}
}

// Required sets the required field
func (vb *ValidationBuilder) Required(required bool) *ValidationBuilder {
	vb.validation[fbSpecKeyRequired] = required
	return vb
}

// Pattern sets the pattern field
func (vb *ValidationBuilder) Pattern(pattern string) *ValidationBuilder {
	vb.validation[fbSpecKeyPattern] = pattern
	return vb
}

// MinLength sets the min_length field
func (vb *ValidationBuilder) MinLength(minLength int) *ValidationBuilder {
	vb.validation[fbSpecKeyMinLength] = minLength
	return vb
}

// MaxLength sets the max_length field
func (vb *ValidationBuilder) MaxLength(maxLength int) *ValidationBuilder {
	vb.validation[fbSpecKeyMaxLength] = maxLength
	return vb
}

// DisplayLength sets the non-destructive UI display_length hint.
func (vb *ValidationBuilder) DisplayLength(displayLength int) *ValidationBuilder {
	vb.validation[fbSpecKeyDisplayLength] = displayLength
	return vb
}

// MinCount sets the minCount field (for lists)
func (vb *ValidationBuilder) MinCount(minCount int) *ValidationBuilder {
	vb.validation[fbSpecKeyMinCount] = minCount
	return vb
}

// MaxCount sets the maxCount field (for lists)
func (vb *ValidationBuilder) MaxCount(maxCount int) *ValidationBuilder {
	vb.validation[fbSpecKeyMaxCount] = maxCount
	return vb
}

// Enum sets the enum field
func (vb *ValidationBuilder) Enum(enum []any) *ValidationBuilder {
	vb.validation[fbSpecKeyEnum] = enum
	return vb
}

// Build builds the validation map
func (vb *ValidationBuilder) Build() map[string]any {
	return vb.validation
}
