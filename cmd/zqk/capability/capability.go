package capability

import (
	"errors"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	hivecap "github.com/zqk-os/zqk/pkg/hive/capability"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewCapabilityCmd creates the public 'zqk capability' command group.
func NewCapabilityCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Deny-by-default capability authorization surface",
		"Evaluate whether a named capability may proceed. Unknown and forbidden actions return a typed policy rejection so agents can recover to an allowed path.",
		"",
		"Stranger-facing demo capabilities include demo:safe (allowed) and demo:forbidden (denied).",
	).
		AddExample("Authorize an allowed capability", paths.CLIUsage("capability", "authorize", "--name", "demo:safe", "--agent-id", "stranger")).
		AddExample("Observe typed denial for a forbidden action", paths.CLIUsage("capability", "authorize", "--name", "demo:forbidden", "--agent-id", "stranger")).
		AddExample("List built-in demo capabilities", paths.CLIUsage("capability", "list"))

	cmd := &cobra.Command{
		Use:   "capability",
		Short: "Deny-by-default capability authorization surface",
		Long:  "Evaluate whether a named capability may proceed. Unknown and forbidden actions return a typed policy rejection.",
	}
	helpBuilder.ApplyToCommand(cmd)
	cmd.PersistentFlags().StringP("format", "f", "table", "Output format")
	cmd.AddCommand(newAuthorizeCmd())
	cmd.AddCommand(newListCmd())
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	return cmd
}

func demoRegistry() hivecap.Registry {
	reg := hivecap.NewRegistry()
	_ = reg.Register(hivecap.StaticCapability{
		CapName: "demo:safe", CapDescription: "Allowed stranger demo action", Allow: true,
	})
	_ = reg.Register(hivecap.StaticCapability{
		CapName: "demo:forbidden", CapDescription: "Forbidden stranger demo action", Allow: false,
	})
	return reg
}

func newAuthorizeCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Authorize a named capability (deny-by-default)",
		"Looks up the capability and evaluates its policy. Unknown names and denied policies return ErrPolicyRejected.",
		"",
	).
		AddExample("Allowed demo capability", paths.CLIUsage("capability", "authorize", "--name", "demo:safe", "--agent-id", "stranger")).
		AddExample("Forbidden demo capability", paths.CLIUsage("capability", "authorize", "--name", "demo:forbidden", "--agent-id", "stranger"))

	cmd := &cobra.Command{
		Use:   "authorize",
		Short: "Authorize a named capability (deny-by-default)",
		Long:  "Looks up the capability and evaluates its policy. Unknown names and denied policies return ErrPolicyRejected.",
		RunE:  runAuthorize,
	}
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().String("name", "", "Capability name to authorize (for example demo:safe)")
	cmd.Flags().String("agent-id", "", "Optional agent id recorded in the evaluation context")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List built-in demo capabilities",
		"Lists the built-in stranger-demo capability set used by capability authorize.",
		"",
	).AddExample("List demo capabilities", paths.CLIUsage("capability", "list"))

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List built-in demo capabilities",
		Long:  "Lists the built-in stranger-demo capability set used by capability authorize.",
		RunE:  runList,
	}
	helpBuilder.ApplyToCommand(cmd)
	return cmd
}

func runAuthorize(cmd *cobra.Command, _ []string) error {
	var flagsBag clipkg.FlagBag
	name := flagsBag.String(cmd, "name")
	agentID := flagsBag.String(cmd, "agent-id")
	if err := flagsBag.Err(); err != nil {
		return err
	}
	if name == "" {
		return errfmt.Errorf("--name is required")
	}

	err := hivecap.Authorize(cmd.Context(), demoRegistry(), name, hivecap.EvalContext{AgentID: agentID})
	if err != nil {
		payload := map[string]any{
			"allowed":         false,
			"capability":      name,
			"agent_id":        agentID,
			"error":           err.Error(),
			"policy_rejected": errors.Is(err, hivecap.ErrPolicyRejected),
		}
		if formatErr := cli.FormatOutput(cmd, payload); formatErr != nil {
			return formatErr
		}
		return err
	}
	return cli.FormatOutput(cmd, map[string]any{
		"allowed":    true,
		"capability": name,
		"agent_id":   agentID,
	})
}

func runList(cmd *cobra.Command, _ []string) error {
	reg := demoRegistry()
	caps := reg.List()
	rows := make([]map[string]any, 0, len(caps))
	for _, c := range caps {
		rows = append(rows, map[string]any{
			"name":        c.Name(),
			"description": c.Description(),
		})
	}
	return cli.FormatOutput(cmd, map[string]any{
		"capabilities": rows,
		"count":        len(rows),
	})
}
