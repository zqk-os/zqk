package objects

import "testing"

func TestTaskStepIsClosed(t *testing.T) {
	t.Parallel()
	closed := []string{"completed", "complete", "implemented", "skipped", "done", "CLOSED", " Completed "}
	for _, s := range closed {
		if !TaskStepIsClosed(s) {
			t.Fatalf("TaskStepIsClosed(%q)=false, want true", s)
		}
	}
	open := []string{"", "pending", "in_progress", "pending_verification", "error"}
	for _, s := range open {
		if TaskStepIsClosed(s) {
			t.Fatalf("TaskStepIsClosed(%q)=true, want false", s)
		}
	}
}
