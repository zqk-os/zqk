package testdiscovery_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/testdiscovery"
)

func TestDiscoveredTarget_Fields(t *testing.T) {
	t.Parallel()

	target := testdiscovery.DiscoveredTarget{
		Language:        "go",
		Path:            "pkg/foo/foo_test.go",
		Function:        "TestFoo",
		Line:            42,
		CriteriaRefs:    []string{"CRIT-1"},
		RequirementRefs: []string{"REQ-1"},
	}

	if target.Language != "go" || target.Function != "TestFoo" {
		t.Errorf("unexpected target fields: %+v", target)
	}
}
