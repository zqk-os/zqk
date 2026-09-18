package shockwave

import (
	"context"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockNode struct {
	mu               sync.Mutex
	receivedPayloads []interface{}
	receivedTiers    []TierMask
}

func (m *mockNode) Deliver(ctx context.Context, payload interface{}, tiers TierMask) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedPayloads = append(m.receivedPayloads, payload)
	m.receivedTiers = append(m.receivedTiers, tiers)
	return payload, nil
}

func TestGatewayDispatcher_Dispatch(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	dispatcher := NewGatewayDispatcher(logger, 10)

	node1 := &mockNode{}
	node2 := &mockNode{}
	dispatcher.RegisterNode("node-1", node1)
	dispatcher.RegisterNode("node-2", node2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dispatcher.Start(ctx)
	defer dispatcher.Stop()

	// Create a shockwave_router object
	routerObj := map[string]interface{}{
		objects.FieldKeyKind: "shockwave_router",
		objects.FieldKeyMetadata: map[string]interface{}{
			objects.FieldKeyID: "router-test",
		},
		objects.FieldKeySpec: map[string]interface{}{
			"routes": []interface{}{
				map[string]interface{}{
					"node_id": "node-1",
					"tiers":   []interface{}{"security", "syntax"},
				},
				map[string]interface{}{
					"node_id": "node-2",
					"tiers":   []interface{}{"semantic"},
				},
			},
		},
	}

	payload := "test-semantic-payload"

	results, err := dispatcher.Dispatch(ctx, routerObj, payload)
	if err != nil {
		t.Fatalf("unexpected error during dispatch: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results from reduce phase, got %d", len(results))
	}

	node1.mu.Lock()
	defer node1.mu.Unlock()
	if len(node1.receivedPayloads) != 1 {
		t.Fatalf("expected node-1 to receive 1 payload, got %d", len(node1.receivedPayloads))
	}
	if node1.receivedPayloads[0] != payload {
		t.Errorf("expected node-1 payload %v, got %v", payload, node1.receivedPayloads[0])
	}
	expectedTiers1 := TierSecurity | TierSyntax
	if node1.receivedTiers[0] != expectedTiers1 {
		t.Errorf("expected node-1 tiers %v, got %v", expectedTiers1, node1.receivedTiers[0])
	}

	node2.mu.Lock()
	defer node2.mu.Unlock()
	if len(node2.receivedPayloads) != 1 {
		t.Fatalf("expected node-2 to receive 1 payload, got %d", len(node2.receivedPayloads))
	}
	if node2.receivedPayloads[0] != payload {
		t.Errorf("expected node-2 payload %v, got %v", payload, node2.receivedPayloads[0])
	}
	expectedTiers2 := TierSemantic
	if node2.receivedTiers[0] != expectedTiers2 {
		t.Errorf("expected node-2 tiers %v, got %v", expectedTiers2, node2.receivedTiers[0])
	}
}

func TestGatewayDispatcher_Dispatch_InvalidKind(t *testing.T) {
	dispatcher := NewGatewayDispatcher(nil, 10)
	routerObj := map[string]interface{}{
		objects.FieldKeyKind: "not_shockwave_router",
	}
	_, err := dispatcher.Dispatch(context.Background(), routerObj, "payload")
	if err == nil {
		t.Fatalf("expected error for invalid kind, got nil")
	}
}
