package profile_builders

import (
	"maps"

	"github.com/zqk-os/zqk/pkg/config"
)

// BaseProfileBuilder provides common functionality for profile builders
type BaseProfileBuilder struct {
	name          string
	version       string
	schemaVersion string
	kind          string
	profileType   config.ProfileType
	metadata      config.ProfileMetadata
	spec          map[string]any
}

// NewBaseProfileBuilder creates a new base profile builder
func NewBaseProfileBuilder(name, version string) *BaseProfileBuilder {
	return &BaseProfileBuilder{
		name:          name,
		version:       version,
		schemaVersion: DefaultProfileSchemaVersion,
		kind:          "profile",
		spec:          make(map[string]any),
	}
}

// SetSchemaVersion sets the schema version
func (b *BaseProfileBuilder) SetSchemaVersion(version string) *BaseProfileBuilder {
	b.schemaVersion = version
	return b
}

// SetKind sets the kind
func (b *BaseProfileBuilder) SetKind(kind string) *BaseProfileBuilder {
	b.kind = kind
	return b
}

// SetType sets the profile type
func (b *BaseProfileBuilder) SetType(profileType config.ProfileType) *BaseProfileBuilder {
	b.profileType = profileType
	return b
}

// SetMetadata sets the metadata
func (b *BaseProfileBuilder) SetMetadata(metadata config.ProfileMetadata) *BaseProfileBuilder {
	b.metadata = metadata
	return b
}

// SetSpec sets the spec (domain-specific configuration)
func (b *BaseProfileBuilder) SetSpec(spec map[string]any) *BaseProfileBuilder {
	b.spec = spec
	return b
}

// Build builds the profile
func (b *BaseProfileBuilder) Build() *config.UnifiedProfile {
	profile := &config.UnifiedProfile{
		SchemaVersion: b.schemaVersion,
		Kind:          b.kind,
		Type:          b.profileType,
		Metadata:      b.metadata,
	}

	// Shallow copy spec and resolved spec (same contract as prior hand loops; use stdlib maps.Copy).
	profile.Spec = make(map[string]any)
	maps.Copy(profile.Spec, b.spec)
	profile.ResolvedSpec = make(map[string]any)
	maps.Copy(profile.ResolvedSpec, profile.Spec)

	return profile
}

// GetVersion returns the version
func (b *BaseProfileBuilder) GetVersion() string {
	return b.version
}

// GetName returns the profile name
func (b *BaseProfileBuilder) GetName() string {
	return b.name
}
