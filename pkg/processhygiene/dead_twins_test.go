package processhygiene

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestDeadTwinsRemoved(t *testing.T) {
	cmd := testkit.ManagedCommand(t, t.Context(), "go", "list", "./...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}

	packages := strings.Split(string(out), "\n")
	deadTwins := []string{
		"github.com/zqk-os/zqk/pkg/agent_feed",
		"github.com/zqk-os/zqk/pkg/utilities/chunking",
		"github.com/zqk-os/zqk/pkg/utilities/mcp_harness",
		"github.com/zqk-os/zqk/pkg/utilities/sortutil",
		"github.com/zqk-os/zqk/pkg/licensing",
		"github.com/zqk-os/zqk/pkg/schedulercore",
	}

	for _, p := range packages {
		for _, dt := range deadTwins {
			if p == dt {
				t.Errorf("dead twin package still exists in go list: %s", dt)
			}
		}
	}
}
