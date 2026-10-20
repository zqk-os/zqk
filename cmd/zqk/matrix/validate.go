package matrix

import (
	"github.com/spf13/cobra"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/quality"
)

// NewMatrixValidateCmd wires the generated validate spec builder to RunE.
func NewMatrixValidateCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixValidateCommandBuilder()
	c.Flags().String("name", "", "Matrix alias from matrix_registry.yaml")
	c.Flags().String("registry", "", "Path to matrix_registry.yaml")
	cli.AddCommonFlags(c)
	c.RunE = runMatrixValidate
	return c
}

func runMatrixValidate(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	name, registryFlag, err := resolveMatrixTargetFlags(cmd)
	if err != nil {
		return err
	}

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
