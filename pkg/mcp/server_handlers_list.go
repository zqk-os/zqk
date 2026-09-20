package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	promptRoleUser         = "user"
	logComponentMcp        = "mcp_server"
	logFieldOperation      = "operation"
	paramKeyURI            = "uri"
	errInvalidParamsFmt    = "invalid params: %v"
	mdFenceOpen            = "```\n"
	mdFenceCloseBlank      = "```\n\n"
	mdFenceClose           = "```"
	snippetKindBacklog     = "  kind: \"backlog_item\"\n"
	snippetFilterExploring = "  filter: [\"status=exploring\"]\n"
	snippetResourceGet     = "resources/get with uri=<uri_from_resources_list>\n"
	fallbackBigPicture     = "# Project Big Picture\n\nProject context is not available. Please check system status and use resources/list to access documentation."
	defaultViewerRole      = "viewer"
)

// ResourceGetParams represents parameters for resources/get
type ResourceGetParams struct {
	URI string `json:"uri"`
}

// PromptGetParams represents parameters for prompts/get
type PromptGetParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// PromptGetResult represents the result of prompts/get
type PromptGetResult struct {
	Messages []PromptMessage `json:"messages"`
}

// PromptMessage represents a message in a prompt
type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func singleUserPrompt(content string) PromptGetResult {
	return PromptGetResult{
		Messages: []PromptMessage{
			{
				Role:    promptRoleUser,
				Content: content,
			},
		},
	}
}

// handleToolsList handles the tools/list method
func (s *Server) handleToolsList(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	// Debug logging: log state before processing
	toolCount := s.getToolCount()
	if s.getTraceWriter() != nil {
		s.traceLogf("[MCP_DEBUG] tools/list called: initialized=%v, tools_count=%d, rootCommand=%v",
			s.initialized.Load(), toolCount, s.rootCommand != nil)
		// Log actual tool names for debugging
		var toolNames []string
		_ = concurrency.RunInRLockWithLogger(
			&s.toolsMu, LockNameMcpServerListToolsDebug, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				toolNames = make([]string, 0, len(s.tools))
				for name := range s.tools {
					toolNames = append(toolNames, name)
				}
				return nil
			},
		)
		s.traceLogf("[MCP_DEBUG] tools/list: registered tools: %v (count=%d)", toolNames, len(toolNames))

		// Also check cache
		if cached := s.toolsCache.Load(); cached != nil {
			cachedTools := cached.([]Tool)
			s.traceLogf("[MCP_DEBUG] tools/list: cached tools count=%d", len(cachedTools))
		} else {
			s.traceLogf("[MCP_DEBUG] tools/list: tools cache is nil")
		}
	}

	// If not initialized and tools are empty, this might be a race condition
	// Log warning but still return empty list (client should call initialize first)
	if !s.initialized.Load() && toolCount == 0 {
		s.traceLogf("[MCP_DEBUG] WARNING: tools/list called before initialize - returning empty list")
		// CRITICAL: Do NOT call SendLogDebug here - it queues a log notification
		// that can interleave with the response write, causing JSON parsing errors
		// Use traceLogf instead (doesn't go through message queue)
		// _ = s.SendLogDebug("tools/list called before initialization...") // DISABLED - causes interleaving
	}

	// CRITICAL: If initialized but tools are empty, this is a serious bug
	// Log error but still return empty list (better than crashing)
	if s.initialized.Load() && toolCount == 0 {
		s.traceLogf("[MCP_ERROR] CRITICAL: tools/list called after initialize but no tools found! initialized=%v", s.initialized.Load())
		// Log to system logger as well (doesn't require trace)
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		logging.Fluent(logger).Error("CRITICAL: No tools found after initialization",
			errfmt.Errorf("tool count is 0 but server is initialized")).
			EmitComponent(logComponentMcp).
			String("trace", "true").
			Log()
	}

	var tools []Tool
	_ = concurrency.RunInRLockWithLogger(
		&s.toolsMu, LockNameMcpServerListTools, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tools = make([]Tool, 0, len(s.tools))
			for _, tool := range s.tools {
				tools = append(tools, tool)
			}
			return nil
		},
	)

	// Sort tools alphabetically by name for consistent ordering and token caching
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	// Log to trace file if available (won't pollute stdout/stderr)
	s.traceLogf("[MCP_DEBUG] tools/list: returning %d tools (initialized=%v, rootCommand=%v)",
		len(tools), s.initialized.Load(), s.rootCommand != nil)

	// Record tools/list event
	var currentClientID, currentSequenceID string
	_ = concurrency.RunInRLock(&s.clientIDMu, func() error {
		currentClientID = s.clientID
		return nil
	})
	_ = concurrency.RunInRLock(&s.sequenceIDMu, func() error {
		currentSequenceID = s.currentSequenceID
		return nil
	})
	if currentClientID != emptyValue && currentSequenceID != emptyValue {
		s.recordClientEvent(currentSequenceID, currentClientID, "tools_list", map[string]any{
			"tools_count": len(tools),
			"initialized": s.initialized.Load(),
		})
	}

	return ToolsListResult{Tools: tools}, nil
}

