package matrix

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/quality"
	"github.com/spf13/cobra"
)

func runMatrixList(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == "" {
		return errfmt.Errorf("project root not found; run from repo root or zqk use")
	}

	registryFlag, _ := cmd.Flags().GetString("registry")

	res, err := quality.ListMatricesFromRegistry(projectRoot, registryFlag)
	if err != nil {
		return err
	}
	return cli.FormatOutput(cmd, res)
}
