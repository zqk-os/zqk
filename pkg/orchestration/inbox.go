package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// Message represents a message or backlog_item to be routed.
type Message struct {
	ID          string
	Kind        string // e.g., "backlog_item"
	Destination string
	Payload     map[string]any
	Timestamp   time.Time
}

// AutonomyInbox provides raw inbox routing service over Neo4j/MemGraph
// for routing backlog_items or messages safely without using a distributed cache.
type AutonomyInbox struct {
	graphPool provider.ConnectionPool
}

// NewAutonomyInbox creates a new AutonomyInbox.
func NewAutonomyInbox(pool provider.ConnectionPool) *AutonomyInbox {
	return &AutonomyInbox{
		graphPool: pool,
	}
}

// RouteMessage routes a message or backlog_item to the appropriate destination
// directly within the graph database.
func (i *AutonomyInbox) RouteMessage(ctx context.Context, msg Message) error {
	return i.graphPool.Execute(ctx, func(conn provider.GraphConnection) error {
		// Scaffold: implementation to insert the message node and link it to the destination

		node := provider.Node{
			ID:     msg.ID,
			Labels: []string{"Message", msg.Kind},
			Properties: map[string]any{
				"destination":        msg.Destination,
				"timestamp":          msg.Timestamp.Unix(),
				objects.FieldKeyKind: msg.Kind, // queried by intent heuristics (labels alone are not props)
			},
		}

		// Merge payload into properties
		for k, v := range msg.Payload {
			node.Properties["payload_"+k] = v
		}

		if err := conn.CreateNode(ctx, node); err != nil {
			return fmt.Errorf("failed to create message node: %w", err)
		}

		// Link to destination node if applicable
		edge := provider.Edge{
			FromID: msg.ID,
			ToID:   msg.Destination,
			Type:   "ROUTED_TO",
			Properties: map[string]any{
				"routed_at": time.Now().Unix(),
			},
		}

		if err := conn.CreateEdge(ctx, edge); err != nil {
			return fmt.Errorf("failed to route message to destination: %w", err)
		}

		return nil
	})
}
