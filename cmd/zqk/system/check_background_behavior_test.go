package system

import (
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestShouldUseAsyncFollowForBackground_WhenZQKJobIDSet(t *testing.T) {
	t.Setenv(zqkenv.JobID().Name(), "SCH-test")
	if !shouldUseAsyncFollowForBackground() {
		t.Fatal("expected shouldUseAsyncFollowForBackground()=true when ZQK_JOB_ID is set")
	}
}

func TestShouldUseAsyncFollowForBackground_WhenZQKJobIDUnset(t *testing.T) {
	t.Setenv(zqkenv.JobID().Name(), "")
	if shouldUseAsyncFollowForBackground() {
		t.Fatal("expected shouldUseAsyncFollowForBackground()=false when ZQK_JOB_ID is empty")
	}
}
