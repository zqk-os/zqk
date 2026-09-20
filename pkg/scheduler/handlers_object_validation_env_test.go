package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestWithZQKJobIDEnv_IncludesJobID(t *testing.T) {
	t.Parallel()
	base := []string{"A=B"}
	out := withZQKJobIDEnv(base, "SCH-val")

	wantJob := zqkenv.JobID().Name() + "=SCH-val"
	foundA, foundJob := false, false
	for _, e := range out {
		if e == "A=B" {
			foundA = true
		}
		if e == wantJob {
			foundJob = true
		}
	}

	if !foundA {
		t.Fatal("expected base env to be preserved")
	}
	if !foundJob {
		t.Fatal("expected job id env to be injected")
	}
}
