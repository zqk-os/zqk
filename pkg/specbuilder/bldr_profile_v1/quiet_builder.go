package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// QuietBuilder builds the quiet profile at version v1_0_0
// File: bldr_profile_v1/quiet_builder.go - version is encoded in package/directory name
type QuietBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewQuietBuilder creates a new builder for quiet profile version v1_0_0
func NewQuietBuilder() *QuietBuilder {
	builder := &QuietBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("quiet", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "quiet",
			Extends:     "base_profile",
			Description: "Quiet profile: automatically filters out terminal statuses (complete, archived, closed, etc.).\\nOptimized for focusing on active work items and suppressing completed/archived items.\\nUses table format with minimal verbosity.\\n",
		}).
		SetSpec(map[string]any{
			"commands": map[string]any{
				"aliases": map[string]any{
					"backlog": "backlog list",
					"list":    "object list",
				},
			},
			objects.FieldKeyFlags: map[string]any{
				objects.FieldKeyFormat: "table",
				"quiet":                true,
			},
			objects.FieldKeyFormat: "table",
			"quiet":                true,
			"verbose":              false,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewQuietBuilder())
}
