package agent

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentidle"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewScoreboardCmd creates the agent scoreboard command
func NewScoreboardCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentScoreboardCommandBuilder()
	cmd.RunE = runScoreboard
	return cmd
}

func runScoreboard(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		storePath := datacell.AgentIdleStorePath(proc.ProjectRoot())

		store, err := agentidle.NewFileStore(storePath)
		if err != nil {
			return err
		}

		records, err := store.GetAllRecords()
		if err != nil {
			return err
		}

		for key, duration := range records {
			logging.FluentEvent(proc.Logger()).Info("agent-idle-record").
				String("key", key).
				String("duration", duration.String()).
				Log()
		}

		return cli.FormatOutput(cmd, records)
	})(cmd, args)
}
