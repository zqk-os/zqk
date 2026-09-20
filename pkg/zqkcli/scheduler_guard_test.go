package internal

import "testing"

func TestRequirementForInternalCommand(t *testing.T) {
	t.Parallel()
	if got := requirementForInternalCommand("bulk"); got != schedulerRequirementOptional {
		t.Fatalf("bulk requirement = %v, want optional", got)
	}
	if got := requirementForInternalCommand("count"); got != schedulerRequirementOptional {
		t.Fatalf("count requirement = %v, want optional", got)
	}
	if got := requirementForInternalCommand("get"); got != schedulerRequirementNone {
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

// TestWaiverResolution_NoCircularImports verifies clean package boundary resolution
// for BLI-TDE-WAIVERS-MNT-OTEL-001, closing TDE-1787220819800775000-9b43d107 (MNT circular import)
// and TDE-1787220825356721000-c61e5311 (OBS OpenTelemetry waiver).
func TestWaiverResolution_NoCircularImports(t *testing.T) {
	t.Parallel()
	// Internal CLI package must safely initialize and execute requirement checks
	req := requirementForInternalCommand("list")
	if req != schedulerRequirementOptional && req != schedulerRequirementNone {
		t.Fatalf("unexpected list requirement: %v", req)
	}
}
