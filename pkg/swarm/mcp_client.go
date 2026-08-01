package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/telemetry"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// MCPExecutor handles starting the native MCP server and routing LLM tool calls to it.
type MCPExecutor struct {
	cmd        *exec.Cmd
	client     *mcp.Client
	tools      []llm.ToolDefinition
	allTools   map[string]llm.ToolDefinition
	lazyTools  []llm.ToolDefinition
	cancel     context.CancelFunc
	stderrFile *os.File
	tracker    telemetry.Tracker
	personaID  string
}

// SetTelemetryContext configures the telemetry tracker and persona for skill invocations.
func (e *MCPExecutor) SetTelemetryContext(personaID string, tracker telemetry.Tracker) {
	e.personaID = personaID
	e.tracker = tracker
	if e.client != nil {
		e.client.SetTracker(tracker)
	}
}

// NewMCPExecutor starts the zqk-mcp binary and initializes the MCP client.
func NewMCPExecutor(ctx context.Context, mcpPath string) (*MCPExecutor, error) {
	ctx, cancel := context.WithCancel(ctx)

	parts := strings.Fields(mcpPath)
	if len(parts) == 0 {
		cancel()
		return nil, fmt.Errorf("invalid mcp path")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	projectRoot := getProjectRoot()
	var traceFile, stderrFile string
	if projectRoot != "" {
		logsDir := filepath.Join(projectRoot, paths.ProjectDataDir, "logs")
		_ = fileutil.EnsureDir(logsDir)
		traceFile = filepath.Join(logsDir, "mcp-trace.log")
		stderrFile = filepath.Join(logsDir, "mcp-stderr.log")
	} else {
		traceFile = filepath.Join(os.TempDir(), "mcp_trace.log")
		stderrFile = filepath.Join(os.TempDir(), "mcp_stderr.log")
	}

	// Add trace environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "ZQK_MCP_TRACE=true", "ZQK_MCP_TRACE_FILE="+traceFile)

	f, _ := os.OpenFile(stderrFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if f != nil {
		cmd.Stderr = f
	}

	var client *mcp.Client
	success := false
	defer func() {
		if !success {
			if client != nil {
				client.Close()
			}
			cancel()
			if cmd.Process != nil {
				_ = cmd.Wait()
			}
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errfmt.Newf("failed to get stdin pipe").Wrap(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errfmt.Newf("failed to get stdout pipe").Wrap(err)
	}

	if err := cmd.Start(); err != nil {
		return nil, errfmt.Newf("failed to start mcp server").Wrap(err)
	}

	transport := mcp.NewDefaultTransport()
	client = mcp.NewClient(stdout, stdin, transport)

	// Initialize the MCP client
	initParams := mcp.InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]any{},
	}

	clientID := os.Getenv(zqkenv.APIKey())
	if clientID == "" {
		clientID = "account:system"
	}
	initParams.Capabilities[objects.FieldKeyClientID] = clientID
	initParams.ClientInfo.Name = "Cursor"
	initParams.ClientInfo.Version = "1.0.0"

	_, err = client.Initialize(ctx, initParams)
	if err != nil {
		return nil, errfmt.Newf("failed to initialize mcp client").Wrap(err)
	}

	if err := client.Initialized(ctx); err != nil {
		return nil, errfmt.Newf("failed to send initialized notification").Wrap(err)
	}

	executor := &MCPExecutor{
		cmd:        cmd,
		client:     client,
		cancel:     cancel,
		stderrFile: f,
	}

	success = true
	return executor, nil
}

// Close shuts down the MCP client and server process.
func (e *MCPExecutor) Close() error {
	if e.client != nil {
		e.client.Close()
	}
	if e.cancel != nil {
		e.cancel()
	}
	if e.cmd != nil {
		e.cmd.Wait() // wait for process to exit
	}
	if e.stderrFile != nil {
		e.stderrFile.Close()
	}
	return nil
}

// GetTools queries the MCP server for available tools and maps them to LLM ToolDefinitions.
func (e *MCPExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	if len(e.tools) > 0 {
		return e.tools, nil
	}
	toolsList, err := e.client.ListTools(ctx)
	if err != nil {
		return nil, errfmt.Newf("failed to list tools").Wrap(err)
	}

	var llmTools []llm.ToolDefinition
	for _, t := range toolsList.Tools {
		// Map input schema to parameters map
		var params map[string]any
		if t.InputSchema != nil {
			schemaBytes, err := json.Marshal(t.InputSchema)
			if err == nil {
				json.Unmarshal(schemaBytes, &params)
				compressSchema(params)
			}
		} else {
			params = map[string]any{}
		}

		llmTools = append(llmTools, llm.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		})
	}

	sort.Slice(llmTools, func(i, j int) bool {
		return llmTools[i].Name < llmTools[j].Name
	})

	e.allTools = make(map[string]llm.ToolDefinition)
	var eagerTools []llm.ToolDefinition
	var lazyTools []llm.ToolDefinition

	for _, t := range llmTools {
		e.allTools[t.Name] = t
		if isEagerTool(t.Name) {
			eagerTools = append(eagerTools, t)
		} else {
			lazyTools = append(lazyTools, t)
		}
	}
	e.lazyTools = lazyTools

	// Inject meta-tools for lazy loading
	eagerTools = append(eagerTools, llm.ToolDefinition{
		Name:        "zqk_mcp_list_tools",
		Description: "List all available lazy-loaded tools in the system. Use this to discover specialized tools.",
		Parameters: map[string]any{
			objects.FieldKeyType: "object",
			"properties":         map[string]any{},
		},
	})
	eagerTools = append(eagerTools, llm.ToolDefinition{
		Name:        "zqk_mcp_get_tool_schema",
		Description: "Get the JSON schema for a specific lazy-loaded tool by its name.",
		Parameters: map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"tool_name": map[string]any{objects.FieldKeyType: "string"},
			},
			"required": []string{"tool_name"},
		},
	})
	eagerTools = append(eagerTools, llm.ToolDefinition{
		Name:        "zqk_mcp_call_tool",
		Description: "Call a lazy-loaded tool by its name and arguments.",
		Parameters: map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"tool_name": map[string]any{objects.FieldKeyType: "string"},
				"arguments": map[string]any{objects.FieldKeyType: "object"},
			},
			"required": []string{"tool_name", "arguments"},
		},
	})

	e.tools = eagerTools
	return eagerTools, nil
}

