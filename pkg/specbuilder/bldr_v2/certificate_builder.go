package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CertificateBuilder builds the certificate spec at version v2_0_0
// File: bldr_v2/certificate_builder.go - version is encoded in package/directory name
type CertificateBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCertificateBuilder creates a new builder for certificate spec version v2_0_0
func NewCertificateBuilder() *CertificateBuilder {
	builder := &CertificateBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("certificate", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a completion certificate issued to an account after fulfilling a scope (e.g. onboarding).\\nAlpha: payload is a public-key encrypted or signed message; issuer uses instance-specific or shared CA.\\nIntended for tutorial completion: user completes curriculum, data is cleaned up, certificate is issued;\\nwith the certificate the holder can log in to the CLI and participate. Instance-specific CAs ensure\\ncerts from one instance are not valid for others where scope differs (e.g. software tester onboarding).\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addCertificateFields()

	return builder
}

// addCertificateFields adds the certificate fields
func (b *CertificateBuilder) addCertificateFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("credential_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("set when certificate is issued").
			Cardinality("one").
			Criticality("association").
			Default("signed_payload").
			Dependencies("CA/signing implementation").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of credential (signed_payload, encrypted_payload). Alpha uses self-signed CA; later may use instance-specific CA.").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
			}).
			Validation("enum").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"signed_payload",
				"encrypted_payload",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CERT-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("expires_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/certification policy").
			AutomationHooks("set at issue time from policy").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("certification policy").
			Lifecycle("mutable (renewal may extend)").
			Observability("yes").
			Purpose("ISO-8601 datetime when the certificate expires (null = no expiry for alpha)").
			Security("non-sensitive").
			SystemUsage([]any{
				"access control",
				"renewal",
			}).
			Validation("ISO-8601 datetime").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("CERT-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("holder_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/certification service").
			AutomationHooks("set when certificate is issued").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("account registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Account that completed the scope and holds this certificate (e.g. account:developer)").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"access control",
			}).
			Validation("Must match account ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(account:[a-z0-9._-]+|ACC-[A-Za-z0-9-]+)$`).
			Required(true).
			Build()).
		WithTraits("field_reference_group", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("CERT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("issued_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("set at issue time").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("system clock").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO-8601 datetime when the certificate was issued").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit",
				"expiration logic",
			}).
			Validation("ISO-8601 datetime").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("CERT-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("issuer_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("set at issue time (CA or instance identifier)").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("CA config").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Identifier of the issuer (CA id or instance id) so verification can use the correct public key. Instance-specific for alpha.").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
			}).
			Validation("Opaque string").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CERT-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("payload", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation only").
			AutomationHooks("set at issue time (signed or encrypted message)").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("CA/signing implementation").
			Lifecycle("immutable").
			Observability("no (opaque; do not log)").
			Purpose("Opaque signed or encrypted payload (e.g. JWS or public-key encrypted blob). Contains attested claims; verification uses issuer public key.").
			Security("sensitive - do not expose raw payload in logs").
			SystemUsage([]any{
				"verification",
				"login gating",
			}).
			Validation("Base64 or opaque string").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CERT-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/certification service").
			AutomationHooks("set when certificate is issued").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("certification policy").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Scope this certificate attests to (e.g. onboarding, software_tester_onboarding). May be standard across instances or instance-specific.").
			Security("non-sensitive").
			SystemUsage([]any{
				"access control",
				"gating",
			}).
			Validation("Alphanumeric and underscores").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z][a-z0-9_]*$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CERT-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CertificateBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CertificateBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CertificateBuilder) GetOntology() string {
	return "certificate"
}

func init() {
	builders.RegisterBuilder(NewCertificateBuilder())
}
