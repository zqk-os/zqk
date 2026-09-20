package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
)

type AgentScoreboardCommandBuilder struct{}

func NewAgentScoreboardCommandBuilder() *AgentScoreboardCommandBuilder {
	return &AgentScoreboardCommandBuilder{}
}

func (b *AgentScoreboardCommandBuilder) Build(proc *cli.Processor) *cobra.Command {
	return &cobra.Command{
		Use:   "scoreboard",
		Short: "agent scoreboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
