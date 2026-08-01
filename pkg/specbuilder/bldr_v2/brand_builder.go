package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// BrandBuilder builds the brand spec at version v2_0_0
// File: bldr_v2/brand_builder.go - version is encoded in package/directory name
type BrandBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBrandBuilder creates a new builder for brand spec version v2_0_0
func NewBrandBuilder() *BrandBuilder {
	builder := &BrandBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("brand", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Brand configuration specification. Defines system branding information including brand name, variants, and substitution patterns for cross-system branding updates. Enables systematic branding changes through the substitution utility by providing declarative brand definitions with scope, patterns, and substitution targets. Supports multi-system branding through meta tags and pointers to substitution configurations. Implements the Cloneable Configuration Pattern - brands can be cloned with shared DNA propagation.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("cloneable_configuration")

	// Add fields
	builder.addBrandFields()

	return builder
}

// addBrandFields adds the brand fields
func (b *BrandBuilder) addBrandFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("brand_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system administrator").
			AutomationHooks("used by substitution utility to identify brand configurations.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("brand naming conventions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Primary brand name identifier. The canonical name for this brand (e.g., 'zqk', 'WorkstreamOS'). Used as the primary reference for branding substitutions and system identification.").
			Security("non-sensitive").
			SystemUsage([]any{
				"brand identification",
				"substitution targeting",
				"system naming",
			}).
			Validation("Must be a valid brand name string. Should follow naming conventions (e.g., PascalCase, no spaces).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(100).
			MinLength(1).
			Pattern(`^[A-Za-z][A-Za-z0-9_-]*$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r--").
		WithSemanticType("identifier").
		WithProfileCode("BRD-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("brand_variant", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system administrator").
			AutomationHooks("used to distinguish brand variants (e.g., 'primary', 'secondary', 'legacy').").
			Cardinality("one").
			Criticality("association").
			Default("primary").
			Dependencies("brand variant strategy.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Brand variant identifier. Distinguishes different variants of the same brand (e.g., 'primary', 'secondary', 'legacy', 'deprecated'). Enables multiple brand configurations for the same brand name with different scopes or contexts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"brand variant management",
				"multi-context branding",
				"brand migration",
			}).
			Validation("Must be a valid variant identifier. Common values: primary, secondary, legacy, deprecated.").
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
		WithTraits("readable", "writable").
		WithPermissions("r--").
		WithSemanticType("identifier").
		WithProfileCode("BRD-002"))
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
		WithProfileCode("BRD-008"))
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
			Purpose("Reference to the source configuration (the DNA). If null/empty, this configuration IS the DNA. If set, this is a clone that references the source DNA. Format: object ID (e.g., 'BRD-001') or object reference (e.g., 'brand:BRD-001').").
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
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("BRD-006"))
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
		WithProfileCode("BRD-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system administrator").
			AutomationHooks("used for brand metadata tagging and filtering.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("brand metadata conventions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Brand metadata. Contains metadata for tagging and filtering: tags (array of meta tags), description (brand description), legal_status (legal status indicator), effective_date (when brand becomes effective), deprecated_date (when brand is deprecated). Enables meta tag-based filtering and pointer resolution for brand substitutions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"brand metadata",
				"tag-based filtering",
				"pointer resolution",
			}).
			Validation("Object containing metadata. Should include: tags (array of strings), description (string), legal_status (string), effective_date (string/date), deprecated_date (string/date).").
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
		WithProfileCode("BRD-005"))
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
		WithProfileCode("BRD-009"))
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
		WithProfileCode("BRD-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system administrator").
			AutomationHooks("used by substitution utility to determine applicability of brand substitutions.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("system architecture and deployment topology.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Brand scope definition. Defines where this brand applies: systems (list of system identifiers), components (list of component names), environments (list of environment names), paths (glob patterns for file paths), metadata_tags (list of meta tags for filtering). Enables multi-system branding with different brands for different contexts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"brand scoping",
				"multi-system branding",
				"context-aware branding",
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
		WithProfileCode("BRD-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("substitution_patterns", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system administrator").
			AutomationHooks("used by substitution utility to load and execute brand substitutions.").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("substitution utility and substitution configuration objects.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Substitution pattern references. Array of references (IDs or paths) to substitution configuration objects that define how to replace the current brand with this brand. Can reference substitution objects by ID or file path. Enables systematic branding changes across code, documentation, and configurations.").
			Security("non-sensitive").
			SystemUsage([]any{
				"brand substitution",
				"systematic branding updates",
				"cross-system branding",
			}).
			Validation("Array of strings. Each string should reference a valid substitution object ID or file path. References should be resolvable by the substitution utility.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(1).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("BRD-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BrandBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BrandBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BrandBuilder) GetOntology() string {
	return "brand"
}

func init() {
	builders.RegisterBuilder(NewBrandBuilder())
}
