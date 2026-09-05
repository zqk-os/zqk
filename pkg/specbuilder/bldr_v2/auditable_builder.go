package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AuditableBuilder builds the auditable spec at version v2_0_0
// File: bldr_v2/auditable_builder.go - version is encoded in package/directory name
type AuditableBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAuditableBuilder creates a new builder for auditable spec version v2_0_0
func NewAuditableBuilder() *AuditableBuilder {
	builder := &AuditableBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("auditable", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("null").
		SetDescription("Core audit metadata inherited by every system object. Ensures provenance, attribution, and lifecycle tracking are available everywhere. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_auditable_traits")

	// Add fields
	builder.addAuditableFields()

	return builder
}

// addAuditableFields adds the auditable fields
func (b *AuditableBuilder) addAuditableFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("archived_at", "datetime").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("role with archive permission.").
			AutomationHooks("triggers archival workflows.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("unused until archived").
			Dependencies("archive policies.").
			Lifecycle("append-only (set once when archived).").
			Observability("logged.").
			Purpose("Timestamp when object was archived (if applicable).").
			Security("non-sensitive").
			SystemUsage([]any{
				"retention",
				"visibility",
			}).
			Validation("ISO-8601.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("archived_by", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("archive-capable roles.").
			AutomationHooks("ensures only authorized actors archive.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("unused until archived").
			Dependencies("review logs.").
			Lifecycle("append-only").
			Observability("logged.").
			Purpose("Who archived the object.").
			Security("limited PII.").
			SystemUsage([]any{
				"audit",
				"approvals",
			}).
			Validation("user registry.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_log", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for change journals and manifest hashing.").
			Cardinality("many").
			Criticality("composition").
			Default("starts empty, append per change").
			Dependencies("rollback, manifests, audits.").
			Lifecycle("append-only").
			Observability("persisted outside git as well.").
			Purpose("Append-only record of meaningful changes.").
			Security("may include commit hashes; non-sensitive otherwise.").
			SystemUsage([]any{
				"rollback",
				"analytics",
			}).
			Validation("each entry includes timestamp, actor, fingerprint, summary.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:audit").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("created_at", "datetime").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("creation events trigger journal entry.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-generated when object is created").
			Dependencies("change_log initialization.").
			Lifecycle("immutable").
			Observability("recorded in journal + manifests.").
			Purpose("Timestamp when the object was first materialized.").
			Security("non_pii").
			SystemUsage([]any{
				"ordering",
				"lifecycle",
			}).
			Validation("ISO-8601 datetime enforced by generator.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("created_by", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to verify permissions for future edits.").
			Cardinality("one").
			Criticality("composition").
			Default("resolved from current CLI login").
			Dependencies("authority checks, rollback reports.").
			Lifecycle("immutable").
			Observability("audited.").
			Purpose("Identifies the user/profile that created the object.").
			Security("may contain user handle (limited PII) – respect masking rules.").
			SystemUsage([]any{
				"attribution",
				"permissions",
			}).
			Validation("must match user registry entry.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:audit").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(1).
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("origin_project", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/importer").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("association").
			Default("current project name").
			Dependencies("cross-project analysis.").
			Lifecycle("immutable").
			Observability("visible in exports.").
			Purpose("Human-readable project/workstream name where the object began.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"migration",
			}).
			Validation("freeform string but encouraged to match registry.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("origin_system", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/importer").
			AutomationHooks("used when exporting/importing graphs.").
			Cardinality("one").
			Criticality("association").
			Default("current repo identifier").
			Dependencies("signed artifact verification.").
			Lifecycle("immutable").
			Observability("stored in manifests.").
			Purpose("Where the object was originally authored (repo, external system).").
			Security("non-sensitive").
			SystemUsage([]any{
				"import validation",
				"signature verification",
			}).
			Validation("enumerated list or URI.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("updated_at", "datetime").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to detect drift/staleness.").
			Cardinality("one").
			Criticality("composition").
			Default("same as created_at until first update").
			Dependencies("change_log, sync triggers.").
			Lifecycle("mutable (overwrites)").
			Observability("always logged.").
			Purpose("Last time any field changed.").
			Security("non_pii").
			SystemUsage([]any{
				"ordering",
				"cache invalidation",
			}).
			Validation("ISO-8601.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("updated_by", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("helps detect unauthorized edits.").
			Cardinality("one").
			Criticality("composition").
			Default("created_by until first update").
			Dependencies("review workflows.").
			Lifecycle("mutable").
			Observability("logged.").
			Purpose("User/profile responsible for last change.").
			Security("limited PII").
			SystemUsage([]any{
				"permissions",
				"notifications",
			}).
			Validation("must map to known user.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:audit").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(1).
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUD-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AuditableBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AuditableBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AuditableBuilder) GetOntology() string {
	return "auditable"
}

func init() {
	builders.RegisterBuilder(NewAuditableBuilder())
}
