package agent_feed

import (
	"context"
	"testing"
)

type mockDelivery struct {
	deliveredFeeds []string
}

func (m *mockDelivery) Deliver(ctx context.Context, feed *AgentFeed, payload interface{}) error {
	m.deliveredFeeds = append(m.deliveredFeeds, feed.ID)
	return nil
}

func TestNestedSwarmOrchestrator(t *testing.T) {
	t.Parallel()

	md := &mockDelivery{}
	orchestrator := NewNestedSwarmOrchestrator(md)

	feeds := []*AgentFeed{
		{ID: "feed-1", Enabled: true, DeliveryMode: ModeLog},
		{ID: "feed-2", Enabled: false, DeliveryMode: ModeLog},
		{ID: "feed-3", Enabled: true, DeliveryMode: ModeOff},
		{ID: "feed-4", Enabled: true, DeliveryMode: ModeNotify},
	}

	errs := orchestrator.OrchestrateNestedSwarm(context.Background(), feeds, "test payload")
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if len(md.deliveredFeeds) != 2 {
		t.Fatalf("expected 2 delivered feeds, got %d", len(md.deliveredFeeds))
	}

	if md.deliveredFeeds[0] != "feed-1" || md.deliveredFeeds[1] != "feed-4" {
		t.Errorf("unexpected delivered feeds: %v", md.deliveredFeeds)
	}
}
