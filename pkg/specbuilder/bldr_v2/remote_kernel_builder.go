package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RemoteKernelBuilder builds the remote_kernel spec at version v2_0_0
// File: bldr_v2/remote_kernel_builder.go - version is encoded in package/directory name
type RemoteKernelBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRemoteKernelBuilder creates a new builder for remote_kernel spec version v2_0_0
func NewRemoteKernelBuilder() *RemoteKernelBuilder {
	builder := &RemoteKernelBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("remote_kernel", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a remote ZQK kernel within the Federated Sovereign Mesh.\\nStores connection details, security primitives, and shared capability metadata\\nto enable cross-node collaboration and context exchange.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addRemoteKernelFields()

	return builder
}

// addRemoteKernelFields adds the remote_kernel fields
func (b *RemoteKernelBuilder) addRemoteKernelFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("capabilities", "list").
		WithTraits("readable", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("endpoint", "string").
		WithTraits("readable", "writable").
		WithSemanticType("location"))
	b.AddFieldBuilder(builders.NewFieldBuilder("infrastructure_adapter_refs", "list").
		WithTraits("field_reference_group", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_heartbeat", "timestamp").
		WithTraits("readable").
		WithSemanticType("time"))
	b.AddFieldBuilder(builders.NewFieldBuilder("public_key", "string").
		WithTraits("readable", "writable").
		WithSemanticType("credential"))
	b.AddFieldBuilder(builders.NewFieldBuilder("shared_namespaces", "list").
		WithTraits("readable", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("trust_level", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"untrusted",
				"verified",
				"sovereign",
				"partner",
			}).
			Build()).
		WithDefault("untrusted").
		WithTraits("readable", "writable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RemoteKernelBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RemoteKernelBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RemoteKernelBuilder) GetOntology() string {
	return "remote_kernel"
}

func init() {
	builders.RegisterBuilder(NewRemoteKernelBuilder())
}
