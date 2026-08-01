package scheduler

import (
	"testing"
	"time"
)

func TestJobStateRegistry_Summarize(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	reg := NewJobStateRegistry(tmp)
	if err := reg.RegisterExecution("SCH-TEST", "exec-1", 1234); err != nil {
		t.Fatalf("RegisterExecution: %v", err)
	}
	if err := reg.CompleteExecution("SCH-TEST", "exec-1", "ok"); err != nil {
		t.Fatalf("CompleteExecution: %v", err)
	}
	sum, err := reg.Summarize(1 * time.Second)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if sum.TotalFiles == 0 {
		t.Fatalf("expected non-zero total files")
	}
	if sum.ByState["completed"] == 0 {
		t.Fatalf("expected completed count > 0, got %+v", sum.ByState)
	}
}
