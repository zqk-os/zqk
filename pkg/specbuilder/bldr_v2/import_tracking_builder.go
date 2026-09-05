package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ImportTrackingBuilder builds the import_tracking spec at version v2_0_0
// File: bldr_v2/import_tracking_builder.go - version is encoded in package/directory name
type ImportTrackingBuilder struct {
	*builders.BaseSpecBuilder
}

// NewImportTrackingBuilder creates a new builder for import_tracking spec version v2_0_0
func NewImportTrackingBuilder() *ImportTrackingBuilder {
	builder := &ImportTrackingBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("import_tracking", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Tracks ontology and schema imports (RDF/OWL, JSON Schema, etc.). Records source file, format, and status for traceability and future translation (BLI-764).\\nLifecycle: import_tracking_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("listable").
		AddTrait("readable").
		AddTrait("writable").
		AddTrait("modifiable").
		AddTrait("formatable").
		AddTrait("filterable").
		AddTrait("sortable").
		AddTrait("searchable")

	// Add fields
	builder.addImportTrackingFields()

	return builder
}

// addImportTrackingFields adds the import_tracking fields
func (b *ImportTrackingBuilder) addImportTrackingFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^IMPTRK-\d{3,}$`).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("imported_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for import history and ordering").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("system clock").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp when the import was recorded").
			Security("non-sensitive").
			SystemUsage([]any{
				"import history",
				"audit",
			}).
			Validation("ISO-8601 datetime string").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("IMPTRK-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_file", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used by ontology import and translation").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("file system path").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Path or identifier of the source file that was imported").
			Security("non-sensitive").
			SystemUsage([]any{
				"import tracking",
				"translation engine",
			}).
			Validation("Non-empty string").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("IMPTRK-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_format", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for format detection and translation routing").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("ontology import").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Detected or specified format (e.g. turtle, rdf_owl, jsonld)").
			Security("non-sensitive").
			SystemUsage([]any{
				"import tracking",
				"translation engine",
			}).
			Validation("Format key string").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("IMPTRK-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for import pipeline and translation (BLI-764)").
			Cardinality("one").
			Criticality("composition").
			Default("ready").
			Dependencies("ontology import, translation engine").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Import status (ready, translated, failed)").
			Security("non-sensitive").
			SystemUsage([]any{
				"import tracking",
				"translation engine",
			}).
			Validation("Enum").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"ready",
				"translated",
				"failed",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("IMPTRK-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ImportTrackingBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ImportTrackingBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ImportTrackingBuilder) GetOntology() string {
	return "import_tracking"
}

func init() {
	builders.RegisterBuilder(NewImportTrackingBuilder())
}
