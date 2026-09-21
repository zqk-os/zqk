package scheduler

import "testing"

func TestRunHardcodedGoLiteralsScan(t *testing.T) {
	t.Parallel()
	res := runHardcodedGoLiteralsScan(".")
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res["status"] != "passed" {
		t.Fatalf("expected status passed, got %v", res["status"])
	}
}
