package service

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/orchestration"
	"github.com/zqk-os/zqk/pkg/orchestration/ticker"
)

type mockOrchestrator struct{}

func (m *mockOrchestrator) ProcessIntent(ctx context.Context, intent orchestration.RawIntent) (orchestration.CapabilityRef, error) {
	return orchestration.CapabilityRef{ID: "test-cap"}, nil
}

func TestSynthesisService(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := lifecycle.GetOrCreateLifecycleWAL(tmpDir)
	orchestrator := &mockOrchestrator{}
	tck := ticker.NewActivityTicker()
	svc := NewSynthesisService(wal, orchestrator, tck)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Append intent event
	_ = wal.Append(&lifecycle.LifecycleEvent{
		EventType: "intent_submission",
		ID:        "INTENT-1",
	})
	_ = wal.Sync()

	// Run service
	go func() {
		_ = svc.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
}
