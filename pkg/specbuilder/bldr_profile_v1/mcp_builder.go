package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// McpBuilder builds the mcp profile at version v1_0_0
// File: bldr_profile_v1/mcp_builder.go - version is encoded in package/directory name
type McpBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewMcpBuilder creates a new builder for mcp profile version v1_0_0
func NewMcpBuilder() *McpBuilder {
	builder := &McpBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("mcp", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeCLIContext).
		SetMetadata(config.ProfileMetadata{
			Name:        "mcp",
			Extends:     "base_profile",
			Description: "MCP context: JSON output for machine readability, logs to stderr.\\nUsed by Model Context Protocol server for AI agent communication.\\nNote: Logging to stderr is handled by the logging framework when ZQK_MCP_ACCOUNT_ID is set.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyFlags: map[string]any{
				objects.FieldKeyContext: "mcp",
				objects.FieldKeyFormat:  "json",
			},
			objects.FieldKeyFormat: "json",
			"quiet":                false,
			"verbose":              false,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewMcpBuilder())
}
