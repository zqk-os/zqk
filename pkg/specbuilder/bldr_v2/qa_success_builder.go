package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// QaSuccessBuilder builds the qa_success spec at version v2_0_0
// File: bldr_v2/qa_success_builder.go - version is encoded in package/directory name
type QaSuccessBuilder struct {
	*builders.BaseSpecBuilder
}

// NewQaSuccessBuilder creates a new builder for qa_success spec version v2_0_0
func NewQaSuccessBuilder() *QaSuccessBuilder {
	builder := &QaSuccessBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("qa_success", "v2_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Cryptographically signed proof of a successful QA audit. Kernel-internal token (QAS-*). Distinct from criteria (satisfiable predicate) and from unsigned audit_event. Payload is item_id plus ECDSA P-256 signature and public_key (pkg/validation/qa). Instances are immutable after mint.").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	builder.addQaSuccessFields()
	return builder
}

func (b *QaSuccessBuilder) addQaSuccessFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder(objects.FieldKeyItemID, "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA auditor / pkg/validation/qa signer.").
			AutomationHooks("bound into the signed payload; id_template interpolates this field.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("signed subject object id (BLI, ATK, or other kernel id).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Identity of the object whose QA audit this token attests.").
			Security("non-sensitive").
			SystemUsage([]any{
				"signing",
				"verification",
				"filtering",
			}).
			Validation("Non-empty kernel object id; required.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			DisplayLength(64).
			Build()).
		WithTraits("filterable", "readable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("QAS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder(objects.FieldKeySignature, "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("pkg/validation/qa AuditorSigner (ECDSA P-256).").
			AutomationHooks("produced by Signer.Sign; verified on auditor gate.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("item_id, public_key, ECDSA P-256 private key.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Hex-encoded ECDSA P-256 ASN.1 signature over the audit payload.").
			Security("non-sensitive (public signature material)").
			SystemUsage([]any{
				"verification",
				"integrity",
			}).
			Validation("Non-empty hex string; required.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("QAS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder(objects.FieldKeyPublicKey, "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("pkg/validation/qa AuditorSigner.").
			AutomationHooks("persisted with the token so verification does not need a key store.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("signature, ECDSA P-256 keypair.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Hex-encoded ECDSA P-256 public key that verifies signature.").
			Security("non-sensitive (public key material)").
			SystemUsage([]any{
				"verification",
				"integrity",
			}).
			Validation("Non-empty hex string; required.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("QAS-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *QaSuccessBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *QaSuccessBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *QaSuccessBuilder) GetOntology() string {
	return "qa_success"
}

func init() {
	builders.RegisterBuilder(NewQaSuccessBuilder())
}
