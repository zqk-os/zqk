package object

import "testing"

func TestRequirementForObjectCommand(t *testing.T) {
	t.Parallel()
	if got := requirementForObjectCommand("bulk"); got != schedulerRequirementOptional {
		t.Fatalf("bulk requirement = %v, want optional", got)
	}
	if got := requirementForObjectCommand("list"); got != schedulerRequirementOptional {
		t.Fatalf("list requirement = %v, want optional", got)
	}
	if got := requirementForObjectCommand("get"); got != schedulerRequirementNone {
		t.Fatalf("get requirement = %v, want none", got)
	}
}

func TestEvaluateSchedulerGuard(t *testing.T) {
	t.Parallel()
	block, warn := evaluateSchedulerGuard(schedulerRequirementRequired, false, false)
	if !block || warn {
		t.Fatalf("required/down/no-override expected block=true warn=false, got block=%v warn=%v", block, warn)
	}
	block, warn = evaluateSchedulerGuard(schedulerRequirementRequired, false, true)
	if block || !warn {
		t.Fatalf("required/down/override expected block=false warn=true, got block=%v warn=%v", block, warn)
	}
}
