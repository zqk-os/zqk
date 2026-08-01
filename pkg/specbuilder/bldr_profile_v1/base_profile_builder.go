package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// BaseProfileBuilder builds the base_profile profile at version v1_0_0
// File: bldr_profile_v1/base_profile_builder.go - version is encoded in package/directory name
type BaseProfileBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewBaseProfileBuilder creates a new builder for base_profile profile version v1_0_0
func NewBaseProfileBuilder() *BaseProfileBuilder {
	builder := &BaseProfileBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("base_profile", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "base_profile",
			Description: "Base profile specification that defines the structure for all context profiles.\\nProfiles define flag bundles, command variants, and output preferences for different use cases.\\n",
		}).
		SetSpec(map[string]any{
			"commands":             map[string]any{},
			objects.FieldKeyFlags:  map[string]any{},
			objects.FieldKeyFormat: "table",
			"quiet":                false,
			objects.FieldKeyStorage: map[string]any{
				"default_page_size": 0,
				"enable_grouping":   false,
				"max_group_size":    0,
				"max_page_size":     0,
			},
			"verbose": false,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewBaseProfileBuilder())
}
