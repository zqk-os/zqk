package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

var (
	atkRegex = regexp.MustCompile(`ATK-[A-Za-z0-9_-]+`)
	bliRegex = regexp.MustCompile(`BLI-[A-Za-z0-9_-]+`)
	priRegex = regexp.MustCompile(`PRI-[A-Za-z0-9_-]+`)
)

// NewAgentHookCmd returns the parent command for vendor and kernel lifecycle hook integrations.
func NewAgentHookCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentHookCommandBuilder(), &cobra.Command{})
	cmd.AddCommand(newPreSubagentHookCmd())
	return cmd
}

func newPreSubagentHookCmd() *cobra.Command {
	var vendor string
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentHookPreSubagentCommandBuilder(), &cobra.Command{
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			return runPreSubagentHook(cmd, proc, vendor)
		}),
	})
	if cmd.Flags().Lookup("vendor") == nil {
		cmd.Flags().StringVar(&vendor, "vendor", "antigravity", "Target agent host vendor format (antigravity, generic)")
	}
	return cmd
}

type antigravityHookInput struct {
	ToolCall struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	} `json:"toolCall"`
	WorkspacePaths []string `json:"workspacePaths"`
	ConversationID string   `json:"conversationId"`
}

type antigravityHookOutput struct {
	Decision  string         `json:"decision"`
	Reason    string         `json:"reason,omitempty"`
	Overwrite map[string]any `json:"overwrite,omitempty"`
}

func runPreSubagentHook(cmd *cobra.Command, proc *cli.Processor, vendor string) error {
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	return runPreSubagentHookWithDeps(proc.OperationContext(), sec, proc.Storage(), proc.ProjectRoot(), vendor, os.Stdin, os.Stdout)
}

func runPreSubagentHookWithDeps(ctx context.Context, sec *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, projectRoot, vendor string, in io.Reader, out io.Writer) error {
	if sp == nil {
		// Output allow and exit gracefully if storage is unreachable
		return emitHookDecision(out, "allow", nil)
	}

	inputBytes, err := io.ReadAll(in)
	if err != nil || len(strings.TrimSpace(string(inputBytes))) == 0 {
		return emitHookDecision(out, "allow", nil)
	}

	var hookIn antigravityHookInput
	if err := json.Unmarshal(inputBytes, &hookIn); err != nil {
		logging.FluentEvent(logging.GetLogger()).Warn("pre-subagent hook failed to parse input JSON").WithError(err).Log()
		return emitHookDecision(out, "allow", nil)
	}

	if hookIn.ToolCall.Name != "invoke_subagent" {
		return emitHookDecision(out, "allow", nil)
	}

	rawSubagents, ok := hookIn.ToolCall.Args["Subagents"]
	if !ok {
		return emitHookDecision(out, "allow", nil)
	}

	subagentsJSON, err := json.Marshal(rawSubagents)
	if err != nil {
		return emitHookDecision(out, "allow", nil)
	}

	var subagents []map[string]any
	if err := json.Unmarshal(subagentsJSON, &subagents); err != nil {
		return emitHookDecision(out, "allow", nil)
	}

	modified := false
	for i, sub := range subagents {
		prompt, _ := sub["Prompt"].(string)

		// If prompt already has complete orchestration context or standing mandates, do not duplicate
		if strings.Contains(prompt, "# Orchestration Context") || strings.Contains(prompt, "## Standing mandates") {
			continue
		}

		targetID := ""
		if m := atkRegex.FindString(prompt); m != "" {
			targetID = m
		} else if m := bliRegex.FindString(prompt); m != "" {
			targetID = m
		} else if m := priRegex.FindString(prompt); m != "" {
			targetID = m
		}

		desc := prompt
		if targetID != "" {
			desc = ""
		} else if len(desc) > 300 {
			desc = desc[:300]
		}

		prepared, prepErr := AssemblePreparedContext(ctx, sec, sp, PreparedContextInput{
			TaskID:      targetID,
			Description: desc,
			ProjectRoot: projectRoot,
			Depth:       1,
			IncludeTDD:  true,
		})
		if prepErr == nil && prepared.Prompt != "" {
			subagents[i]["Prompt"] = fmt.Sprintf("%s\n\n## Subagent Specific Instructions\n%s", prepared.Prompt, prompt)
			modified = true
		}
	}

	if !modified {
		return emitHookDecision(out, "allow", nil)
	}

	overwrite := map[string]any{
		"Subagents": subagents,
	}
	return emitHookDecision(out, "allow", overwrite)
}

func emitHookDecision(out io.Writer, decision string, overwrite map[string]any) error {
	hookOut := antigravityHookOutput{
		Decision:  decision,
		Overwrite: overwrite,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(hookOut)
}
