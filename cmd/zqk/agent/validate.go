package agent

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/validate"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

func NewValidateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentValidateCommandBuilder()
	cmd.RunE = cli.WithProcessor(validate.RunValidateAgent)
	return cmd
}
