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
