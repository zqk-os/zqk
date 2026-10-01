package matrix

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/quality"
)

func runMatrixGet(cmd *cobra.Command, _ []string) error {
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
	matrixPath, _ := cmd.Flags().GetString("matrix")
	profilePath, _ := cmd.Flags().GetString("profile")
	filterPairs, _ := cmd.Flags().GetStringArray("filter")
	globPat, _ := cmd.Flags().GetString("glob")
	goOnly, _ := cmd.Flags().GetBool("go-only")
	cvsID, _ := cmd.Flags().GetString("cvs-id")
	limit, _ := cmd.Flags().GetInt("limit")
	columnNames, _ := cmd.Flags().GetStringArray("field")

	filters, err := quality.ParseColumnValuePairs(filterPairs, false, "--filter")
	if err != nil {
		return err
	}

	resolved, err := quality.ResolveMatrixForCLI(projectRoot, name, registryFlag, matrixPath, profilePath)
	if err != nil {
		return err
	}

	res, err := quality.QueryMatrixCSV(resolved.CSVPath, resolved.ProfilePath, resolved.Alias, filters, globPat, goOnly, cvsID, resolved.SessionRefColumn, limit, columnNames)
	if err != nil {
		return err
	}

	return cli.FormatOutput(cmd, res)
}
