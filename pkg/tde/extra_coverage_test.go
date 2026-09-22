// BLI-STARTER-COMMUNITY-046 / PRI-STARTER-COMMUNITY-046 coverage elevation
package tde

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExtraRegisterAndExecuteAction(t *testing.T) {
	ctx := context.Background()
	if err := ExecuteAction(ctx, Envelope{Operation: "missing-op"}); err == nil {
		t.Fatal("missing handler")
	}
	var saw string
	RegisterAction("extra-op", func(_ context.Context, env Envelope) error {
		saw = env.ID
		return nil
	})
	if err := ExecuteAction(ctx, Envelope{ID: "TDE-1", Operation: "extra-op"}); err != nil || saw != "TDE-1" {
		t.Fatalf("execute = %q %v", saw, err)
	}
	RegisterAction("extra-fail", func(context.Context, Envelope) error { return errors.New("boom") })
	if err := ExecuteAction(ctx, Envelope{Operation: "extra-fail"}); err == nil {
		t.Fatal("handler err")
	}

	wal, err := NewStagingWAL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(t.TempDir(), wal, &mockMutator{applied: map[string]bool{}}, 10*time.Millisecond)
	d.Start()
	time.Sleep(25 * time.Millisecond)
	d.Stop()
	_ = NewTransportEnforcer(nil)
	if _, err := (&FastPathEvaluator{}).Evaluate(ctx, nil); err == nil {
		t.Fatal("nil env")
	}
}
