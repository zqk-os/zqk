package agent

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/validate"
)

func NewValidateCmd() *cobra.Command {
	cmd := validate.NewValidateAgentCmd()
	cmd.Use = "validate"
	cmd.Short = "Validate codebase and optionally verify cryptographic stamp"
	return cmd
}
