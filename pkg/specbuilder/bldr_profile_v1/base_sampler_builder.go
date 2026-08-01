package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// BaseSamplerBuilder builds the base_sampler profile at version v1_0_0
// File: bldr_profile_v1/base_sampler_builder.go - version is encoded in package/directory name
type BaseSamplerBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewBaseSamplerBuilder creates a new builder for base_sampler profile version v1_0_0
func NewBaseSamplerBuilder() *BaseSamplerBuilder {
	builder := &BaseSamplerBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("base_sampler", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeMetricsSampler).
		SetMetadata(config.ProfileMetadata{
			Name:        "base_sampler",
			Description: "Base sampler profile that defines default metrics sampling configuration.\\nAll sampler profiles extend this base profile.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyBatchSize:       50,
			objects.FieldKeyEnabled:         true,
			objects.FieldKeyFlushInterval:   "5m",
			objects.FieldKeyGroupByObjectID: true,
			objects.FieldKeyMaxBatchSize:    1000,
			objects.FieldKeyMetricType:      "system",
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewBaseSamplerBuilder())
}
