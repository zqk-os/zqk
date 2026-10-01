package main

import (
	"errors"
	"flag"
	"os"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

func main() {
	mtlsAddr := flag.String("mtls-addr", "", "Address to listen on for mTLS (e.g. :8443)")
	certFile := flag.String("cert", "", "Path to TLS certificate file")
	keyFile := flag.String("key", "", "Path to TLS key file")
	caFile := flag.String("ca", "", "Path to CA certificate file")
	flag.Parse()

	brand.SetExecutableName("zqk")
	// Create minimal MCP server - no timeout hooks, no CLI tool registration, no complex initialization
	server := mcp.NewServer()

	// Register 1 tool of each type for troubleshooting
	registerMinimalTools(server)

	// Register agent execution tools (execute_bash, read_file, write_file, etc.)
	mcp.RegisterAgentExecutionTools(server)

	// Register schema resources (JSON-LD ontologies)
	// These are useful for agents to understand object schemas and CLI structure
	mcp.RegisterSchemaResources(server)

	// Register onboarding prompts for testing
	mcp.RegisterOnboardingPrompts(server)

	// Create minimal security context (system context)
	secCtx := pkgctx.NewSystemSecurityContext()
	server.SetSecurityContext(secCtx)

	// Set minimal initialization context
	// Try to find project root by looking for .zqk directory
	projectRoot := cli.ResolveProjectRoot(".")
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: projectRoot,
	}
	server.SetCliInitializationContext(initCtx)

	if *mtlsAddr != "" {
		if *certFile == "" || *keyFile == "" || *caFile == "" {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Error("mTLS requires -cert, -key, and -ca flags", errors.New("missing flags")).Log()
			os.Exit(1)
		}
		if err := server.ServeMTLS(*mtlsAddr, *certFile, *keyFile, *caFile); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Error("MCP mTLS server error", err).Log()
			os.Exit(1)
		}
	} else {
		// Start serving - reads from stdin, writes to stdout
		if err := server.Serve(); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Error("MCP server error", err).Log()
			os.Exit(1)
		}
	}
}

// registerMinimalTools registers 1 tool of each type for minimal testing
// These tools are already handled in server.go's handleToolCallWithContext switch statement
func registerMinimalTools(server *mcp.Server) {
	// 1. Echo tool (test tool)
	server.RegisterTool(
		"zqk_test_echo",
		"Test tool that echoes a message back",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"message": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Message to echo back",
				},
			},
			"required": []string{"message"},
		},
		nil,
	)

	// 2. Graph tool (1 of 3 - graph_traversal)
	server.RegisterTool(
		"zqk_graph_traversal",
		"Perform multi-hop graph traversal starting from a node",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"start_node_id"},
			"properties": map[string]any{
				"start_node_id": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "ID of the starting node",
				},
				"max_depth": map[string]any{
					objects.FieldKeyType:        "number",
					"default":                   3,
					objects.FieldKeyDescription: "Maximum traversal depth",
				},
			},
		},
		nil,
	)

	// 3. Common tool (1 of 5 - object_list)
	server.RegisterTool(
		"zqk_object_list",
		"List objects with filtering and pagination",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyKind: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Object kind to filter by",
				},
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"json", "yaml", "table"},
					"default":                   "json",
					objects.FieldKeyDescription: "Output format",
				},
			},
		},
		nil,
	)

	// 4. Workflow tool (1 of 4 - get_current_priority_plan)
	server.RegisterTool(
		"zqk_get_current_priority_plan",
		"Get the current active priority plan",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        "string",
					"enum":                      []string{"json", "yaml", "table"},
					"default":                   "json",
					objects.FieldKeyDescription: "Output format",
				},
			},
		},
		nil,
	)

	// 5. Interactive tool (create_object_interactive)
	server.RegisterTool(
		"zqk_create_object_interactive",
		"Interactively create an object with guided field collection",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"kind"},
			"properties": map[string]any{
				objects.FieldKeyKind: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Object kind to create (e.g., 'backlog_item', 'milestone')",
				},
				objects.FieldKeySessionID: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Session ID for continuing an existing session (optional)",
				},
			},
			"additionalProperties": true,
		},
		nil,
	)
}
