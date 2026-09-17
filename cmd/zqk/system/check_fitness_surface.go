package system

import (
	"github.com/lanceman/zqk/pkg/fitness"
)

// annotateIssueClasses fills Issue.IssueClass from fitness.ClassifyIssue when empty.
func annotateIssueClasses(issues []Issue) {
	for i := range issues {
		if issues[i].IssueClass != "" {
			continue
		}
		issues[i].IssueClass = string(fitness.ClassifyIssue(fitness.IssueInput{
			Tier:        issues[i].Tier,
			Category:    issues[i].Category,
			Message:     issues[i].Message,
			AutoFixable: issues[i].AutoFixable,
		}))
	}
}

// filterIssuesForSurface keeps issues whose class is visible on the named surface.
// Unknown / empty surface returns issues unchanged (caller opts in).
func filterIssuesForSurface(surface string, issues []Issue) []Issue {
	if surface == "" {
		return issues
	}
	annotateIssueClasses(issues)
	s := fitness.Surface(surface)
	out := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		if fitness.VisibleOnSurface(s, fitness.IssueClass(issue.IssueClass)) {
			out = append(out, issue)
		}
	}
	return out
}
