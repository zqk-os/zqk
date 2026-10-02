package matrix

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/quality"
)

func runMatrixList(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	registryFlag, _ := cmd.Flags().GetString("registry")

	res, err := quality.ListMatricesFromRegistry(projectRoot, registryFlag)
	if err != nil {
		return err
	}
	return cli.FormatOutput(cmd, res)
}