// handleResourcesList handles the resources/list method
func (s *Server) handleResourcesList(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("resources_list", map[string]any{})

	// Use lock-free cache for resources
	var resources []Resource
	if cached := s.resourcesCache.Load(); cached != nil {
		resources = cached.([]Resource)
	} else {
		// Fallback: cache not initialized yet
		_ = concurrency.RunInRLockWithLogger(
			&s.resourcesMu, LockNameMcpServerListResourcesFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				resources = mapValues(s.resources)
				return nil
			},
		)
		s.resourcesCache.Store(resources)
	}

	// IDE (and other MCP clients) require resources to be a JSON array, not null.
	// A nil Go slice marshals to JSON null and fails client schema validation.
	if resources == nil {
		resources = make([]Resource, 0)
	}

	return map[string]any{
		"resources": resources,
	}, nil
}

// handleResourcesGet handles the resources/get method
func (s *Server) handleResourcesGet(_ context.Context, _ string, params json.RawMessage) (any, error) {
	var getParams ResourceGetParams
	if err := json.Unmarshal(params, &getParams); err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: fmt.Sprintf(errInvalidParamsFmt, err),
		}
	}

	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("resources_get", map[string]any{
		paramKeyURI: getParams.URI,
	})

	// Get the resource (still need lock for map access)
	var resource Resource
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&s.resourcesMu, LockNameMcpServerGetResource, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			resource, exists = s.resources[getParams.URI]
			return nil
		},
	)
	if !exists {
		return nil, &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("resource not found: %s", getParams.URI),
		}
	}

	// Handle schema:// URIs with dynamic schema generation
	if strings.HasPrefix(getParams.URI, "schema://") {
		return s.handleSchemaResource(getParams.URI)
	}

	// Use shared resource loader to load resource content
	loader := NewResourceLoaderFromContext(s.initCtx, s.resourceURISchemeResolver, s.mimeAdapterRegistry)
	loadResult, err := loader.LoadResourceWithMimeType(getParams.URI, resource.MimeType)
	if err != nil {
		return nil, &JSONRPCError{
			Code:    ServerError,
			Message: fmt.Sprintf("failed to load resource: %v", err),
		}
	}

	mimeType := loadResult.MimeType
	content := loadResult.Content

	// Enforce TCP/IP-style chunked transmission limits (blocking massive single-blob JSON-RPC transfers)
	const MaxMCPBlobSize = 4 * 1024 * 1024 // 4 MB limit for Control Plane
	if len(content) > MaxMCPBlobSize {
		return nil, &JSONRPCError{
			Code:    ServerError,
			Message: fmt.Sprintf("Resource exceeds MCP Control-Plane size limit (%d bytes). Massive single-blob JSON-RPC transfers are blocked. Please use chunked transmission or Signed URLs for Data-Plane transit.", len(content)),
		}
	}

	return map[string]any{
		"contents": []map[string]any{
			{
				paramKeyURI: getParams.URI,
				"mimeType":  mimeType,
				"text":      string(content),
			},
		},
	}, nil
}

// handlePromptsList handles the prompts/list method
func (s *Server) handlePromptsList(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("prompts_list", map[string]any{})

	// Use lock-free cache for prompts
	prompts := s.ListPrompts()

	// IDE (and other MCP clients) require prompts to be a JSON array, not null.
	// A nil Go slice marshals to JSON null and fails client schema validation.
	if prompts == nil {
		prompts = make([]Prompt, 0)
	}

	return map[string]any{
		"prompts": prompts,
	}, nil
}

