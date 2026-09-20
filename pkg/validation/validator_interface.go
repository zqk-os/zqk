package validation

import (
	"context"
	"reflect"
)

const emptyValidatorName = ""

// Validator is the interface that all validators must implement
// This allows pluggable validation backends (Go, SHACL, custom, etc.)
type Validator interface {
	// Validate validates an object instance against its specification
	// Returns a ValidationResult with errors and warnings
	Validate(ctx context.Context, obj map[string]any, kind string, options *ValidationOptions) (*ValidationResult, error)

	// Name returns the name of the validator (e.g., "go", "shacl", "custom")
	Name() string

	// SupportsFeature checks if the validator supports a specific feature
	// Features: "lifecycle", "semantic_types", "custom_rules", "shacl_export"
	SupportsFeature(feature string) bool
}

// ValidationProgressFunc is called at validation stage boundaries (spec, lifecycle)
// so the CLI can emit progress. Compatible with pkg/context.ValidationProgressFunc.
type ValidationProgressFunc func(stage, message string)

// ValidationOptions provides options for validation
type ValidationOptions struct {
	// CurrentState is the current lifecycle state (for transition validation)
	CurrentState string

	// ValidateLifecycle enables/disables lifecycle validation
	ValidateLifecycle bool

	// ValidateSemanticTypes enables/disables semantic type validation
	ValidateSemanticTypes bool

	// ValidateDisplayLength enables/disables display_length validation warnings
	// Default: false (suppressed by default - display_length is for UI formatting, not data validation)
	// Set to true to enable warnings when values exceed display_length constraints
	ValidateDisplayLength bool

	// CustomRules allows passing custom validation rules
	CustomRules map[string]any

	// ProjectRoot is the git/worktree root used for commit_refs evidence checks.
	// When empty, validators resolve via paths.FindNearestProjectRoot(".").
	ProjectRoot string

	// StrictMode enables strict validation (fail on unknown fields, etc.)
	StrictMode bool

	// ProgressCallback is optional; when set, called at stage boundaries (e.g. "spec", "lifecycle").
	// Used by the async CLI pattern to report validation progress.
	ProgressCallback ValidationProgressFunc

	// OnValidationFailure is an optional callback that is triggered when an object fails validation.
	// This is used to implement the "shockwave" cascade deletion for invalid objects.
	OnValidationFailure func(obj map[string]any, kind string, errors []ValidationError)

	// ObjectStatusLookup is an optional callback that allows the validator to resolve the status of an object by its ID.
	// This helps avoid circular dependencies between pkg/validation and pkg/storage.
	ObjectStatusLookup func(id string) (string, error)

	// ObjectLookup is an optional callback that allows the validator to resolve the full object map by its ID.
	ObjectLookup func(id string) (map[string]any, error)

	// DependentsLookup returns object IDs that reference the given ID (one-level reverse refs).
	// Used for child-owned membership preconditions (e.g. backlog_item.priority_plan_ref → plan).
	DependentsLookup func(id string) []string

	// IsDraftPlaneOnly reports whether an object ID exists only on the draft plane.
	// Used to ensure CAS-resident objects do not reference draft-plane-only objects (cross-plane invariant).
	IsDraftPlaneOnly func(id string) bool
}

// DefaultValidationOptions returns default validation options
func DefaultValidationOptions() *ValidationOptions {
	return &ValidationOptions{
		ValidateLifecycle:     true,
		ValidateSemanticTypes: true,
		StrictMode:            false,
	}
}

// ValidatorRegistry manages available validators
type ValidatorRegistry struct {
	validators       map[string]Validator
	defaultValidator string
}

// NewValidatorRegistry creates a new validator registry
func NewValidatorRegistry() *ValidatorRegistry {
	registry := &ValidatorRegistry{
		validators:       make(map[string]Validator),
		defaultValidator: "go", // Default to Go-based validator
	}

	// Register default Go validator (SHACL-inspired)
	goValidator := NewGoValidator()
	registry.Register("go", goValidator)
	registry.Register("default", goValidator) // Alias for "go"

	return registry
}

// Register registers a validator with the registry
func (vr *ValidatorRegistry) Register(name string, validator Validator) {
	vr.validators[name] = validator
}

// nilValidatorInterface reports whether v is a nil interface or a nil pointer
// stored in the interface (e.g. var g *GoValidator; Register("go", g)), which is
// not equal to nil in Go and would panic on method calls.
func nilValidatorInterface(v Validator) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}

// Get retrieves a validator by name
// Returns the default validator if name is empty or not found
func (vr *ValidatorRegistry) Get(name string) Validator {
	if name == emptyValidatorName {
		name = vr.defaultValidator
	}
	if validator, ok := vr.validators[name]; ok {
		if nilValidatorInterface(validator) {
			return nil
		}
		return validator
	}
	// Return default if not found
	if validator, ok := vr.validators[vr.defaultValidator]; ok {
		if nilValidatorInterface(validator) {
			return nil
		}
		return validator
	}
	return nil
}

// SetDefault sets the default validator name
func (vr *ValidatorRegistry) SetDefault(name string) {
	if _, ok := vr.validators[name]; ok {
		vr.defaultValidator = name
	}
}

// List returns all registered validator names
func (vr *ValidatorRegistry) List() []string {
	names := make([]string, 0, len(vr.validators))
	for name := range vr.validators {
		names = append(names, name)
	}
	return names
}

// Global registry instance
var globalRegistry *ValidatorRegistry

func init() {
	globalRegistry = NewValidatorRegistry()
}

// GetGlobalRegistry returns the global validator registry
func GetGlobalRegistry() *ValidatorRegistry {
	return globalRegistry
}
