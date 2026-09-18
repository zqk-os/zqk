package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/internal/cli"
)

func TestSchedulerStateCommandRegistered(t *testing.T) {
	t.Parallel()
	cmd := NewSchedulerCmd()
	for _, c := range cmd.Commands() {
		if c.Name() != "state" {
			continue
		}
		ann := c.Annotations
		if ann == nil {
			t.Fatalf("scheduler state command must declare orchestration annotations")
		}
		if ann[cli.AnnRequiresSchedulerCheck] != "false" {
			t.Fatalf("scheduler state must set %s=false (avoid blocking PreRun when daemon runs)", cli.AnnRequiresSchedulerCheck)
		}
		if ann[cli.AnnRequiresSession] != "false" {
			t.Fatalf("scheduler state must set %s=false (avoid session contention with daemon)", cli.AnnRequiresSession)
		}
		return
	}
	t.Fatalf("expected scheduler state command to be registered")
}
