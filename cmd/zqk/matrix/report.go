package matrix

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/quality"
)

func runMatrixReport(cmd *cobra.Command, _ []string) error {
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

	name, _ := cmd.Flags().GetString("name")
	registryFlag, _ := cmd.Flags().GetString("registry")
	matrixPath, _ := cmd.Flags().GetString("matrix")
	profilePath, _ := cmd.Flags().GetString("profile")
	goOnly, _ := cmd.Flags().GetBool("go-only")

	resolved, err := quality.ResolveMatrixForCLI(projectRoot, name, registryFlag, matrixPath, profilePath)
	if err != nil {
		return err
	}

	sum, err := quality.SummarizeMatrixFromPaths(resolved.CSVPath, resolved.ProfilePath, goOnly, resolved.SessionRefColumn)
	if err != nil {
		return err
	}
	sum.MatrixName = resolved.Alias

	return cli.FormatOutput(cmd, sum)
}
