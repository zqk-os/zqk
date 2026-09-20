package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/ingestion/adapters"
	"github.com/zqk-os/zqk/pkg/logging"
)

func NewSyncAgentsCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSyncAgentsCommandBuilder(), &cobra.Command{Use: "sync-agents"})
	cli.BindAsyncProgress(cmd, func(c *cobra.Command, args []string) error {
		dir, err := c.Flags().GetString("dir")
		if err != nil {
			return cli.Guard(c).Err(err).Wrapf("failed to get dir flag: %w").Return()
		}
		return runSyncAgents(c, dir)
	})
	return cmd
}

func runSyncAgents(cmd *cobra.Command, dir string) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("processor: %w").Return()
	}
	ctx := proc.OperationContext()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	adapter := adapters.NewGeminiAdapter(logger)
	if err := adapter.Ingest(ctx, dir); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("ingest: %w").Return()
	}
	return nil
}