// handlePromptsGet handles the prompts/get method
func (s *Server) handlePromptsGet(ctx context.Context, _ string, params json.RawMessage) (any, error) {
	var getParams PromptGetParams
	if err := json.Unmarshal(params, &getParams); err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: fmt.Sprintf(errInvalidParamsFmt, err),
		}
	}

	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("prompts_get", map[string]any{
		objects.FieldKeyName: getParams.Name,
	})

	// Get the prompt template
	var prompt Prompt
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&s.promptsMu, LockNameMcpServerGetPrompt, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			prompt, exists = s.prompts[getParams.Name]
			return nil
		},
	)
	if !exists {
		return nil, &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("prompt not found: %s", getParams.Name),
		}
	}

	// Note: Resources are referenced dynamically via resources/list
	// Prompts instruct users to use resources/list to find resource URIs

	// Get agent roles from security context for role-aware prompts
	var agentRoles []string
	if s.secCtx != nil {
		if secCtx, ok := s.secCtx.(interface{ GetRoles() []string }); ok {
			agentRoles = secCtx.GetRoles()
		}
	}
	if len(agentRoles) == 0 {
		agentRoles = []string{defaultViewerRole} // default
	}

	// Assemble project context for dynamic prompts
	var projectCtx *ProjectContext
	if s.initCtx != nil {
		projectRoot := s.initCtx.GetProjectRoot()
		if projectRoot != emptyValue {
			assembler := NewProjectContextAssembler(projectRoot)
			if ctx, err := assembler.AssembleContext(); err == nil {
				projectCtx = ctx
			}
		}
	}

	// Create role-aware prompt generator
	var generator *RoleAwarePromptGenerator
	if projectCtx != nil {
		generator = NewRoleAwarePromptGenerator(projectCtx, agentRoles)
		if s.storageProvider != nil {
			generator = generator.WithStorageProvider(s.storageProvider)
		}

		// Query assignee_persona_ref if agent_task_id is provided
		if getParams.Arguments != nil {
			if taskID, ok := getParams.Arguments["agent_task_id"].(string); ok && taskID != emptyValue {
				if s.storageProvider != nil {
					filter := map[string]any{
						objects.FieldKeyKind: "agent_task",
						"filters": map[string]any{
							objects.FieldKeyID: taskID,
						},
						"limit": 1,
					}
					secCtx, _ := s.secCtx.(*pkgctx.SecurityContext)
					if secCtx == nil {
						secCtx = pkgctx.NewSystemSecurityContext()
					}
					resultAny, err := s.storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
					if err == nil {
						adapter := AdaptStorageResult(resultAny)
						if len(adapter.Objects) > 0 {
							if personaRef, ok := adapter.Objects[0][objects.FieldKeyAssigneePersonaRef].(string); ok {
								generator = generator.WithAssigneePersona(personaRef)
							}
						}
					}
				}
			}
		}
	}

	// Create role prompt renderer for loading prompts from role objects
	var promptRenderer *RolePromptRenderer
	if generator != nil && s.secCtx != nil {
		if secCtx, ok := s.secCtx.(interface{ GetRoles() []string }); ok {
			roles := secCtx.GetRoles()
			if len(roles) == 0 {
				roles = []string{defaultViewerRole}
			}
			secCtxForRender := &pkgctx.SecurityContext{Roles: roles}

			// Create guidance generator: prefer server storage for role object loading
			var guidanceGen *RoleGuidanceGenerator
			if s.storageProvider != nil {
				guidanceGen = NewRoleGuidanceGenerator(s.storageProvider)
			} else if generator.guidanceGenerator != nil {
				guidanceGen = generator.guidanceGenerator
			}

			promptRenderer = NewRolePromptRenderer(guidanceGen, generator, secCtxForRender)
		}
	}

	// Try to render prompt from role objects first
	if promptRenderer != nil {
		rendered, found := promptRenderer.RenderPrompt(pkgctx.NewSystemContext(), getParams.Name)
		if found {
			return singleUserPrompt(rendered), nil
		}
	}

	// Fallback to hardcoded prompts (existing logic)
	// Render the prompt based on name
	switch getParams.Name {
	case "welcome":
		// Generate role-aware welcome with project context
		var welcomeContent string
		if generator != nil {
			// Use dynamic big picture prompt with panic recovery
			var bigPicture string
			var welcomeBase, welcomeQuickStart string
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Log panic but don't crash - fall back to static content
						logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
						logging.Fluent(logger).Error("Panic generating welcome message", errfmt.Errorf("panic: %v", r)).
							EmitComponent(logComponentMcp).
							String(logFieldOperation, "prompts/get welcome").
							Log()
					}
				}()
				bigPicture = generator.GenerateBigPicturePrompt()
				welcomeBase, welcomeQuickStart = generator.GenerateWelcomeMessage()
			}()

			// If panic occurred, use fallback
			if bigPicture == emptyValue || welcomeBase == emptyValue || welcomeQuickStart == emptyValue {
				// Fallback to static welcome
				systemHealthMsg := fmt.Sprintf(WelcomeMessageSystemHealth, GetToolName("system_status"))
				welcomeContent = fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
					WelcomeMessageBase,
					systemHealthMsg,
					WelcomeMessageRoleCheck,
					WelcomeMessageQuickStartFallback)
			} else {
				systemHealthMsg := fmt.Sprintf(WelcomeMessageSystemHealth, GetToolName("system_status"))
				welcomeContent = fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
					welcomeBase,
					systemHealthMsg,
					bigPicture,
					welcomeQuickStart)
			}
		} else {
			// Fallback to static welcome
			systemHealthMsg := fmt.Sprintf(WelcomeMessageSystemHealth, GetToolName("system_status"))
			welcomeContent = fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
				WelcomeMessageBase,
				systemHealthMsg,
				WelcomeMessageRoleCheck,
				WelcomeMessageQuickStartFallback)
		}
		return singleUserPrompt(welcomeContent), nil
	case "execution_context":
		// Dynamic execution context prompt
		var content string
		if generator != nil {
			content = generator.GenerateExecutionContextPrompt()
		} else {
			content = "# Execution Context\n\nProject context is not available. Please check system status and use resources/list to access documentation."
		}
		return singleUserPrompt(content), nil
	case "big_picture":
		// Dynamic big picture prompt with panic recovery
		var content string
		if generator != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Log panic but don't crash - fall back to static content
						logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
						logging.Fluent(logger).Error("Panic generating big picture prompt", errfmt.Errorf("panic: %v", r)).
							EmitComponent(logComponentMcp).
							String(logFieldOperation, "prompts/get big_picture").
							Log()
					}
				}()
				content = generator.GenerateBigPicturePrompt()
			}()
			if content == emptyValue {
				content = fallbackBigPicture
			}
		} else {
			content = fallbackBigPicture
		}
		return singleUserPrompt(content), nil
	case "my_role", "current_role": // Support both names for backward compatibility
		// Show current role and permissions
		var content string
		if generator != nil {
			content = generator.GenerateMyRolePrompt()
		} else {
			// Fallback if generator not available
			content = "# Your Current Role\n\n"
			if len(agentRoles) > 0 {
				content += fmt.Sprintf("**Your Roles**: %s\n\n", strings.Join(agentRoles, ", "))
			} else {
				content += "**Your Role**: viewer (default)\n\n"
			}
			content += "Use tools/list to see what operations are available to you.\n"
			content += "Use prompts/get with name=\"role_based_access\" to understand permissions.\n"
		}
		return singleUserPrompt(content), nil
	case "getting_started":
		content := `# Getting Started with MCP Server

I'm the MCP server, your interface to the knowledge kernel. Here's how to get started:

## Understanding Your Access

**Important**: The tools available to you depend on your role and permissions. The MCP server filters commands based on:
- Your security context (roles and permissions)
- Server configuration (exposed_commands and write_operations)
- Command-level permission requirements

**To see what tools you have access to**, use the tools/list method. This will show you exactly which tools are available to you based on your current role and permissions.

## Available Capabilities

1. **Tools**: The tools available to you are determined by your role/permissions:
   - **Read Operations** (typically available to all):
     - Object operations: list, get, count, neighbors, related
     - System operations: status, check, validate
     - Reports: PCS (Project Confidence Score), EDD (Effort Distribution), Blockers
     - Graph operations: traversal, resolve references, state-aware queries
   
   - **Write Operations** (may require specific permissions):
     - Object create, update, delete (if enabled in config and you have permissions)
     - Object transitions (state changes)
     - Other write operations based on your role

2. **Prompts**: Use prompts/get to get help:
   - "getting_started" - This guide
   - "query_help" - How to query objects
   - "create_object_guide" - Guide and template for creating objects (if you have write access)
   - "common_tasks" - Examples of common tasks
   - "filter_syntax" - Filter syntax guide
   - "role_based_access" - Understanding role-based access

3. **Resources**: Access critical documentation directly:
   - Use resources/list to see all available resources
   - Use resources/get with uri="file://docs/..." to read documentation
   - Critical resources include:
     - Lifecycles guide (object state transitions)
     - System health monitoring guide
     - Workstreams guide
     - Architecture decision records
     - Policy and decision lifecycle guides

## Quick Start

1. **Check your available tools**: Use tools/list to see what you can do
2. **Check system health FIRST**: Use ` + GetToolName("system_status") + ` to verify system is healthy
   - **CRITICAL**: Always check system health before performing operations
   - Use ` + GetToolName("system_check") + ` to validate specific objects
   - Use system validation tools to ensure data integrity
3. **List objects**: Use ` + GetToolName("object_list") + ` to see what's in the system
4. **Get an object**: Use ` + GetToolName("object_get") + ` with an object ID (e.g., "BLI-626")
5. **Query objects**: Use ` + GetToolName("object_list") + ` with filters to find specific objects
6. **Review critical resources**: Use resources/list to see available documentation
   - Read lifecycle guides to understand state transitions
   - Review system health monitoring documentation
   - Check workstream documentation for workflow understanding

## Example Queries

- List all backlog items: ` + GetToolName("object_list") + ` with kind="backlog_item"
- Find items by status: ` + GetToolName("object_list") + ` with kind="backlog_item" and filter=["status=exploring"]
- Count objects: ` + GetToolName("object_count") + ` with kind="backlog_item"

## Understanding Permissions

If you need write access (create, update, delete), your role and permissions must allow it. The server configuration also controls which commands are exposed. Use prompts/get with name="role_based_access" to learn more about how permissions work.

## System Health - Critical Priority

**ALWAYS monitor system health**:
- Run ` + GetToolName("system_status") + ` regularly to check overall system health
- Use ` + GetToolName("system_check") + ` on objects before making changes
- Review system health monitoring resource: use resources/list to find "health_monitoring" resource
- Address any violations or health issues before proceeding with operations

## Workflows and Lifecycles

**Understanding workflows and lifecycles is essential**:
- Review lifecycle guide: use resources/list to find "lifecycles_guide" resource
- Understand state transitions before updating objects
- Check workstream documentation: use resources/list to find "workstreams_guide" resource
- Use prompts/get with name="object_lifecycle" for lifecycle overview

## Need Help?

- Use prompts/get with name="query_help" for query syntax
- Use prompts/get with name="common_tasks" for examples
- Use prompts/get with name="filter_syntax" for filter details
- Use prompts/get with name="role_based_access" to understand permissions
- Use resources/list to see all available documentation
- Use resources/get to read specific documentation files

Let me know what you'd like to explore!`
		return singleUserPrompt(content), nil
	case "query_help":
		content := "# Query Help\n\n" +
			"## Querying Objects\n\n" +
			"Use " + GetToolName("object_list") + " to query objects with filtering, sorting, and pagination.\n\n" +
			"### Basic Query\n" +
			mdFenceOpen +
			GetToolName("object_list") + " with:\n" +
			snippetKindBacklog +
			mdFenceCloseBlank +
			"### Filtering\n" +
			"Filters use the format: field=value or field:value\n\n" +
			"Examples:\n" +
			"- `filter: [\"status=exploring\"]` - Items in \"exploring\" status\n" +
			"- `filter: [\"priority=high\"]` - High priority items\n" +
			"- `filter: [\"status=exploring\", \"priority=high\"]` - Multiple filters (AND)\n" +
			"- `filter: ['status!=\"complete|archived\"']` - Negation filter\n\n" +
			"### Sorting\n" +
			mdFenceOpen +
			"sort-by: \"title\"\n" +
			"sort-asc: true\n" +
			mdFenceCloseBlank +
			"### Pagination\n" +
			mdFenceOpen +
			"offset: 0\n" +
			"limit: 10\n" +
			mdFenceCloseBlank +
			"### Grouping\n" +
			mdFenceOpen +
			"group-by: \"status\"\n" +
			"group-limit: 5\n" +
			mdFenceCloseBlank +
			"### Counting\n" +
			"Use " + GetToolName("object_count") + " for just counts:\n" +
			mdFenceOpen +
			GetToolName("object_count") + " with:\n" +
			snippetKindBacklog +
			snippetFilterExploring +
			mdFenceCloseBlank +
			"For more details, use prompts/get with name=\"filter_syntax\""
		return singleUserPrompt(content), nil
	case "create_object_template":
		kind := objects.KindBacklogItem
		if getParams.Arguments != nil {
			if k, ok := getParams.Arguments[objects.FieldKeyKind].(string); ok && k != emptyValue {
				kind = k
			}
		}
		content := fmt.Sprintf(`# Creating a %s Object

**Note**: Write operations (create, update, delete) may not be available to you depending on:
- Your role and permissions
- Server configuration (write_operations must be enabled)
- Command-level permission requirements

**First, check if you have access**: Use tools/list to see if object creation or similar write tools are available to you.

## General Process (if you have write access)

1. Identify the object kind: %s
2. Determine required fields (use fields command to see schema, if available)
3. Use the appropriate create command via MCP tool

## Common Object Kinds

- backlog_item - Work items, tasks, features
- goal - Strategic goals
- milestone - Project milestones
- workstream - Workstreams
- priority_plan - Priority plans

## Example: Creating a Backlog Item (if tool is available)

If object creation tools are available in your tools list:
- Use the tool with required fields like title, description
- Provide the object kind: "%s"
- Optionally link to goals, milestones, or priority plans using reference fields

## Getting Schema Information

To see what fields are required for a %s:
- Check if fields command is available via MCP (may require permissions)
- Or check the object schema documentation
- Use `+GetToolName("object_get")+` on an existing object of the same kind to see the structure

## If Write Access is Not Available

If you don't see create/update/delete tools in your tools/list:
- You may need different permissions or roles
- The server configuration may restrict write operations
- Contact your administrator to enable write access if needed
- Use prompts/get with name="role_based_access" to understand how to get write permissions

## Tips (if you have write access)

- Start with minimal required fields
- Add optional fields as needed
- Link objects using reference fields (*_ref, *_refs)
- Use lifecycle states appropriately
- Validate objects after creation using system validation tools`, kind, kind, kind, kind, kind)
		return singleUserPrompt(content), nil
	case "common_tasks":
		content := "# Common Tasks\n\n" +
			"Here are examples of common tasks you can perform:\n\n" +
			"## CRITICAL: Always Check System Health First\n\n" +
			"**Before performing any operations, check system health**:\n" +
			mdFenceOpen +
			"# 1. Check overall system status\n" +
			GetToolName("system_status") + "\n\n" +
			"# 2. Validate system integrity\n" +
			"# Use system validation tools\n\n" +
			"# 3. Check specific object health\n" +
			GetToolName("system_check") + " with id=\"BLI-626\"\n" +
			mdFenceCloseBlank +
			"**Review system health documentation**:\n" +
			mdFenceOpen +
			"# First, list resources to find the system health monitoring guide\n" +
			"resources/list\n" +
			"# Then get the resource by name (look for 'health_monitoring')\n" +
			snippetResourceGet +
			mdFenceCloseBlank +
			"## 1. List All Backlog Items\n" +
			mdFenceOpen +
			GetToolName("object_list") + " with kind=\"backlog_item\"\n" +
			mdFenceCloseBlank +
			"## 2. Find Items by Status\n" +
			mdFenceOpen +
			GetToolName("object_list") + " with:\n" +
			snippetKindBacklog +
			snippetFilterExploring +
			mdFenceCloseBlank +
			"## 3. Get a Specific Object\n" +
			mdFenceOpen +
			GetToolName("object_get") + " with id=\"BLI-626\"\n" +
			mdFenceCloseBlank +
			"## 4. Count Objects\n" +
			mdFenceOpen +
			GetToolName("object_count") + " with:\n" +
			"  kind: \"backlog_item\"\n" +
			"  filter: [\"status=complete\"]\n" +
			mdFenceCloseBlank +
			"## 5. Find Related Objects\n" +
			mdFenceOpen +
			"# Use graph traversal tools to find related objects\n" +
			"  id: \"BLI-626\"\n" +
			"  depth: 2\n" +
			mdFenceCloseBlank +
			"## 6. Get Project Metrics\n" +
			mdFenceOpen +
			GetToolName("reports_pcs") + "  # Project Confidence Score\n" +
			GetToolName("reports_edd") + "  # Effort Distribution Discrepancy\n" +
			GetToolName("reports_blockers") + "  # Dependencies & Blockers\n" +
			mdFenceCloseBlank +
			"## 7. Graph Traversal\n" +
			mdFenceOpen +
			"graph_traversal with:\n" +
			"  start_node_id: \"BLI-626\"\n" +
			"  max_depth: 3\n" +
			"  direction: \"outgoing\"\n" +
			mdFenceCloseBlank +
			"## 8. Resolve References\n" +
			mdFenceOpen +
			"resolve_references with:\n" +
			"  references: [\"goal:GOAL-123\", \"milestone:MIL-456\"]\n" +
			mdFenceCloseBlank +
			"## 9. Access Critical Documentation\n" +
			mdFenceOpen +
			"# List all available resources\n" +
			"resources/list\n\n" +
			"# Read lifecycle guide (look for 'lifecycles_guide' in resources/list)\n" +
			snippetResourceGet + "\n" +
			"# Read workstream guide (look for 'workstreams_guide' in resources/list)\n" +
			snippetResourceGet + "\n" +
			"# Read system health monitoring guide (look for 'health_monitoring' in resources/list)\n" +
			snippetResourceGet +
			mdFenceCloseBlank +
			"## 10. Understand Lifecycles Before State Changes\n" +
			mdFenceOpen +
			"# Get lifecycle information via prompt\n" +
			"prompts/get with name=\"object_lifecycle\"\n\n" +
			"# Or read the full lifecycle guide (use resources/list to find 'lifecycles_guide')\n" +
			"resources/list  # Find 'lifecycles_guide' resource\n" +
			"resources/get with uri=<uri_from_resources_list>\n" +
			"```"
		return singleUserPrompt(content), nil
	case "object_lifecycle":
		content := "# Object Lifecycle Guide\n\n" +
			"Objects follow defined lifecycles with states and transitions.\n\n" +
			"## Understanding Lifecycles\n\n" +
			"- Each object kind has a lifecycle definition\n" +
			"- Objects move through states (e.g., exploring → planning → executing → complete)\n" +
			"- State transitions may have conditions or requirements\n\n" +
			"## Common States\n\n" +
			"- exploring - Initial exploration phase\n" +
			"- planning - Planning and design\n" +
			"- executing - Active work\n" +
			"- complete - Work completed\n" +
			"- archived - Archived/historical\n\n" +
			"## Querying by State\n\n" +
			"Use filters to find objects in specific states:\n" +
			mdFenceOpen +
			GetToolName("object_list") + " with:\n" +
			snippetKindBacklog +
			snippetFilterExploring +
			mdFenceCloseBlank +
			"## State-Aware Queries\n\n" +
			"Use state_aware_query for lifecycle-aware operations:\n" +
			mdFenceOpen +
			"state_aware_query with:\n" +
			"  query_type: \"active_items\"\n" +
			"  filters: {\"status\": \"exploring|planning|executing\"}\n" +
			mdFenceClose
		return singleUserPrompt(content), nil
	case "filter_syntax":
		content := "# Filter Syntax Guide\n\n" +
			"Filters use a simple but powerful syntax.\n\n" +
			"## Basic Format\n\n" +
			"`field=value` or `field:value`\n\n" +
			"## Examples\n\n" +
			"### Equality\n" +
			"- `status=exploring` - Exact match\n" +
			"- `priority=high` - Exact match\n\n" +
			"### Negation\n" +
			"- `status!=\"complete\"` - Not equal\n" +
			"- `status!=\"complete|archived\"` - Not in set (pipe-separated)\n\n" +
			"### Multiple Filters\n" +
			"When you provide multiple filters, they are combined with AND:\n" +
			mdFenceOpen +
			"filter: [\"status=exploring\", \"priority=high\"]\n" +
			mdFenceOpen +
			"This finds items that are BOTH exploring AND high priority.\n\n" +
			"### Common Fields\n" +
			"- status - Object lifecycle state\n" +
			"- priority - Priority level\n" +
			"- kind - Object kind\n" +
			"- title - Object title (partial match may work)\n\n" +
			"## Tips\n\n" +
			"- Use quotes for values with special characters\n" +
			"- Pipe (|) for OR within a filter\n" +
			"- Multiple filters = AND logic\n" +
			"- Check object schema for available filter fields"
		return singleUserPrompt(content), nil
	case "role_based_access":
		content := "# Role-Based Access in ZQK MCP\n\n" +
			"The tools available to you are determined by your role, permissions, and server configuration.\n\n" +
			"## How Access Works\n\n" +
			"1. **Security Context**: Your access is determined by:\n" +
			"   - Your account ID and roles (e.g., \"admin\", \"developer\", \"viewer\")\n" +
			"   - Your permissions (e.g., \"read:*\", \"write:backlog_item\", \"delete:*\")\n" +
			"   - Server configuration (exposed_commands and write_operations)\n\n" +
			"2. **Command Filtering**: Commands are filtered at multiple levels:\n" +
			"   - **Config Level**: Server config defines which commands are exposed\n" +
			"   - **Permission Level**: Your roles/permissions determine if you can execute them\n" +
			"   - **Runtime Check**: Each command execution is validated against your permissions\n\n" +
			"## Checking Your Access\n\n" +
			"**Always check your available tools first**:\n" +
			"- Use tools/list to see exactly which tools you have access to\n" +
			"- The list is automatically filtered based on your role and permissions\n" +
			"- If a tool isn't in the list, you don't have access to it\n\n" +
			"## Common Permission Patterns\n\n" +
			"### Read-Only Access (Most Common)\n" +
			"- **Roles**: viewer, developer, or no specific role\n" +
			"- **Permissions**: read:* or read:backlog_item, read:goal, etc.\n" +
			"- **Available Tools**: list, get, count, neighbors, related, status, check, validate, reports\n\n" +
			"### Write Access\n" +
			"- **Roles**: developer, admin, or specific write roles\n" +
			"- **Permissions**: write:* or write:backlog_item, write:goal, etc.\n" +
			"- **Additional Requirement**: write_operations must be enabled in server config\n" +
			"- **Available Tools**: create, update, delete, transition (if configured and permitted)\n\n" +
			"### Admin Access\n" +
			"- **Roles**: admin\n" +
			"- **Permissions**: * (all permissions)\n" +
			"- **Available Tools**: All tools, including system operations\n\n" +
			s.generatePermissionFormatExamples() +
			"## If You Need More Access\n\n" +
			"If you need write access but don't have it:\n\n" +
			"1. **Check your tools**: Use tools/list to confirm what's available\n" +
			"2. **Understand the restriction**: The tool may not be exposed in server config, or you may lack permissions\n" +
			"3. **Contact administrator**: They can:\n" +
			"   - Add your role/permissions to the security context\n" +
			"   - Enable write_operations in the server config\n" +
			"   - Add specific commands to exposed_commands or write_operations\n\n" +
			"## Server Configuration\n\n" +
			"The server configuration (in .zqk/mcp/config.yaml) controls:\n" +
			"- exposed_commands: Which commands are available via MCP (whitelist)\n" +
			"- write_operations: Which write commands are enabled (requires permissions)\n" +
			"- blocked_commands: Which commands are explicitly blocked (blacklist)\n\n" +
			"## Best Practices\n\n" +
			"1. **Always check tools/list first** - Don't assume a tool is available\n" +
			"2. **Use elicitation** - If a tool requires parameters you don't have, the server will ask\n" +
			"3. **Understand your role** - Know what operations your role typically allows\n" +
			"4. **Request access appropriately** - If you need write access, work with your administrator\n\n" +
			"## Example: Checking Access\n\n" +
			mdFenceOpen +
			"1. Call tools/list to see available tools\n" +
			"2. If object creation tools are in the list, you can create objects\n" +
			"3. If it's not, you need additional permissions or config changes\n" +
			mdFenceCloseBlank +
			"For more details on specific operations, use other prompts like \"query_help\" or \"create_object_template\"."
		return singleUserPrompt(content), nil
	default:
		// Generic prompt rendering (can be extended)
		return singleUserPrompt(prompt.Description), nil
	}
}

// handleRootsList handles the roots/list method
func (s *Server) handleRootsList(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("roots_list", map[string]any{})

	roots := make([]map[string]any, 0, len(s.roots))
	for _, root := range s.roots {
		roots = append(roots, map[string]any{
			paramKeyURI: root,
		})
	}
	return map[string]any{
		"roots": roots,
	}, nil
}
