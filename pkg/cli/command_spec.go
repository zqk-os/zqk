package cli

import (
	"github.com/lanceman/zqk/pkg/errfmt"
)

// CommandSpec represents a declarative specification for a CLI command
// This follows the spec-driven builder pattern for semantic organization
// Commands can be defined in YAML and loaded, or built programmatically
type CommandSpec struct {
	// Metadata
	Name        string `yaml:"name"`        // Command name/use (e.g., "get <id>")
	Short       string `yaml:"short"`       // Short description
	Description string `yaml:"description"` // Long description (can be multi-line)

	// Command structure
	Args        *ArgsSpec        `yaml:"args,omitempty"`        // Argument validation
	Flags       []FlagSpec       `yaml:"flags,omitempty"`       // Custom flags
	Subcommands []SubcommandSpec `yaml:"subcommands,omitempty"` // Subcommands

	// Help configuration
	Help *HelpSpec `yaml:"help,omitempty"` // Help text configuration

	// Execution
	RunE  string `yaml:"run_e,omitempty"` // Function name to execute (resolved at build time)
	Async bool   `yaml:"async,omitempty"` // When true, RunE is wrapped with BindAsyncProgress (operation type from command path)

	// Common configuration
	CommonFlags bool `yaml:"common_flags,omitempty"` // Whether to include common flags (default: true)
	// When true, generated non-CRUD builders call [AddQueryFlags] after Build (filter, sort, pagination, count, fields).
	QueryFlags bool `yaml:"query_flags,omitempty"`

	// Harness surfaces (commands that delegate to trait/list or count helpers at runtime).
	// When true, ExpectedLocalFlagNamesFromCommandSpec includes the same flags as AddListFlags / AddCountFlags.
	ListHarnessFlags  bool `yaml:"list_harness_flags,omitempty"`  // pkg/cli harness list flags (filter, sort-by, ...)
	CountHarnessFlags bool `yaml:"count_harness_flags,omitempty"` // AddCountFlags (filter, group-by, include-zero-count)

	// Fields harness (AddFieldsFlags). When FieldsIncludeListKinds is true, matches root "internal fields";
	// when false, matches "internal <kind> fields".
	FieldsHarnessFlags     bool `yaml:"fields_harness_flags,omitempty"`
	FieldsIncludeListKinds bool `yaml:"fields_include_list_kinds,omitempty"`

	// Orchestration (root PreRunE uses these via annotations; see docs/architecture/COMMAND_ORCHESTRATION.md)
	// When set to false, root skips the corresponding init (storage pre-warm, session, scheduler check).
	RequiresStorage        *bool `yaml:"requires_storage,omitempty"`         // default: true for object or internal subtree
	RequiresSession        *bool `yaml:"requires_session,omitempty"`         // default: true
	RequiresSchedulerCheck *bool `yaml:"requires_scheduler_check,omitempty"` // default: true

	// Trait requirements
	RequiredTraits      []string           `yaml:"required_traits,omitempty"`       // Traits required for this command to work (e.g., ["listable"])
	ConditionalTraits   []ConditionalTrait `yaml:"conditional_traits,omitempty"`    // Traits required when specific flags are used
	RequiredTraitGroups []string           `yaml:"required_trait_groups,omitempty"` // Trait groups required (e.g., ["base_object_traits"])

	// Semantic enhancements
	Aliases  []string `yaml:"aliases,omitempty"`  // Alternative names/synonyms for this command
	Synonyms []string `yaml:"synonyms,omitempty"` // Semantic synonyms (e.g., "task" for "backlog_item")

	// Entitlements
	EntitlementBundle string `yaml:"entitlement_bundle,omitempty"` // Required entitlement bundle (e.g. "mesh")
}

// ArgsSpec defines argument validation
type ArgsSpec struct {
	Type      string `yaml:"type"`                // "exact", "minimum", "maximum", "range"
	Count     *int   `yaml:"count,omitempty"`     // Exact count (for "exact")
	Min       *int   `yaml:"min,omitempty"`       // Minimum count (for "minimum", "range")
	Max       *int   `yaml:"max,omitempty"`       // Maximum count (for "maximum", "range")
	Validator string `yaml:"validator,omitempty"` // Custom validator function name
}

