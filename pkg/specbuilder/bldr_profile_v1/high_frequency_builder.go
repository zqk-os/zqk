package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// HighFrequencyBuilder builds the high_frequency profile at version v1_0_0
// File: bldr_profile_v1/high_frequency_builder.go - version is encoded in package/directory name
type HighFrequencyBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewHighFrequencyBuilder creates a new builder for high_frequency profile version v1_0_0
func NewHighFrequencyBuilder() *HighFrequencyBuilder {
	builder := &HighFrequencyBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("high_frequency", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeMetricsSampler).
		SetMetadata(config.ProfileMetadata{
			Name:        "high_frequency",
			Extends:     "base_sampler",
			Description: "For very high-frequency events (e.g., file locks during contention).\\nUses larger batches and shorter flush intervals.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyAppliesTo: []any{
				"file_lock_metric",
			},
			objects.FieldKeyBatchSize:       200,
			objects.FieldKeyFlushInterval:   "2m",
			objects.FieldKeyGroupByObjectID: true,
			objects.FieldKeyMaxBatchSize:    2000,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewHighFrequencyBuilder())
}
