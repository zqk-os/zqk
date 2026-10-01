package system

import (
	"github.com/zqk-os/zqk/pkg/systemcheck/autofix"
)

var fixDependencyOrder = autofix.FixDependencyOrder

// getFixDependencyPriority delegates to pkg/systemcheck/autofix.GetFixDependencyPriority.
func getFixDependencyPriority(issue Issue) int {
	return autofix.GetFixDependencyPriority(issue)
}

// sortIssuesByDependency delegates to pkg/systemcheck/autofix.SortIssuesByDependency.
func sortIssuesByDependency(issues []Issue) []Issue {
	return autofix.SortIssuesByDependency(issues)
}
