package gate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatApprovalsMarkdown(t *testing.T) {
	empty := FormatApprovalsMarkdown(nil)
	assert.Contains(t, empty, "## ZQK Gate Approval Queue")
	assert.Contains(t, empty, "No approval requests found.")

	md := FormatApprovalsMarkdown([]*ApprovalRequest{
		{
			ID:        "APR-001",
			Type:      "release_cut",
			Target:    "v0.8.0",
			Status:    StatusStaged,
			CreatedAt: "2026-11-25T08:00:00Z",
		},
	})
	assert.Contains(t, md, "| ID | Type | Target | Status | Created At |")
	assert.Contains(t, md, "| `APR-001` | `release_cut` | `v0.8.0` | `staged` | `2026-11-25T08:00:00Z` |")
}
