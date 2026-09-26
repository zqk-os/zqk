package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
)

const (
	observerSearchToolSuffix          = "observer_search"
	observerRegisterToolSuffix        = "observer_register_agent"
	observerRequestGuidanceToolSuffix = "request_guidance"
	observerSearchMaxHits             = 20
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

	server.RegisterTool(
		GetToolName(observerRequestGuidanceToolSuffix),
		"Request ambient assistance and dynamic steering from the Observer Coach and team feed when encountering ambiguity, permission denials, or missing deliverables. Provide your question/obstacle, context, and optional task ID. Returns actionable guidance and logs the signal to the agent feed.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"query": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The question, obstacle, or ambiguity you need guidance on",
				},
				"task_id": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional current task ID (e.g. ATK-*, BLI-*)",
				},
				"context": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional context, attempted steps, or observed error messages",
				},
			},
			"required": []string{"query"},
		},
		server.handleObserverRequestGuidanceTool,
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

func (s *Server) handleObserverRequestGuidanceTool(ctx context.Context, args map[string]any) (any, error) {
	root := s.fileSandboxRoot()
	if s.initCtx != nil && s.initCtx.ProjectRoot != "" {
		root = s.initCtx.ProjectRoot
	}
	return HandleObserverRequestGuidance(ctx, root, args)
}

// HandleObserverRequestGuidance queries the Observer Coach and logs the signal to the agent feed.
func HandleObserverRequestGuidance(ctx context.Context, projectRoot string, args map[string]any) (any, error) {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		if q, ok := args["issue"].(string); ok {
			query = q
		} else if q, ok := args["message"].(string); ok {
			query = q
		} else if q, ok := args["question"].(string); ok {
			query = q
		}
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required: describe what guidance, clarification, or obstacle you need help with")
	}

	taskID, _ := args[objects.FieldKeyID].(string)
	if taskID == "" {
		taskID, _ = args["task_id"].(string)
	}
	contextDetails, _ := args["context"].(string)

	// 1. Emit assistance signal onto agent_feed for collaboration traceability
	if projectRoot != "" {
		_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot:      projectRoot,
			Message:          fmt.Sprintf("ASSISTANCE_REQUEST (task %s): %s | Context: %s", taskID, query, contextDetails),
			AgentID:          "swarm_agent",
			ToAgentID:        "observer_coach",
			Sender:           "agent_assistance_signal",
			EventType:        agentfeed.FeedEventTypeSteering,
			SkipEnabledCheck: true,
		})
	}

	// 2. Synthesize actionable guidance from Observer Coach
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Observer Coach Guidance (Task: %s)\n", taskID))
	sb.WriteString(fmt.Sprintf("**Query:** %s\n\n", query))

	lowerQ := strings.ToLower(query + " " + contextDetails)
	if strings.Contains(lowerQ, "where") || strings.Contains(lowerQ, "find") || strings.Contains(lowerQ, "deliverable") || strings.Contains(lowerQ, "artifact") || strings.Contains(lowerQ, "eval") {
		sb.WriteString("#### Deliverable & File Guidance:\n")
		sb.WriteString("- Artifact deliverables MUST be written to repository files using `write_file` or `write_code`.\n")
		sb.WriteString("- Code evaluations and reviews belong in `docs/eval/*.md` or the directory specified by upstream deliverables.\n")
		sb.WriteString("- Do not invent speculative kernel object kinds (e.g. `eval_finding`). Use standard files or registered kinds.\n\n")
	}

	if strings.Contains(lowerQ, "permission") || strings.Contains(lowerQ, "denied") || strings.Contains(lowerQ, "read-only") || strings.Contains(lowerQ, "system") {
		sb.WriteString("#### Permission & Scope Guidance:\n")
		sb.WriteString("- Swarm workers run with plan-scoped permissions. They cannot mutate system-managed fields (`created_at`, `updated_at`, `status`).\n")
		sb.WriteString("- Status transitions are managed by the orchestrator upon write verification. Focus solely on producing code/doc artifacts.\n\n")
	}

	if strings.Contains(lowerQ, "symbol") || strings.Contains(lowerQ, "function") || strings.Contains(lowerQ, "type") || strings.Contains(lowerQ, "code") {
		sb.WriteString("#### AST Discovery Guidance:\n")
		sb.WriteString("- Use `zqk_observer_search` with symbol substrings or package paths to locate symbols in the live AST.\n")
		sb.WriteString("- Read discovered code with `read_code` or `read_file`.\n\n")
	}

	if projectRoot != "" {
		tips := observer.ReadCachedTips(projectRoot)
		if len(tips) > 0 {
			sb.WriteString("#### Ambient System Insights:\n")
			for _, tip := range tips {
				sb.WriteString(fmt.Sprintf("- %s\n", tip))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("#### Recommended Immediate Action:\n")
	sb.WriteString("Proceed with editing or creating the target deliverable files using `write_file`/`write_code`. If you need to search codebase symbols, run `zqk_observer_search`.\n")

	return sb.String(), nil
}
