package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// IntegrityManifestBuilder builds the integrity_manifest spec at version v2_0_0
// File: bldr_v2/integrity_manifest_builder.go - version is encoded in package/directory name
type IntegrityManifestBuilder struct {
	*builders.BaseSpecBuilder
}

// NewIntegrityManifestBuilder creates a new builder for integrity_manifest spec version v2_0_0
func NewIntegrityManifestBuilder() *IntegrityManifestBuilder {
	builder := &IntegrityManifestBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("integrity_manifest", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Snapshot of critical files/objects (e.g., doc_index, project decisions) with hashes for integrity verification. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addIntegrityManifestFields()

	return builder
}

// addIntegrityManifestFields adds the integrity_manifest fields
func (b *IntegrityManifestBuilder) addIntegrityManifestFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("entries", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used during verification.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("hash calculation.").
			Lifecycle("immutable once published.").
			Observability("yes").
			Purpose("Hash entries (target reference + hash + timestamp).").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
			}).
			Validation("list of entry objects.").
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
		WithProfileCode("MAN-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("hash_algorithm", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("instructs verifier.").
			Cardinality("one").
			Criticality("composition").
			Default("sha256").
			Dependencies("hash verification.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Algorithm used (sha256, blake3, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
			}).
			Validation("known algorithms.").
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
		WithProfileCode("MAN-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin.").
			AutomationHooks("used during verification.").
			Cardinality("many (>=1)").
			Criticality("composition").
			Default("required").
			Dependencies("integrity tooling.").
			Lifecycle("immutable once published.").
			Observability("yes").
			Purpose("Files/objects included in the manifest (paths, object IDs).").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
			}).
			Validation("array of references or paths.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("MAN-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *IntegrityManifestBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *IntegrityManifestBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *IntegrityManifestBuilder) GetOntology() string {
	return "integrity_manifest"
}

func init() {
	builders.RegisterBuilder(NewIntegrityManifestBuilder())
}
