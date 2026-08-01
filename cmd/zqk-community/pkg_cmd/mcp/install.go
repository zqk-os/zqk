package mcp

import (
	"fmt"
	"os"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/spf13/cobra"
)

func NewInstallCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMcpInstallCommandBuilder(), &cobra.Command{Use: "install", Short: "Install MCP server configurations for supported IDEs"})
	cmd.RunE = runInstall
	cli.RequireSession(cmd, false)
	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	logger := logging.GetLoggerFromProfile(cli.GetContext(cmd).Profile)

	// Determine if we are in a project
	projectRoot := cli.GetContext(cmd).ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("zqk mcp install must be run inside a ZQK project (no project root detected)")
	}

	if err := mcp.AutoInstall(projectRoot, logger); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n✅ ZQK MCP server configuration applied. Please restart your IDE(s) for the changes to take effect.\n")
	return nil
}
