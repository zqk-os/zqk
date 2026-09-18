package agent

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/swarm"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewAgentCmd creates the agent command group for multi-agent orchestration
func NewAgentCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentCommandBuilder(), &cobra.Command{
		Use:   "agent",
		Short: "Multi-agent orchestration and delegation",
		Long: `Provides orchestration and tooling for multiple agents to execute workstream items.
Enables task routing, context delegation to specialized sub-agents, and shared scheduler contracts.`,
	})

	cmd.AddCommand(NewOrchestrateCmd())
	cmd.AddCommand(NewStatusCmd())
	cmd.AddCommand(NewEvaluateCmd())
	cmd.AddCommand(NewSynthesizeSkillCmd())
	cmd.AddCommand(NewAgentNewCmd())
	cmd.AddCommand(NewSyncLoopCmd())
	cmd.AddCommand(NewExecuteCmd())
	cmd.AddCommand(NewPrepareContextCmd())
	cmd.AddCommand(NewClaimCmd())
	cmd.AddCommand(NewReleaseCmd())
	cmd.AddCommand(NewRecoverCmd())
	cmd.AddCommand(NewNextCmd())
	cmd.AddCommand(NewChatResponderCmd())
	cmd.AddCommand(NewValidateCmd())
	cmd.AddCommand(NewScoreboardCmd())
	cmd.AddCommand(NewSeatWorkerCmd())
	cmd.AddCommand(NewGuidingStepCmd())
	cmd.AddCommand(swarm.NewAgentSwarmInitCmd())

	return cmd
}
