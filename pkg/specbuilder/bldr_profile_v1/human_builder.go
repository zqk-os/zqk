package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// HumanBuilder builds the human profile at version v1_0_0
// File: bldr_profile_v1/human_builder.go - version is encoded in package/directory name
type HumanBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewHumanBuilder creates a new builder for human profile version v1_0_0
func NewHumanBuilder() *HumanBuilder {
	builder := &HumanBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("human", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "human",
			Extends:     "base_profile",
			Description: "Human-friendly table output.\\nOptimized for terminal display with readable formatting.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyFlags: map[string]any{
				objects.FieldKeyFormat: "table",
			},
			objects.FieldKeyFormat: "table",
			"quiet":                false,
			"verbose":              false,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewHumanBuilder())
}
