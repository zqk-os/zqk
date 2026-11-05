package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ApprovalRequestBuilder builds the approval_request spec at version v2_0_0
type ApprovalRequestBuilder struct {
	*builders.BaseSpecBuilder
}

// NewApprovalRequestBuilder creates a new builder for approval_request spec version v2_0_0
func NewApprovalRequestBuilder() *ApprovalRequestBuilder {
	builder := &ApprovalRequestBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("approval_request", "v2_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Cryptographic Human-in-the-Loop Mutation Approval Request").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	builder.addApprovalRequestFields()

	return builder
}

func (b *ApprovalRequestBuilder) addApprovalRequestFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("type", "string").
		WithTraits("readable", "writable").
		WithSemanticType("statement"))

	b.AddFieldBuilder(builders.NewFieldBuilder("target", "string").
		WithTraits("readable", "writable").
		WithSemanticType("identifier"))

	b.AddFieldBuilder(builders.NewFieldBuilder("payload", "string").
		WithTraits("readable", "writable").
		WithSemanticType("statement"))

	b.AddFieldBuilder(builders.NewFieldBuilder("payload_hash", "string").
		WithTraits("readable", "writable").
		WithSemanticType("digest"))

	b.AddFieldBuilder(builders.NewFieldBuilder("hash", "string").
		WithTraits("readable", "writable").
		WithSemanticType("digest"))

	b.AddFieldBuilder(builders.NewFieldBuilder("reviewer", "string").
		WithTraits("readable", "writable").
		WithSemanticType("identifier"))

	b.AddFieldBuilder(builders.NewFieldBuilder("reviewed_at", "string").
		WithTraits("readable", "writable").
		WithSemanticType("time"))

	b.AddFieldBuilder(builders.NewFieldBuilder("reason", "string").
		WithTraits("readable", "writable").
		WithSemanticType("statement"))
}

func (b *ApprovalRequestBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *ApprovalRequestBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *ApprovalRequestBuilder) GetOntology() string {
	return "approval_request"
}

func init() {
	builders.RegisterBuilder(NewApprovalRequestBuilder())
}
