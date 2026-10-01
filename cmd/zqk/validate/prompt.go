package validate

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

type buildPromptOptions struct {
	TaskTitle   string
	PlanTitle   string
	PlanID      string
	TargetAgent string
	Capability  string
	IncludeTDD  bool
}

func NewBuildPromptCmd() *cobra.Command {
	var opts buildPromptOptions

	cmd := bldr_cli_cmd_v1.NewAgentBuildPromptCommandBuilder()
	cmd.Use = "build-prompt"
	cmd.Short = "Generate a fully policy-injected prompt for an agent task"
	cmd.Long = `Generates a markdown prompt for an agent task by injecting active project policies,
feedback, and architecture rules via the agentprompt builder.

This command acts as an injection hook to guarantee that external LLM calls or
orchestrators provide agents with context that strictly aligns with project policies.`
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		if opts.TaskTitle == "" {
			return errfmt.Errorf("requires --task-title")
		}

		ctx := proc.OperationContext()

		promptOpts := agentprompt.TaskPromptOptions{
			TaskTitle:       opts.TaskTitle,
			PlanTitle:       opts.PlanTitle,
			PlanID:          opts.PlanID,
			TargetAgent:     opts.TargetAgent,
			Capability:      opts.Capability,
			IncludeTDD:      opts.IncludeTDD,
			IncludeObserver: true,
		}

		prompt, err := agentprompt.BuildTaskPrompt(ctx, proc.Storage(), proc.SecurityContext(), proc.ProjectRoot(), promptOpts)
		if err != nil {
			return errfmt.Newf("failed to build unified agent prompt").Wrap(err)
		}

		// Also append the new mandatory push/PR instructions as a universal baseline
		prompt += "\n\n## 🛑 Subagent Definition of Done (Push & PR Mandate)\n"
		prompt += "When your coding task is complete, you MUST execute `git push origin HEAD` and `gh pr create` (with an appropriate title and body).\n"
		prompt += "DO NOT stop at a local commit. Leaving unpushed commits is considered a catastrophic failure.\n"

		// Output the generated prompt to stdout
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(prompt))
		return nil
	})

	cmd.Flags().StringVar(&opts.TaskTitle, "task-title", "", "The core task directive")
	cmd.Flags().StringVar(&opts.PlanTitle, "plan-title", "", "Optional plan title context")
	cmd.Flags().StringVar(&opts.PlanID, "plan-id", "", "Optional plan ID context")
	cmd.Flags().StringVar(&opts.TargetAgent, "target-agent", "", "Optional target agent persona")
	cmd.Flags().StringVar(&opts.Capability, "capability", "", "Optional capability context")
	cmd.Flags().BoolVar(&opts.IncludeTDD, "include-tdd", true, "Include Mandatory TDD Paradigm")

	return cmd
}
