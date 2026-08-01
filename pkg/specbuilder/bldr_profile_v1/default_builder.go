package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// DefaultBuilder builds the default profile at version v1_0_0
// File: bldr_profile_v1/default_builder.go - version is encoded in package/directory name
type DefaultBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewDefaultBuilder creates a new builder for default profile version v1_0_0
func NewDefaultBuilder() *DefaultBuilder {
	builder := &DefaultBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("default", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeMetricsSampler).
		SetMetadata(config.ProfileMetadata{
			Name:        "default",
			Extends:     "base_sampler",
			Description: "Default sampler profile for metric-enabled objects.\\nUsed when no specific profile matches.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyBatchSize:       50,
			objects.FieldKeyFlushInterval:   "5m",
			objects.FieldKeyGroupByObjectID: true,
			objects.FieldKeyIsDefault:       true,
			objects.FieldKeyMaxBatchSize:    1000,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewDefaultBuilder())
}
