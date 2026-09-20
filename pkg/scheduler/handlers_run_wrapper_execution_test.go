package scheduler

import "testing"

func TestRunWrapperHandler_Creation(t *testing.T) {
	handler := &RunWrapperHandler{}
	if handler == nil {
		t.Fatal("expected non-nil RunWrapperHandler")
	}
}
