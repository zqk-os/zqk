package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// McpBuiltInToolLifecycleBuilder builds the mcp_built_in_tool lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/mcp_built_in_tool_builder.go - version is encoded in package/directory name
type McpBuiltInToolLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewMcpBuiltInToolLifecycleBuilder creates a new builder for mcp_built_in_tool lifecycle version v1_0_0
func NewMcpBuiltInToolLifecycleBuilder() *McpBuiltInToolLifecycleBuilder {
	builder := &McpBuiltInToolLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("mcp_built_in_tool", "v1_0_0"),
	}

	// Add statuses and transitions
	builder.addMcpBuiltInToolLifecycleData()

	return builder
}

// addMcpBuiltInToolLifecycleData adds the mcp_built_in_tool lifecycle statuses and transitions
func (b *McpBuiltInToolLifecycleBuilder) addMcpBuiltInToolLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "proposed",
		Display: "Proposed",
	})
	b.AddStatus(objects.Status{
		Value:   "under_review",
		Display: "Under Review",
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
	})
	b.AddStatus(objects.Status{
		Value:   "deprecated",
		Display: "Deprecated",
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "proposed",
		Description: "Propose tool for implementation",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"tool name defined",
			"use case documented",
			"benefits identified",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "proposed",
		To:          "under_review",
		Description: "Submit for review and approval",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"implementation plan documented",
			"usage metrics analyzed",
			"conversion criteria met",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "under_review",
		To:          "active",
		Description: "Approve and activate tool",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"code implemented",
			"tests written",
			"documentation updated",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "under_review",
		Description: "Tool updated - return to review",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "deprecated",
		Description: "Deprecate tool (replaced or no longer needed)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "under_review",
		Description: "Quarterly review reminder (POLICY-MCP-001)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Manual archival",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewMcpBuiltInToolLifecycleBuilder())
}
