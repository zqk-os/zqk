package system

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

func NewAgentLockdownCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAgentLockdownCommandBuilder(), &cobra.Command{Use: "agent-lockdown"})
	cli.BindAsyncProgress(cmd, func(c *cobra.Command, args []string) error {
		unlock, err := c.Flags().GetBool("unlock")
		if err != nil {
			return cli.Guard(c).Err(err).Wrapf("failed to get unlock flag: %w").Return()
		}
		return runAgentLockdown(c, unlock)
	})
	return cmd
}

func runAgentLockdown(cmd *cobra.Command, unlock bool) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("processor: %w").Return()
	}

	ctx := proc.OperationContext()
	projectRoot := ProjectRootOrResolve("")
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	dirs := []string{".gemini", ".ide", ".claude"}

	for _, dir := range dirs {
		target := filepath.Join(projectRoot, dir)
		var args []string
		if unlock {
			args = []string{"-R", "nouchg", target}
			logger.Info("Unlocking agent directory: " + target)
		} else {
			args = []string{"-R", "uchg", target}
			logger.Info("Locking agent directory: " + target)
		}

		execCmd := execwrap.CommandContext(ctx, "chflags", args...)
		if err := execCmd.Run(); err != nil {
			logger.Warn("Failed to update flags (directory may not exist): " + target + " - " + err.Error())
		}
	}

	if unlock {
		logger.Info("Agent directories successfully unlocked.")
		cmd.Println("✓ Agent directories successfully unlocked.")
	} else {
		logger.Info("Agent directories successfully locked down.")
		cmd.Println("✓ Agent directories successfully locked down.")
	}

	return nil
}
