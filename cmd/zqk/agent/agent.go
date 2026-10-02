package agent

import (
	stdcontext "context"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/feed"
	"github.com/zqk-os/zqk/cmd/zqk/learn"
	"github.com/zqk-os/zqk/cmd/zqk/swarm"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
)

// procSecurity extracts the security context from the processor or returns system context.
func procSecurity(proc *cli.Processor) *pkgctx.SecurityContext {
	if proc != nil {
		if sec := proc.SecurityContext(); sec != nil {
			return sec
		}
	}
	return pkgctx.NewSystemSecurityContext()
}

// procStorageTuple extracts standard context, security context, and storage provider from the processor.
func procStorageTuple(proc *cli.Processor) (stdcontext.Context, *pkgctx.SecurityContext, storage.ObjectStorageProvider) {
	if proc == nil {
		return stdcontext.Background(), pkgctx.NewSystemSecurityContext(), nil
	}
	return proc.OperationContext(), procSecurity(proc), proc.Storage()
}

// newAgentProcessor initializes a new processor and verifies project root presence.
func newAgentProcessor(cmd *cobra.Command) (*cli.Processor, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, err
	}
	if proc.ProjectRoot() == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	return proc, nil
}

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
	cmd.AddCommand(NewClaimGateCmd())
	cmd.AddCommand(NewReleaseCmd())
	cmd.AddCommand(NewRecoverCmd())
	cmd.AddCommand(NewNextCmd())
	cmd.AddCommand(NewChatResponderCmd())
	cmd.AddCommand(NewValidateCmd())
	cmd.AddCommand(NewScoreboardCmd())
	cmd.AddCommand(NewSeatWorkerCmd())
	cmd.AddCommand(NewGuidingStepCmd())
	cmd.AddCommand(swarm.NewAgentSwarmInitCmd())
	cmd.AddCommand(swarm.NewSwarmCmd())
	cmd.AddCommand(feed.NewFeedCmd())
	cmd.AddCommand(learn.NewLearnCmd())
	cmd.AddCommand(NewAgentHookCmd())

	return cmd
}
