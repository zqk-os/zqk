package gate

import (
	"fmt"
	"strings"
)

// FormatApprovalsMarkdown renders a list of ApprovalRequests as a deterministic
// GitHub-Flavored Markdown table suitable for PR comments and CI job summaries.
func FormatApprovalsMarkdown(list []*ApprovalRequest) string {
	var b strings.Builder
	b.WriteString("## ZQK Gate Approval Queue\n\n")
	if len(list) == 0 {
		b.WriteString("_No approval requests found._\n")
		return b.String()
	}
	b.WriteString("| ID | Type | Target | Status | Created At |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, item := range list {
		if item == nil {
			continue
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			item.ID, item.Type, item.Target, item.Status, item.CreatedAt)
	}
	return b.String()
}
