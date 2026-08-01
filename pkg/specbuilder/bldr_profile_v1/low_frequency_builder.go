package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// LowFrequencyBuilder builds the low_frequency profile at version v1_0_0
// File: bldr_profile_v1/low_frequency_builder.go - version is encoded in package/directory name
type LowFrequencyBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewLowFrequencyBuilder creates a new builder for low_frequency profile version v1_0_0
func NewLowFrequencyBuilder() *LowFrequencyBuilder {
	builder := &LowFrequencyBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("low_frequency", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeMetricsSampler).
		SetMetadata(config.ProfileMetadata{
			Name:        "low_frequency",
			Extends:     "base_sampler",
			Description: "For lower frequency events that can use smaller batches and longer intervals.\\nGlobal batching for low-frequency events.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyAppliesTo: []any{
				"command_metric",
			},
			objects.FieldKeyBatchSize:       50,
			objects.FieldKeyFlushInterval:   "10m",
			objects.FieldKeyGroupByObjectID: false,
			objects.FieldKeyMaxBatchSize:    500,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewLowFrequencyBuilder())
}
