package main

import (
	"errors"
	"flag"
	"os"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ServerConfig holds command-line configuration for mcp-simple.
type ServerConfig struct {
	MTLSAddr string
	CertFile string
	KeyFile  string
	CAFile   string
}

// parseServerConfig parses flags from argument slice.
func parseServerConfig(args []string) (ServerConfig, error) {
	fs := flag.NewFlagSet("mcp-simple", flag.ContinueOnError)
	mtlsAddr := fs.String("mtls-addr", "", "Address to listen on for mTLS (e.g. :8443)")
	certFile := fs.String("cert", "", "Path to TLS certificate file")
	keyFile := fs.String("key", "", "Path to TLS key file")
	caFile := fs.String("ca", "", "Path to CA certificate file")
	if err := fs.Parse(args); err != nil {
		return ServerConfig{}, err
	}

	cfg := ServerConfig{
		MTLSAddr: *mtlsAddr,
		CertFile: *certFile,
		KeyFile:  *keyFile,
		CAFile:   *caFile,
	}

	if cfg.MTLSAddr != "" {
		if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
			return cfg, errors.New("mTLS requires -cert, -key, and -ca flags")
		}
	}
	return cfg, nil
}

// buildServer constructs and configures a minimal MCP server instance.
func buildServer(projectRoot string) *mcp.Server {
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
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: projectRoot,
	}
	server.SetCliInitializationContext(initCtx)
	return server
}

// serverRunner abstracts the serving interface of mcp.Server for testing.
type serverRunner interface {
	Serve() error
	ServeMTLS(addr, cert, key, ca string) error
}

// runWithServer runs the server with the given configuration.
func runWithServer(cfg ServerConfig, server serverRunner) error {
	if cfg.MTLSAddr != "" {
		return server.ServeMTLS(cfg.MTLSAddr, cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	}
	return server.Serve()
}

var serverFactory = func(projectRoot string) serverRunner {
	return buildServer(projectRoot)
}

var osExit = os.Exit

func run(args []string) error {
	cfg, err := parseServerConfig(args)
	if err != nil {
		return err
	}

	projectRoot := cli.ResolveProjectRoot(".")
	server := serverFactory(projectRoot)
	return runWithServer(cfg, server)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Error("MCP server error", err).Log()
		osExit(1)
	}
}

// registerMinimalTools registers 1 tool of each type for minimal testing.
func registerMinimalTools(server *mcp.Server) {
	registerEchoTool(server)
	registerGraphTraversalTool(server)
	registerObjectListTool(server)
	registerPriorityPlanTool(server)
	registerInteractiveCreationTool(server)
}

func registerEchoTool(server *mcp.Server) {
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
}

func registerGraphTraversalTool(server *mcp.Server) {
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
}

func registerObjectListTool(server *mcp.Server) {
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
}

func registerPriorityPlanTool(server *mcp.Server) {
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
}

func registerInteractiveCreationTool(server *mcp.Server) {
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
