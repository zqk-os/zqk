package system

import (
	"github.com/spf13/cobra"
)

// filterResultsByTierIfNeeded filters results by tier if flag is set
func filterResultsByTierIfNeeded(cmd *cobra.Command, allResults []CheckResult) []CheckResult {
	tierFilter, err := cmd.Flags().GetInt("tier")
	if err != nil {
		tierFilter = 0
	}
	if tierFilter > 0 {
		return filterResultsByTier(allResults, tierFilter)
	}
	return allResults
}
