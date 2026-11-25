package testdiscovery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatDiscoveredTargetsMarkdown(t *testing.T) {
	empty := FormatDiscoveredTargetsMarkdown(nil)
	assert.Contains(t, empty, "## ZQK Discovered Test Inventory (0 targets)")
	assert.Contains(t, empty, "No test targets discovered.")

	md := FormatDiscoveredTargetsMarkdown([]DiscoveredTarget{
		{Language: "python", Path: "tests/test_api.py", Line: 12, Function: "test_health"},
		{Language: "go", Path: "pkg/gate/gate_test.go", Line: 24, Function: "TestListApprovals", CriteriaRefs: []string{"CRIT-001"}},
	})
	assert.Contains(t, md, "## ZQK Discovered Test Inventory (2 targets)")
	assert.Contains(t, md, "| `go` | `pkg/gate/gate_test.go:24` | `TestListApprovals` | CRIT-001 |")
	assert.Contains(t, md, "| `python` | `tests/test_api.py:12` | `test_health` | - |")
	assert.Less(t, strings.Index(md, "`go`"), strings.Index(md, "`python`"), "targets must be sorted deterministically by language")
}
