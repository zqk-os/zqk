package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
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

	builder := clipkg.NewCommandBuilder("pre-command")
	builder.WithShort("Pre-command interceptor hook")
	builder.WithAliases("intercept")
	help := clipkg.DynamicHelpBuilder("Pre-command interceptor hook")
	help.WithDescriptionLines(paths.RewriteCanonicalCLIInvocations("Pre-command lifecycle hook intercepting shell commands and rewriting code searches to token-budgeted zqk grep."))
	help.AddExample("Run agent hook pre-command", "%s agent hook pre-command --vendor antigravity")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("vendor", "", "antigravity", "Target agent host vendor format (antigravity, generic)")
	builder.AddStringFlag("project-root", "", "", "Project root directory")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()

	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		in, out := commandIO(cmd)
		v, _ := cmd.Flags().GetString("vendor")
		if v == "" {
			v = vendor
		}
		pr, _ := cmd.Flags().GetString("project-root")
		if pr == "" {
			pr = projectRoot
		}
		return runPreCommandHookWithDeps(cmd.Context(), nil, nil, pr, v, in, out)
	}

	return cmd
}

// runPreCommandHookWithDeps processes the hook input payload, invokes the interceptor, and writes the decision JSON.
func runPreCommandHookWithDeps(ctx context.Context, _ *pkgctx.SecurityContext, _ storage.ObjectStorageProvider, projectRoot, _ string, in io.Reader, out io.Writer) error {
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

// extractToolName locates the tool name in standard and nested payload structures.
func extractToolName(raw map[string]any) string {
	if val, ok := raw["tool_name"].(string); ok && val != "" {
		return val
	}
	if val, ok := raw["toolName"].(string); ok && val != "" {
		return val
	}
	if val, ok := raw["tool"].(string); ok && val != "" {
		return val
	}
	if tc, ok := raw["toolCall"].(map[string]any); ok {
		if name, ok := tc["name"].(string); ok && name != "" {
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
		if s, ok := raw[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}

	// In tool_args
	if args, ok := raw["tool_args"].(map[string]any); ok {
		for _, k := range keys {
			if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}

	// In toolCall.args
	if tc, ok := raw["toolCall"].(map[string]any); ok {
		if args, ok := tc["args"].(map[string]any); ok {
			for _, k := range keys {
				if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
	}

	// In args
	if args, ok := raw["args"].(map[string]any); ok {
		for _, k := range keys {
			if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
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
		if s, ok := raw[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}

	// In workspacePaths
	if wp, ok := raw["workspacePaths"].([]any); ok && len(wp) > 0 {
		if s, ok := wp[0].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}

	// In tool_args
	if args, ok := raw["tool_args"].(map[string]any); ok {
		for _, k := range cwdKeys {
			if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}

	// In args
	if args, ok := raw["args"].(map[string]any); ok {
		for _, k := range cwdKeys {
			if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}

	if projectRoot != "" {
		return projectRoot
	}

	wd, err := os.Getwd()
	if err == nil {
		return wd
	}
	return ""
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
