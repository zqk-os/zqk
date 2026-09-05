package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// SyncEvent represents a state change broadcast to the mesh.
type SyncEvent struct {
	NodeID    string    `json:"node_id"`
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"` // e.g., "GraphState" or "Schema"
	ItemID    string    `json:"item_id"`
	Signature string    `json:"signature,omitempty"`
}

// PayloadForSignature returns the canonical byte representation of the event for signing.
func (e *SyncEvent) PayloadForSignature() []byte {
	return []byte(fmt.Sprintf("%s|%d|%s|%s", e.NodeID, e.Timestamp.UnixNano(), e.Kind, e.ItemID))
}

// StateSyncer defines how nodes synchronize state without direct polling.
type StateSyncer interface {
	Start(ctx context.Context) error
	Stop() error
	Broadcast(ctx context.Context, event SyncEvent) error
	Subscribe(handler func(event SyncEvent))
}

// GossipSyncer implements StateSyncer using UDP broadcast/multicast and verifies via GraphProvider.
type GossipSyncer struct {
	addr      *net.UDPAddr
	conn      *net.UDPConn
	graphPool provider.ConnectionPool
	verifier  *TrustVerifier
	handlers  []func(SyncEvent)
	mu        sync.RWMutex
	cancel    context.CancelFunc
}

// NewGossipSyncer creates a new GossipSyncer. If graphPool is nil, it just routes events.
func NewGossipSyncer(address string, pool provider.ConnectionPool, verifier *TrustVerifier) (*GossipSyncer, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve udp addr: %w", err)
	}

	return &GossipSyncer{
		addr:      addr,
		graphPool: pool,
		verifier:  verifier,
	}, nil
}

func (g *GossipSyncer) Start(ctx context.Context) error {
	conn, err := net.ListenUDP("udp", g.addr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	g.conn = conn

	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	go g.listen(ctx)
	return nil
}

func (g *GossipSyncer) Stop() error {
	if g.cancel != nil {
		g.cancel()
	}
	if g.conn != nil {
		return g.conn.Close()
	}
	return nil
}

func (g *GossipSyncer) Broadcast(ctx context.Context, event SyncEvent) error {
	if g.conn == nil {
		return fmt.Errorf("syncer not started")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// In a real implementation we'd sign the payload here if we have the private key,
	// but for this iteration, we assume event.Signature is already populated by the caller
	// or we don't broadcast signed events yet if not provided.

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// We use the same connection to write to the broadcast/multicast address
	_, err = g.conn.WriteToUDP(data, g.addr)
	if err != nil {
		return fmt.Errorf("broadcast write: %w", err)
	}

	return nil
}

func (g *GossipSyncer) Subscribe(handler func(event SyncEvent)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.handlers = append(g.handlers, handler)
}

func (g *GossipSyncer) listen(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_ = g.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := g.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			var event SyncEvent
			if err := json.Unmarshal(buf[:n], &event); err != nil {
				continue
			}

			// Validate cryptographic trust
			if g.verifier != nil {
				state := SyncedState{
					NodeID:    event.NodeID,
					Payload:   event.PayloadForSignature(),
					Signature: event.Signature,
				}
				if err := g.verifier.VerifyState(ctx, state); err != nil {
					// Invalid signature or unknown node
					continue
				}
			}

			// Validate with graph DB if pool is available
			if g.graphPool != nil {
				g.validateWithGraph(ctx, event)
			}

			g.mu.RLock()
			handlers := g.handlers
			g.mu.RUnlock()

			for _, h := range handlers {
				go h(event)
			}
		}
	}
}

func (g *GossipSyncer) validateWithGraph(ctx context.Context, event SyncEvent) {
	// Example: Query graph DB to ensure the state actually exists or was updated
	// In a real implementation we'd check object hashes or timestamps.
	conn, err := g.graphPool.GetConnection(ctx)
	if err != nil {
		return
	}
	defer func() { _ = g.graphPool.ReturnConnection(conn) }()

	// Just a simple ping or check (e.g. MATCH (n {id: $id}) RETURN n)
	_, _ = conn.ExecuteQuery(ctx, provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    "MATCH (n) WHERE n.id = $id RETURN count(n)",
		Params:   map[string]any{objects.FieldKeyID: event.ItemID},
	})
}
