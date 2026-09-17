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
		allResults = filterResultsByTier(allResults, tierFilter)
	}
	return filterResultsBySurfaceIfNeeded(cmd, allResults)
}

// filterResultsBySurfaceIfNeeded applies fitness surface audience filter when --surface is set.
func filterResultsBySurfaceIfNeeded(cmd *cobra.Command, allResults []CheckResult) []CheckResult {
	surface, err := cmd.Flags().GetString("surface")
	if err != nil || surface == "" {
		// Still annotate classes when format is json? Keep cheap: only when surface set.
		return allResults
	}
	out := make([]CheckResult, 0, len(allResults))
	for _, r := range allResults {
		filtered := filterIssuesForSurface(surface, r.Issues)
		if len(filtered) == 0 {
			continue
		}
		r.Issues = filtered
		out = append(out, r)
	}
	return out
}