// FlagSpec defines a command flag
type FlagSpec struct {
	Name         string `yaml:"name"`                    // Flag name
	Shorthand    string `yaml:"shorthand,omitempty"`     // Short flag (e.g., "f" for --file)
	Type         string `yaml:"type"`                    // "string", "bool", "int", "string_array"
	Default      any    `yaml:"default,omitempty"`       // Default value
	Description  string `yaml:"description"`             // Flag description
	Required     bool   `yaml:"required,omitempty"`      // Whether flag is required
	ValidateFunc string `yaml:"validate_func,omitempty"` // Validation function name
}

// SubcommandSpec defines a subcommand
type SubcommandSpec struct {
	Name    string       `yaml:"name"`               // Subcommand name
	Spec    *CommandSpec `yaml:"spec,omitempty"`     // Inline spec (nested)
	SpecRef string       `yaml:"spec_ref,omitempty"` // Reference to external spec file
}

// HelpSpec defines help text configuration
type HelpSpec struct {
	Short        string            `yaml:"short,omitempty"`         // Short description (overrides top-level)
	Description  []string          `yaml:"description,omitempty"`   // Description lines
	Examples     []HelpExampleSpec `yaml:"examples,omitempty"`      // Examples
	ExcludeFlags []string          `yaml:"exclude_flags,omitempty"` // Flags to exclude from auto-discovery
}

// HelpExampleSpec defines a help example
type HelpExampleSpec struct {
	Comment string `yaml:"comment"` // Example comment/description
	Command string `yaml:"command"` // Example command (with %s for command name substitution)
}

// ConditionalTrait defines a trait that is required when a specific flag is used
type ConditionalTrait struct {
	Flag   string   `yaml:"flag"`   // Flag name (e.g., "group-by")
	Traits []string `yaml:"traits"` // Traits required when this flag is used (e.g., ["groupable"])
}

// CRUDCommandSpec extends CommandSpec with CRUD-specific patterns
type CRUDCommandSpec struct {
	CommandSpec `yaml:",inline"`

	// CRUD-specific configuration
	OperationType    string `yaml:"operation_type"`              // "create", "read", "update", "delete", "list"
	DataInput        bool   `yaml:"data_input,omitempty"`        // Include file/data flags
	UpdateFlags      bool   `yaml:"update_flags,omitempty"`      // Include update-specific flags
	DryRun           bool   `yaml:"dry_run,omitempty"`           // Include dry-run flag
	Cascade          bool   `yaml:"cascade,omitempty"`           // Include cascade flag (for delete)
	UnlinkReferences bool   `yaml:"unlink_references,omitempty"` // Include --unlink-references (for delete)

	// Semantic enhancements (inherited from CommandSpec)
	// Aliases and synonyms are defined in the embedded CommandSpec
}

// Validate validates the command spec
func (s *CommandSpec) Validate() error {
	if s.Name == emptyValue {
		return errfmt.Errorf("command spec must have a name")
	}
	if s.Short == emptyValue && s.Description == emptyValue {
		return errfmt.Errorf("command spec must have either short or description")
	}
	return nil
}

// GetName returns the command name
func (s *CommandSpec) GetName() string {
	return s.Name
}

// Validate validates the CRUD command spec
func (s *CRUDCommandSpec) Validate() error {
	if err := s.CommandSpec.Validate(); err != nil {
		return err
	}
	if s.OperationType == emptyValue {
		return errfmt.Errorf("CRUD command spec must have an operation_type")
	}
	validOps := map[string]bool{
		"create": true,
		"read":   true,
		"update": true,
		"delete": true,
		"list":   true,
	}
	if !validOps[s.OperationType] {
		return errfmt.Errorf("invalid operation_type: %s (must be one of: create, read, update, delete, list)", s.OperationType)
	}
	return nil
}

// GetName returns the command name
func (s *CRUDCommandSpec) GetName() string {
	return s.Name
}
