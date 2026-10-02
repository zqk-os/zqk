package matrix

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/quality"
)

func runMatrixGet(cmd *cobra.Command, _ []string) error {
	_, resolved, err := resolveMatrixFromCmd(cmd)
	if err != nil {
		return err
	}

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

	res, err := quality.QueryMatrixCSV(resolved.CSVPath, resolved.ProfilePath, resolved.Alias, filters, globPat, goOnly, cvsID, resolved.SessionRefColumn, limit, columnNames)
	if err != nil {
		return err
	}

	return cli.FormatOutput(cmd, res)
}
