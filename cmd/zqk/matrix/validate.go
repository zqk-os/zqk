package matrix

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/quality"
)

// NewMatrixValidateCmd wires the generated validate spec builder to RunE.
func NewMatrixValidateCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixValidateCommandBuilder()
	c.RunE = runMatrixValidate
	return c
}

func runMatrixValidate(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == "" {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("project root not found; run from repo root or zqk use"))
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
