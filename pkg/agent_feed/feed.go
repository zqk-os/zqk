package agent_feed

import (
	"context"
	"fmt"
)

type DeliveryMode string

const (
	ModeOff       DeliveryMode = "off"
	ModeLog       DeliveryMode = "log"
	ModeClipboard DeliveryMode = "clipboard"
	ModePaste     DeliveryMode = "paste"
	ModeNotify    DeliveryMode = "notify"
)

type AgentFeed struct {
	ID                      string
	Enabled                 bool
	DeliveryMode            DeliveryMode
	ContractSchemaVersion   string
	ConfigPathOverride      string
	EventsJSONLPathOverride string
}

type DeliveryService interface {
	Deliver(ctx context.Context, feed *AgentFeed, payload interface{}) error
}

type NestedSwarmOrchestrator struct {
	Delivery DeliveryService
}

func NewNestedSwarmOrchestrator(d DeliveryService) *NestedSwarmOrchestrator {
	return &NestedSwarmOrchestrator{Delivery: d}
}

func (n *NestedSwarmOrchestrator) OrchestrateNestedSwarm(ctx context.Context, feeds []*AgentFeed, payload interface{}) []error {
	var errs []error
	for _, f := range feeds {
		if !f.Enabled || f.DeliveryMode == ModeOff {
			continue
		}
		if err := n.Delivery.Deliver(ctx, f, payload); err != nil {
			errs = append(errs, fmt.Errorf("failed to deliver to feed %s: %w", f.ID, err))
		}
	}
	return errs
}
