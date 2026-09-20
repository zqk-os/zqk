package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
)

const (
	observerSearchToolSuffix   = "observer_search"
	observerRegisterToolSuffix = "observer_register_agent"
	observerSearchMaxHits      = 20
)

// RegisterObserverTools exposes live Go AST search and agent registration.
func RegisterObserverTools(server *Server) {
	server.RegisterTool(
		GetToolName(observerSearchToolSuffix),
		"Search the live Go AST for functions, methods, types, and structs. Call this BEFORE inventing a file or package. Pass name (symbol substring) and/or path (pkg/... or cmd/... file or dir). Returns file, line, kind, and signature. Then read_code the returned path.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyName: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Symbol substring (e.g. BranchRef, UnpaidLookupStreak)",
				},
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional pkg/ or cmd/ file or directory to scope the walk",
				},
				objects.FieldKeyKind: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional entity kind: function, method, type, interface, struct",
				},
				mcpToolArgKeyLimit: map[string]any{
					objects.FieldKeyType:        "number",
					objects.FieldKeyDescription: "Maximum hits (default 20, max 50)",
				},
			},
		},
		server.handleObserverSearchTool,
	)

	server.RegisterTool(
		GetToolName(observerRegisterToolSuffix),
		"Automatically discover and register a new agent into the system config.yaml",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"account_id": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Account ID of the agent (e.g. 'ACC-...'). Can also be client_id",
				},
				"roles": map[string]any{
					objects.FieldKeyType:        "array",
					"items":                     map[string]any{"type": "string"},
					objects.FieldKeyDescription: "Expected roles for the agent",
				},
				"profile": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Expected profile (e.g. 'ai-agent', 'mcp')",
				},
			},
			"required": []string{"account_id"},
		},
		server.handleObserverRegisterTool,
	)
}

// HandleObserverSearch runs a scoped AST query against the sandbox root.
func HandleObserverSearch(ctx context.Context, root string, args map[string]any) (any, error) {
	name, _ := args[objects.FieldKeyName].(string)
	path, _ := args[objects.FieldKeyPath].(string)
	kind, _ := args[objects.FieldKeyKind].(string)
	if strings.TrimSpace(name) == "" && strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("observer_search requires name or path")
	}
	q := observer.Query{
		Root:  root,
		Name:  name,
		Path:  path,
		Kind:  kind,
		Limit: observerSearchLimit(args),
	}
	hits, err := observer.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return "No AST hits. Try a shorter name or a pkg/ / cmd/ path, then read_code the real file.", nil
	}
	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		lines = append(lines, observer.FormatHit(hit))
	}
	return strings.Join(lines, "\n"), nil
}

func observerSearchLimit(args map[string]any) int {
	switch v := args[mcpToolArgKeyLimit].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return observerSearchMaxHits
	}
}

func (s *Server) handleObserverSearchTool(ctx context.Context, args map[string]any) (any, error) {
	return HandleObserverSearch(ctx, s.fileSandboxRoot(), args)
}

func (s *Server) handleObserverRegisterTool(ctx context.Context, args map[string]any) (any, error) {
	accountID, ok := args["account_id"].(string)
	if !ok || accountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	var roles []string
	if rawRoles, ok := args["roles"].([]any); ok {
		for _, r := range rawRoles {
			if str, ok := r.(string); ok {
				roles = append(roles, str)
			}
		}
	}

	profile, _ := args["profile"].(string)
	if profile == "" {
		profile = "ai-agent"
	}

	if s.config == nil {
		return nil, fmt.Errorf("server configuration is not loaded")
	}

	if s.config.MCPServer.Security.AgentRegistry == nil {
		s.config.MCPServer.Security.AgentRegistry = make(map[string]AgentConfig)
	}

	s.config.MCPServer.Security.AgentRegistry[accountID] = AgentConfig{
		AccountID: accountID,
		Roles:     roles,
		Profile:   profile,
		Required:  false, // We auto register them, so not strictly required a priori? Actually, it's just registered.
	}

	if s.initCtx != nil && s.initCtx.ProjectRoot != "" {
		err := SaveMCPConfig(s.initCtx.ProjectRoot, s.config)
		if err != nil {
			return nil, fmt.Errorf("failed to save config: %v", err)
		}
	}

	return fmt.Sprintf("Successfully registered agent %s with roles %v and profile %s", accountID, roles, profile), nil
}
