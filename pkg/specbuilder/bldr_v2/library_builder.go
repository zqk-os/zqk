package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// LibraryBuilder builds the library spec at version v2_0_0
// File: bldr_v2/library_builder.go - version is encoded in package/directory name
type LibraryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewLibraryBuilder creates a new builder for library spec version v2_0_0
func NewLibraryBuilder() *LibraryBuilder {
	builder := &LibraryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("library", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Library specification for architectural patterns and composable specifications. Defines libraries of reusable components, connectors, and architectural patterns that can compose into more complex systems. Each spec layer provides more granular components that comprise to make more complicated structures and systems. Libraries built from ground up with semantic structure and definition enable significant data compression by understanding the semantic stack. Implements the Cloneable Configuration Pattern - libraries can be cloned with shared DNA propagation.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("cloneable_configuration")

	// Add fields
	builder.addLibraryFields()

	return builder
}

// addLibraryFields adds the library fields
func (b *LibraryBuilder) addLibraryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("composable_specs", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by composition system to load and compose specifications.").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("spec registry and composition system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to composable specifications. Array of references (IDs or ontology names) to specifications that comprise this library. Specs can comprise specs that comprise specs, enabling hierarchical composition. Each spec layer provides more granular components.").
			Security("non-sensitive").
			SystemUsage([]any{
				"spec composition",
				"hierarchical structure building",
				"semantic stack construction",
			}).
			Validation("Array of strings. Each string should reference a valid spec ontology or ID. References should be resolvable by the spec registry.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(0).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("LIB-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("connector_patterns", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to identify and apply connector patterns for architectural composition.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("connector pattern registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Connector patterns defined in this library. Array of connector pattern identifiers that describe how components connect and compose. Connectors form the basis for building out semantic libraries that describe architectural patterns.").
			Security("non-sensitive").
			SystemUsage([]any{
				"connector identification",
				"pattern matching",
				"architectural composition",
			}).
			Validation("Array of strings. Each string should reference a valid connector pattern identifier.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(0).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("LIB-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("dna_fields", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by DNA propagation system to determine which fields to propagate from source to clones.").
			Cardinality("one").
			Criticality("composition").
			Default([]any{}).
			Dependencies("source configuration field names.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of field names that are part of the DNA (propagated from source to clones). Fields listed here are inherited from the source configuration during propagation. Fields not listed are not part of the DNA and can be freely set on clones. Only applicable to DNA source configurations (dna_source_ref is null/empty).").
			Security("non-sensitive").
			SystemUsage([]any{
				"DNA field definition",
				"propagation scope",
				"clone inheritance",
			}).
			Validation("Array of strings. Each string must be a valid field name from the configuration schema.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(0).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("LIB-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("dna_source_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used by DNA propagation system to identify source configuration and propagate updates.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("source configuration object.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the source configuration (the DNA). If null/empty, this configuration IS the DNA. If set, this is a clone that references the source DNA. Format: object ID (e.g., 'LIB-001') or object reference (e.g., 'library:LIB-001').").
			Security("non-sensitive").
			SystemUsage([]any{
				"DNA propagation",
				"clone identification",
				"source reference resolution",
			}).
			Validation("Must be a valid object ID or reference if provided. If empty, configuration is the DNA source.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^([A-Z]+-\d+|[\w:]+)?$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("LIB-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("dna_version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used by DNA propagation system to detect when DNA has been updated and clones need propagation.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("source configuration updated_at timestamp.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Version/timestamp of the DNA being used by this clone. Set to source configuration's updated_at when clone is created or DNA is propagated. Used to detect when DNA has been updated and clone needs propagation. Format: ISO-8601 datetime (e.g., '2026-01-10T08:00:00Z').").
			Security("non-sensitive").
			SystemUsage([]any{
				"DNA version tracking",
				"propagation detection",
				"clone synchronization",
			}).
			Validation("ISO-8601 datetime format if provided.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("LIB-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("library_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to identify and reference library configurations.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("library naming conventions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Primary library name identifier. The canonical name for this library (e.g., 'connector_library', 'semantic_patterns', 'architecture_components'). Used as the primary reference for library composition and semantic compression.").
			Security("non-sensitive").
			SystemUsage([]any{
				"library identification",
				"composition targeting",
				"semantic stack navigation",
			}).
			Validation("Must be a valid library name string. Should follow naming conventions (e.g., snake_case, kebab-case).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(100).
			MinLength(1).
			Pattern(`^[a-z][a-z0-9_-]*$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r--").
		WithSemanticType("identifier").
		WithProfileCode("LIB-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("library_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to categorize and filter libraries by type.").
			Cardinality("one").
			Criticality("association").
			Default("architectural_patterns").
			Dependencies("library type taxonomy.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Library type/category. Distinguishes different types of libraries (e.g., 'architectural_patterns', 'connectors', 'semantic_components', 'composable_specs'). Enables type-based filtering and composition rules.").
			Security("non-sensitive").
			SystemUsage([]any{
				"library categorization",
				"composition rules",
				"semantic stack organization",
			}).
			Validation("Must be a valid library type. Common values: architectural_patterns, connectors, semantic_components, composable_specs, circuit_breakers.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(50).
			MinLength(1).
			Pattern(`^[a-z][a-z0-9_]*$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("r--").
		WithSemanticType("identifier").
		WithProfileCode("LIB-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for library metadata tagging and filtering.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("library metadata conventions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Library metadata. Contains metadata for tagging and filtering: tags (array of meta tags), description (library description), version (library version), status (library status indicator), effective_date (when library becomes effective), deprecated_date (when library is deprecated). Enables meta tag-based filtering and pointer resolution for library composition.").
			Security("non-sensitive").
			SystemUsage([]any{
				"library metadata",
				"tag-based filtering",
				"pointer resolution",
			}).
			Validation("Object containing metadata. Should include: tags (array of strings), description (string), version (string), status (string), effective_date (string/date), deprecated_date (string/date).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("LIB-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("overrides", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by clone resolution to merge DNA fields with clone-specific overrides.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("source configuration schema.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Field overrides that differ from the DNA. Contains field names and values that override the inherited DNA values. Only fields listed here override DNA; all other DNA fields are inherited. Overrides take precedence over DNA values during clone resolution. Only applicable to clone configurations (dna_source_ref is set).").
			Security("non-sensitive").
			SystemUsage([]any{
				"clone customization",
				"override resolution",
				"clone inheritance",
			}).
			Validation("Object containing field name → value mappings. Field names must be valid fields from the configuration schema. Values must match field types and validation rules.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("LIB-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("propagation_mode", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by DNA propagation system to determine propagation strategy.").
			Cardinality("one").
			Criticality("association").
			Default("immediate").
			Dependencies("propagation requirements and system architecture.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("How updates to the DNA propagate to this clone. Options: immediate (changes propagate immediately when source is updated), manual (changes require explicit propagation command), scheduled (changes propagate on schedule, e.g., daily sync), versioned (clones track DNA version and can choose when to update). Only applicable to clone configurations (dna_source_ref is set).").
			Security("non-sensitive").
			SystemUsage([]any{
				"propagation control",
				"clone synchronization",
				"update strategy",
			}).
			Validation("Must be one of: immediate, manual, scheduled, versioned.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"immediate",
				"manual",
				"scheduled",
				"versioned",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("LIB-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to determine applicability of library in different contexts.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("system architecture and deployment topology.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Library scope definition. Defines where this library applies: systems (list of system identifiers), components (list of component names), environments (list of environment names), paths (glob patterns for file paths), metadata_tags (list of meta tags for filtering). Enables multi-system library usage with different libraries for different contexts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"library scoping",
				"multi-system library management",
				"context-aware composition",
			}).
			Validation("Object containing scope definitions. Should include: systems (array of strings), components (array of strings), environments (array of strings), paths (array of glob patterns), metadata_tags (array of strings).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("LIB-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("semantic_structure", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by semantic compression system to understand semantic stack for compression.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("semantic compression system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Semantic structure definition. Defines the semantic stack structure for this library, enabling significant data compression by understanding how components relate semantically. Contains semantic token mappings, hierarchy definitions, and compression strategies.").
			Security("non-sensitive").
			SystemUsage([]any{
				"semantic compression",
				"semantic stack navigation",
				"data compression optimization",
			}).
			Validation("Object containing semantic structure definitions. Should include semantic tokens, hierarchy, and compression metadata.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("LIB-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *LibraryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *LibraryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *LibraryBuilder) GetOntology() string {
	return "library"
}

func init() {
	builders.RegisterBuilder(NewLibraryBuilder())
}
