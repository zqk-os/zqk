package matrix

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/quality"
)

// NewMatrixValidateCmd wires the generated validate spec builder to RunE.
func NewMatrixValidateCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixValidateCommandBuilder()
	c.RunE = runMatrixValidate
	return c
}

func runMatrixValidate(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	registryFlag, _ := cmd.Flags().GetString("registry")

	res, err := quality.ValidateMatrixRegistry(projectRoot, name, registryFlag)
	if err != nil {
		return err
	}
	if err := cli.FormatOutput(cmd, res); err != nil {
		return err
	}
	if !res.AllOK() {
		return errfmt.Errorf("one or more matrices failed validation")
	}
	return nil
}
