package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// DebugBuilder builds the debug profile at version v1_0_0
// File: bldr_profile_v1/debug_builder.go - version is encoded in package/directory name
type DebugBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewDebugBuilder creates a new builder for debug profile version v1_0_0
func NewDebugBuilder() *DebugBuilder {
	builder := &DebugBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("debug", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "debug",
			Extends:     "base_profile",
			Description: "Debugging with full details.\\nYAML format for structured output with verbose logging enabled.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyFlags: map[string]any{
				objects.FieldKeyFormat: "yaml",
				"verbose":              true,
			},
			objects.FieldKeyFormat: "yaml",
			"quiet":                false,
			"verbose":              true,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewDebugBuilder())
}
