package mcp

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
)

func NewInstallCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcInstallCommandBuilder()
	cmd.Flags().Bool("all", false, "Install MCP server configuration for all supported hosts (Cursor, Claude Code, Cline, Windsurf)")
	cmd.RunE = runInstall
	return cmd
}

func runInstall(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			return errfmt.Errorf("%s must be run inside a project (no project root detected)", paths.CLIUsage("mcp", "install"))
		}

		if err := mcp.AutoInstall(projectRoot, logger); err != nil {
			return err
		}

		logging.Fluent(logger).Info("ZQK MCP server configuration applied; restart IDE(s) for changes to take effect").Log()
		cmd.Println("✓ ZQK MCP server configuration applied; restart IDE(s) for changes to take effect")
		return nil
	})(cmd, nil)
}
