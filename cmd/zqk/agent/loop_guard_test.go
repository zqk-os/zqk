package agent

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestLoadLoopGuardConfig_Defaults(t *testing.T) {
	t.Setenv(zqkenv.AgentSyncMaxLoops().Name(), "")
	t.Setenv(zqkenv.AgentMaxVerificationAttempts().Name(), "")
	t.Setenv(zqkenv.AgentSyncMaxStagnantTicks().Name(), "")
	cfg := LoadLoopGuardConfig()
	if cfg.MaxSyncLoops != defaultMaxSyncLoops {
		t.Fatalf("MaxSyncLoops=%d want %d", cfg.MaxSyncLoops, defaultMaxSyncLoops)
	}
	if cfg.MaxVerificationAttempts != defaultMaxVerificationAttempts {
		t.Fatalf("MaxVerificationAttempts=%d want %d", cfg.MaxVerificationAttempts, defaultMaxVerificationAttempts)
	}
	if cfg.MaxStagnantProgressTicks != defaultMaxStagnantProgressTicks {
		t.Fatalf("MaxStagnantProgressTicks=%d want %d", cfg.MaxStagnantProgressTicks, defaultMaxStagnantProgressTicks)
	}
}

func TestLoadLoopGuardConfig_EnvOverride(t *testing.T) {
	t.Setenv(zqkenv.AgentSyncMaxLoops().Name(), "7")
	t.Setenv(zqkenv.AgentMaxVerificationAttempts().Name(), "2")
	t.Setenv(zqkenv.AgentSyncMaxStagnantTicks().Name(), "4")
	cfg := LoadLoopGuardConfig()
	if cfg.MaxSyncLoops != 7 || cfg.MaxVerificationAttempts != 2 || cfg.MaxStagnantProgressTicks != 4 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadLoopGuardConfig_NegativeOverrides(t *testing.T) {
	t.Setenv(zqkenv.AgentSyncMaxLoops().Name(), "-5")
	t.Setenv(zqkenv.AgentMaxVerificationAttempts().Name(), "0")
	t.Setenv(zqkenv.AgentSyncMaxStagnantTicks().Name(), "-1")
	cfg := LoadLoopGuardConfig()
	if cfg.MaxSyncLoops != defaultMaxSyncLoops {
		t.Fatalf("MaxSyncLoops=%d want %d", cfg.MaxSyncLoops, defaultMaxSyncLoops)
	}
	if cfg.MaxVerificationAttempts != defaultMaxVerificationAttempts {
		t.Fatalf("MaxVerificationAttempts=%d want %d", cfg.MaxVerificationAttempts, defaultMaxVerificationAttempts)
	}
	if cfg.MaxStagnantProgressTicks != defaultMaxStagnantProgressTicks {
		t.Fatalf("MaxStagnantProgressTicks=%d want %d", cfg.MaxStagnantProgressTicks, defaultMaxStagnantProgressTicks)
	}
}

func TestNewStagnationGuard_DefaultMax(t *testing.T) {
	g := newStagnationGuard(0)
	if g.max != defaultMaxStagnantProgressTicks {
		t.Fatalf("g.max=%d want %d", g.max, defaultMaxStagnantProgressTicks)
	}
}

func TestStagnationGuard_TripsOnUnchangedFingerprint(t *testing.T) {
	g := newStagnationGuard(3)
	if g.Observe("a") {
		t.Fatal("first observe should not trip")
	}
	if g.Observe("a") {
		t.Fatal("second should not trip")
	}
	if g.Observe("a") {
		t.Fatal("third should not trip (sameTick==max)")
	}
	if !g.Observe("a") {
		t.Fatal("fourth should trip (sameTick>max)")
	}
	if g.Observe("b") {
		t.Fatal("progress reset should clear trip")
	}
}

func TestTaskProgressFingerprint_IncludesAttempts(t *testing.T) {
	task := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusPendingVerification,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:                "Step 1",
				objects.FieldKeyStatus:               objects.ObjectStatusPendingVerification,
				objects.FieldKeyVerificationAttempts: 2,
			},
		},
	}
	fp1 := taskProgressFingerprint(task)
	task[objects.FieldKeyTaskSteps].([]any)[0].(map[string]any)[objects.FieldKeyVerificationAttempts] = 3
	fp2 := taskProgressFingerprint(task)
	if fp1 == fp2 {
		t.Fatalf("expected attempt change to alter fingerprint: %q", fp1)
	}
}

func TestTaskProgressFingerprint_EdgeCases(t *testing.T) {
	// 1. Nil task
	if got := taskProgressFingerprint(nil); got != "" {
		t.Fatalf("expected empty for nil task, got %q", got)
	}

	// 2. Nil stagnation guard
	var g *stagnationGuard
	if g.Observe("test") {
		t.Fatalf("expected false for nil stagnation guard")
	}

	// 3. Completeness validation steps
	taskWithCV := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyCompletenessValidation: []any{
			map[string]any{
				objects.FieldKeyName:                 "Check 1",
				objects.FieldKeyStatus:               objects.ObjectStatusApproved,
				objects.FieldKeyVerificationAttempts: "2",
			},
		},
	}
	fp := taskProgressFingerprint(taskWithCV)
	if fp == "" {
		t.Fatalf("expected non-empty fingerprint for task with completeness_validation")
	}
}

func TestVerificationAttemptsOf_Types(t *testing.T) {
	// Nil step
	if verificationAttemptsOf(nil) != 0 {
		t.Fatalf("expected 0 for nil step")
	}

	// Missing attempts key
	if verificationAttemptsOf(map[string]any{}) != 0 {
		t.Fatalf("expected 0 for missing key")
	}

	// int
	if verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: 5}) != 5 {
		t.Fatalf("expected 5 for int")
	}

	// int64
	if verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: int64(6)}) != 6 {
		t.Fatalf("expected 6 for int64")
	}

	// float64
	if verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: float64(7)}) != 7 {
		t.Fatalf("expected 7 for float64")
	}

	// string
	if verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: "8"}) != 8 {
		t.Fatalf("expected 8 for string")
	}

	// invalid type
	if verificationAttemptsOf(map[string]any{objects.FieldKeyVerificationAttempts: []string{"a"}}) != 0 {
		t.Fatalf("expected 0 for invalid type")
	}
}
