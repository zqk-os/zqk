package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// AiAgentBuilder builds the ai_agent profile at version v1_0_0
// File: bldr_profile_v1/ai_agent_builder.go - version is encoded in package/directory name
type AiAgentBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewAiAgentBuilder creates a new builder for ai_agent profile version v1_0_0
func NewAiAgentBuilder() *AiAgentBuilder {
	builder := &AiAgentBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("ai_agent", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "ai_agent",
			Extends:     "base_profile",
			Description: "Machine-readable output for AI agents.\\nOptimized for programmatic consumption with JSON format and minimal verbosity.\\n",
		}).
		SetSpec(map[string]any{
			"commands": map[string]any{
				"aliases": map[string]any{
					"get":  "object get --format jsonl",
					"list": "object list --format jsonl",
				},
			},
			objects.FieldKeyFlags: map[string]any{
				objects.FieldKeyFormat: "jsonl",
				"verbose":              false,
			},
			objects.FieldKeyFormat: "jsonl",
			"quiet":                false,
			"verbose":              false,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewAiAgentBuilder())
}
