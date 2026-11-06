package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search/interceptor"
	"github.com/zqk-os/zqk/pkg/storage"
)

type preCommandHookOutput struct {
	Decision  string         `json:"decision"`
	Reason    string         `json:"reason,omitempty"`
	Overwrite map[string]any `json:"overwrite,omitempty"`
}

// NewPreCommandHookCmd creates the CLI subcommand for pre-command interception.
func NewPreCommandHookCmd() *cobra.Command {
	var vendor string
	var projectRoot string

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentHookPreCommandCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			in, out := commandIO(cmd)
			v, err := cmd.Flags().GetString("vendor")
			if err != nil || v == "" {
				v = vendor
			}
			pr, err := cmd.Flags().GetString("project-root")
			if err != nil || pr == "" {
				pr = projectRoot
			}
			return runPreCommandHookWithDeps(cmd.Context(), nil, nil, pr, v, in, out)
		},
	})

	return cmd
}

// runPreCommandHookWithDeps processes the hook input payload, invokes the interceptor, and writes the decision JSON.
func runPreCommandHookWithDeps(ctx context.Context, sec *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, projectRoot, vendor string, in io.Reader, out io.Writer) error {
	_ = sec
	_ = sp
	_ = vendor

	inputBytes, err := safeDrainInput(in)
	if err != nil || len(strings.TrimSpace(string(inputBytes))) == 0 {
		return emitPreCommandDecision(out, "allow", "", nil)
	}

	var raw map[string]any
	if err := json.Unmarshal(inputBytes, &raw); err != nil {
		logging.FluentEvent(logging.GetLogger()).Warn("pre-command hook failed to parse input JSON").WithError(err).Log()
		return emitPreCommandDecision(out, "allow", "", nil)
	}

	toolName := extractToolName(raw)
	if toolName != "" && !isCommandTool(toolName) {
		return emitPreCommandDecision(out, "allow", "", nil)
	}

	commandLine := extractCommandLine(raw)
	if strings.TrimSpace(commandLine) == "" {
		return emitPreCommandDecision(out, "allow", "", nil)
	}

	cwd := extractCwd(raw, projectRoot)

	res, err := interceptor.Intercept(ctx, interceptor.InterceptRequest{
		CommandLine: commandLine,
		Cwd:         cwd,
		ProjectRoot: projectRoot,
	})
	if err != nil || res == nil || !res.Intercepted {
		return emitPreCommandDecision(out, "allow", "", nil)
	}

	overwrite := map[string]any{
		"CommandLine": res.RewrittenCommand,
	}

	reason := res.Reason
	if reason == "" || reason == interceptor.ReasonRewritten {
		reason = paths.RewriteCanonicalCLIInvocations("Intercepted search: redirected to token-budgeted zqk grep trigram engine")
	}

	return emitPreCommandDecision(out, "allow", reason, overwrite)
}

// mapString safely extracts a string field from a map.
func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// mapSubMap safely extracts a nested map[string]any from a map.
func mapSubMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if sm, ok := v.(map[string]any); ok {
			return sm
		}
	}
	return nil
}

// mapFirstStringSliceElement safely extracts the first string from a slice field in a map.
func mapFirstStringSliceElement(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch items := m[key].(type) {
	case []any:
		if len(items) > 0 {
			if s, ok := items[0].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	case []string:
		if len(items) > 0 {
			return strings.TrimSpace(items[0])
		}
	}
	return ""
}

// extractToolName locates the tool name in standard and nested payload structures.
func extractToolName(raw map[string]any) string {
	for _, key := range []string{"tool_name", "toolName", "tool"} {
		if val := mapString(raw, key); val != "" {
			return val
		}
	}
	if tc := mapSubMap(raw, "toolCall"); tc != nil {
		if name := mapString(tc, "name"); name != "" {
			return name
		}
	}
	return ""
}

// isCommandTool checks if the tool is responsible for executing shell commands.
func isCommandTool(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "command") ||
		strings.Contains(lower, "bash") ||
		strings.Contains(lower, "sh") ||
		strings.Contains(lower, "terminal") ||
		strings.Contains(lower, "exec") ||
		strings.Contains(lower, "run")
}

// extractCommandLine inspects top-level and nested tool arguments for the command string.
func extractCommandLine(raw map[string]any) string {
	keys := []string{"command_line", "CommandLine", "command", "cmd"}

	// Top level check
	for _, k := range keys {
		if s := strings.TrimSpace(mapString(raw, k)); s != "" {
			return s
		}
	}

	// In tool_args, toolCall.args, and args
	containers := []map[string]any{
		mapSubMap(raw, "tool_args"),
		mapSubMap(mapSubMap(raw, "toolCall"), "args"),
		mapSubMap(raw, "args"),
	}

	for _, container := range containers {
		if container == nil {
			continue
		}
		for _, k := range keys {
			if s := strings.TrimSpace(mapString(container, k)); s != "" {
				return s
			}
		}
	}

	return ""
}

// extractCwd extracts working directory from payload or falls back to project root or current working dir.
func extractCwd(raw map[string]any, projectRoot string) string {
	cwdKeys := []string{"cwd", "Cwd", "working_directory", "workingDirectory"}

	// Top level
	for _, k := range cwdKeys {
		if s := strings.TrimSpace(mapString(raw, k)); s != "" {
			return s
		}
	}

	// In workspacePaths
	if s := mapFirstStringSliceElement(raw, "workspacePaths"); s != "" {
		return s
	}

	// In tool_args and args
	containers := []map[string]any{
		mapSubMap(raw, "tool_args"),
		mapSubMap(raw, "args"),
	}

	for _, container := range containers {
		if container == nil {
			continue
		}
		for _, k := range cwdKeys {
			if s := strings.TrimSpace(mapString(container, k)); s != "" {
				return s
			}
		}
	}

	if projectRoot != "" {
		return projectRoot
	}

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// emitPreCommandDecision writes the formatted JSON response to out.
func emitPreCommandDecision(out io.Writer, decision, reason string, overwrite map[string]any) error {
	output := preCommandHookOutput{
		Decision:  decision,
		Reason:    reason,
		Overwrite: overwrite,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}
