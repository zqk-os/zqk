package object

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/entitlements"
)

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

func TestObjectPersistentPreRun_ElevatedInternalGate(t *testing.T) {
	prev := entitlements.CurrentCheckerForTest()
	defer entitlements.RegisterChecker(prev)
	entitlements.RegisterChecker(&entitlements.CommunityChecker{})
	brand.SetExecutableName("zqk")
	defer brand.SetExecutableName("zqk")

	root := NewObjectCmd()
	getCmd, _, err := root.Find([]string{"get"})
	if err != nil || getCmd == nil {
		t.Fatalf("find get: %v", err)
	}
	_ = root.PersistentFlags().Set(FlagElevatedInternal, "true")
	// Parent PersistentPreRunE is what production uses for all verbs.
	if err := root.PersistentPreRunE(getCmd, []string{"BLI-1"}); err == nil {
		t.Fatal("expected community --internal on get to fail via object PersistentPreRunE")
	}
}
