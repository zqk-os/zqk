package processhygiene

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDeadTwinsRemoved(t *testing.T) {
	cmd := exec.Command("go", "list", "./...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}

	packages := strings.Split(string(out), "\n")
	deadTwins := []string{
		"github.com/lanceman/zqk/pkg/agent_feed",
		"github.com/lanceman/zqk/pkg/utilities/chunking",
		"github.com/lanceman/zqk/pkg/utilities/mcp_harness",
		"github.com/lanceman/zqk/pkg/utilities/sortutil",
		"github.com/lanceman/zqk/pkg/licensing",
		"github.com/lanceman/zqk/pkg/schedulercore",
	}

	for _, p := range packages {
		for _, dt := range deadTwins {
			if p == dt {
				t.Errorf("dead twin package still exists in go list: %s", dt)
			}
		}
	}
}
