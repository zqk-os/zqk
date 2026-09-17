package mcp

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// RegisterGraphTools registers graph traversal tools with the MCP server
func RegisterGraphTools(server *Server) {
	graphEnabledHint := fmt.Sprintf("%s=true", brand.EnvVar("GRAPH_ENABLED"))

	// Graph traversal tool
	server.RegisterTool(
		GetToolName("graph_traversal"),
		"Perform multi-hop graph traversal starting from a node and following relationships. Use this to explore object relationships, find connected items, or trace dependencies. Requires graph backend to be enabled ("+graphEnabledHint+"). Example: "+GetToolName("graph_traversal")+" with start_node_id='BLI-010', max_depth=2.",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"start_node_id"},
			"properties": map[string]any{
				"start_node_id": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "ID of the starting node (required)",
				},
				"relationship": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Relationship type to traverse (e.g., \"HAS_CHILD\", \"RELATES_TO\"). Optional, traverses all if not specified",
				},
				"direction": map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"outgoing", "incoming", "both"},
					"default":                   "outgoing",
					objects.FieldKeyDescription: "Traversal direction",
				},
				"max_depth": map[string]any{
					objects.FieldKeyType:        "number",
					"default":                   3,
					objects.FieldKeyDescription: "Maximum traversal depth",
				},
				"filter_labels": map[string]any{
					objects.FieldKeyType:        "array",
					objects.FieldKeyDescription: "Filter nodes by labels",
					"items": map[string]any{
						objects.FieldKeyType: "string",
					},
				},
				"filter_properties": map[string]any{
					objects.FieldKeyType:        "object",
					objects.FieldKeyDescription: "Filter nodes by properties",
				},
				"limit": map[string]any{
					objects.FieldKeyType:        "number",
					"default":                   100,
					objects.FieldKeyDescription: "Maximum number of nodes to return",
				},
			},
		},
		nil, // Handler is registered in handleToolCall
	)

	// Resolve references tool
	server.RegisterTool(
		GetToolName("resolve_references"),
		"Resolve object references (e.g., \"goal:GOAL-123\", \"milestone:MIL-456\") to actual objects. Use this to expand reference strings found in object fields into full object data. Useful for understanding relationships and dependencies. Requires graph backend to be enabled ("+graphEnabledHint+"). Example: "+GetToolName("resolve_references")+" with references=['goal:GOAL-001', 'milestone:MIL-022'].",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"references"},
			"properties": map[string]any{
				"references": map[string]any{
					objects.FieldKeyType:        "array",
					objects.FieldKeyDescription: "List of references to resolve (required)",
					"items": map[string]any{
						objects.FieldKeyType: "string",
					},
				},
				"include_related": map[string]any{
					objects.FieldKeyType:        "boolean",
					"default":                   false,
					objects.FieldKeyDescription: "Include related objects",
				},
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"json", "yaml"},
					"default":                   "json",
					objects.FieldKeyDescription: "Output format",
				},
			},
		},
		nil,
	)

	// State-aware query tool
	server.RegisterTool(
		GetToolName("state_aware_query"),
		"Perform queries that are aware of object lifecycle states and relationships. Use this to find active items, blocked items, dependencies, or track progress. Query types: active_items, blocked_items, dependencies, progress. Requires graph backend to be enabled ("+graphEnabledHint+"). Example: "+GetToolName("state_aware_query")+" with query_type='active_items', format='json'.",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"query_type"},
			"properties": map[string]any{
				"query_type": map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"active_items", "blocked_items", "dependencies", "progress"},
					objects.FieldKeyDescription: "Type of query",
				},
				"filters": map[string]any{
					objects.FieldKeyType:        "object",
					objects.FieldKeyDescription: "Additional filters (status, kind, date_range, etc.)",
				},
				"include_metrics": map[string]any{
					objects.FieldKeyType:        "boolean",
					"default":                   false,
					objects.FieldKeyDescription: "Include calculated metrics",
				},
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"json", "yaml", "markdown"},
					"default":                   "json",
					objects.FieldKeyDescription: "Output format",
				},
			},
		},
		nil,
	)
}

// RegisterAllTools registers all available tools (graph and metrics)
// Note: Metrics tools are now bootstrapped automatically from context
// during server initialization, not manually registered here
func RegisterAllTools(server *Server) {
	RegisterGraphTools(server)
	RegisterEchoTool(server)            // Test tool for MCP communication validation
	RegisterChatInjectTool(server)      // Chat integration tool
	RegisterIdeBridgeTool(server)       // IDE bridge control bus (zqk-ide-bridge extension)
	RegisterCommonTools(server)         // Common CLI commands as built-in tools (always available)
	RegisterAgentExecutionTools(server) // Quantum Sandbox tool for determinisitic evaluation
	RegisterObserverTools(server)       // Live Go AST search (do not persist a census)
	RegisterVerificationTools(server)   // Reusable async trigger for verification
	RegisterInteractiveTools(server)    // Interactive object creation tools
	RegisterProjectContextTool(server)  // P0-3: Project context onboarding tool
	RegisterMetricsTools(server)        // Metrics tools for observability

	// Register all onboarding prompts
	RegisterOnboardingPrompts(server)

	RegisterExternalTools(server) // Tools for handling massive blobs and external transit

	// Register critical resources (lifecycles, workflows, system health)
	// Resources are registered dynamically - only if files exist
	RegisterCriticalResources(server)

	// Discover additional resources dynamically from docs directory
	// This allows new documentation to be automatically available
	DiscoverAdditionalResources(server)
}

// RegisterAllToolsWithSecurityContext registers all available tools with security context
// This allows role-based filtering of workflow tools
func RegisterAllToolsWithSecurityContext(server *Server, secCtx *pkgctx.SecurityContext) {
	RegisterGraphTools(server)
	RegisterEchoTool(server)              // Test tool for MCP communication validation
	RegisterChatInjectTool(server)        // Chat integration tool
	RegisterIdeBridgeTool(server)         // IDE bridge control bus (zqk-ide-bridge extension)
	RegisterCommonTools(server)           // Common CLI commands as built-in tools (always available)
	RegisterAgentExecutionTools(server)   // Quantum Sandbox tool for determinisitic evaluation
	RegisterObserverTools(server)         // Live Go AST search (do not persist a census)
	RegisterInteractiveTools(server)      // Interactive object creation tools
	RegisterWorkflowTools(server, secCtx) // Workflow-aware tools (role-based)
	RegisterProjectContextTool(server)    // P0-3: Project context onboarding tool
	RegisterMetricsTools(server)          // Metrics tools for observability

	// Register all onboarding prompts
	RegisterOnboardingPrompts(server)

	RegisterExternalTools(server) // Tools for handling massive blobs and external transit

	// Register critical resources (lifecycles, workflows, system health)
	// Resources are registered dynamically - only if files exist
	RegisterCriticalResources(server)

	// Discover additional resources dynamically from docs directory
	// This allows new documentation to be automatically available
	DiscoverAdditionalResources(server)
}