func isEagerTool(name string) bool {
	switch name {
	case "zqk_read_file", "zqk_write_file", "zqk_read_code", "zqk_execute_bash",
		"zqk_help", "zqk_object_create", "zqk_object_update", "zqk_object_delete",
		"zqk_object_get", "zqk_object_list", "zqk_object_fields", "zqk_system_status",
		"zqk_workflow_next", "zqk_agent_next", "zqk_get_next_backlog_item":
		return true
	}
	return false
}

// compressSchema recursively strips large non-essential fields from JSON schemas
// to dramatically reduce the token payload sent to LLMs during tool definition.
func compressSchema(schema map[string]any) {
	if schema == nil {
		return
	}
	delete(schema, "examples")
	delete(schema, "additionalProperties")
	delete(schema, "patternProperties")
	delete(schema, "$schema")
	delete(schema, "format")
	delete(schema, "default")

	if props, ok := schema["properties"].(map[string]any); ok {
		for _, v := range props {
			if propMap, ok := v.(map[string]any); ok {
				compressSchema(propMap)
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		compressSchema(items)
	}
}

// ExecuteToolCall routes a ToolCall to the MCP client.
func (e *MCPExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	if call.Name == "zqk_mcp_list_tools" {
		var summary []map[string]string
		for _, t := range e.lazyTools {
			summary = append(summary, map[string]string{
				objects.FieldKeyName:        t.Name,
				objects.FieldKeyDescription: t.Description,
			})
		}
		b, _ := json.Marshal(summary)
		return string(b), nil
	}

	if call.Name == "zqk_mcp_get_tool_schema" {
		var args map[string]any
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return "", errfmt.Newf("failed to parse arguments").Wrap(err)
		}
		toolName, _ := args["tool_name"].(string)
		if t, ok := e.allTools[toolName]; ok {
			b, _ := json.MarshalIndent(t, "", "  ")
			return string(b), nil
		}
		return "", errfmt.Errorf("tool '%s' not found", toolName)
	}

	if call.Name == "zqk_mcp_call_tool" {
		var args map[string]any
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return "", errfmt.Newf("failed to parse arguments").Wrap(err)
		}
		toolName, _ := args["tool_name"].(string)
		innerArgs, _ := args["arguments"].(map[string]any)
		innerArgsBytes, _ := json.Marshal(innerArgs)
		call.Name = toolName
		call.Arguments = string(innerArgsBytes)
	}

	normalizedName, err := e.normalizeToolName(call.Name)
	if err != nil {
		return "", err
	}

	if normalizedName != call.Name {
		logger := logging.GetLogger()
		logging.FluentEvent(logger).Warn("Fuzzy matched tool name").
			WithFields(
				logging.String("original", call.Name),
				logging.String("normalized", normalizedName),
			).Log()
	}

	var args map[string]any
	if call.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return "", errfmt.Newf("failed to unmarshal tool arguments").Wrap(err)
		}
	} else {
		args = map[string]any{}
	}

	start := time.Now()
	result, err := e.client.CallTool(ctx, normalizedName, args)
	duration := time.Since(start)

	if e.tracker != nil && e.personaID != "" {
		e.tracker.RecordPersonaSkillInvocation(ctx, e.personaID, normalizedName, duration, err == nil)
	}

	if err != nil {
		return "", err
	}

	return string(result), nil
}

func getProjectRoot() string {
	if pr := os.Getenv(zqkenv.ProjectRoot()); pr != "" {
		return pr
	}
	if tr := os.Getenv(zqkenv.TestRoot()); tr != "" {
		return tr
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	d := make([][]int, len(s1)+1)
	for i := range d {
		d[i] = make([]int, len(s2)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			min := d[i-1][j] + 1
			if d[i][j-1]+1 < min {
				min = d[i][j-1] + 1
			}
			if d[i-1][j-1]+cost < min {
				min = d[i-1][j-1] + cost
			}
			d[i][j] = min
		}
	}
	return d[len(s1)][len(s2)]
}

// normalizeToolName attempts to find the correct tool name, using exact match,
// underscore replacement, and fuzzy matching as fallbacks.
func (e *MCPExecutor) normalizeToolName(callName string) (string, error) {
	// First, check for exact match
	for _, t := range e.tools {
		if t.Name == callName {
			return callName, nil
		}
	}

	// Clean up the name: replace spaces with underscores
	cleanName := strings.ReplaceAll(callName, " ", "_")
	for _, t := range e.tools {
		if t.Name == cleanName {
			return cleanName, nil
		}
	}

	// Fuzzy match
	bestMatch := ""
	bestScore := 999
	for _, t := range e.tools {
		dist := levenshteinDistance(cleanName, t.Name)
		if dist < bestScore {
			bestScore = dist
			bestMatch = t.Name
		}
	}

	// Threshold for fuzzy match
	if bestScore <= 3 {
		return bestMatch, nil
	}

	// Not found
	return "", fmt.Errorf("tool not found: %q. did you mean: %q? Use exact tool names with underscores.", callName, bestMatch)
}
