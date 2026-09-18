package mcp

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/spf13/cobra"
)

func NewInstallCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcInstallCommandBuilder()
	cmd.RunE = runInstall
	return cmd
}

func runInstall(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			return errfmt.Errorf("zqk mcp install must be run inside a ZQK project (no project root detected)")
		}

		if err := mcp.AutoInstall(projectRoot, logger); err != nil {
			return err
		}

		logging.Fluent(logger).Info("ZQK MCP server configuration applied; restart IDE(s) for changes to take effect").Log()
		return nil
	})(cmd, nil)
}
