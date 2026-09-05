package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/mesh"
)

type mockStateSyncer struct {
	events []mesh.SyncEvent
	err    error
}

func (m *mockStateSyncer) Start(ctx context.Context) error { return nil }
func (m *mockStateSyncer) Stop() error                     { return nil }
func (m *mockStateSyncer) Broadcast(ctx context.Context, event mesh.SyncEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}
func (m *mockStateSyncer) Subscribe(handler func(event mesh.SyncEvent)) {}

type mockStateProvider struct {
	events  []mesh.SyncEvent
	synced  []string
	getErr  error
	markErr error
}

func (m *mockStateProvider) GetUnsyncedState(ctx context.Context, limit int) ([]mesh.SyncEvent, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if limit < len(m.events) {
		return m.events[:limit], nil
	}
	return m.events, nil
}

func (m *mockStateProvider) MarkSynced(ctx context.Context, itemID string) error {
	if m.markErr != nil {
		return m.markErr
	}
	m.synced = append(m.synced, itemID)
	return nil
}

type mockPeerDiscoverer struct {
	peers []mesh.RemoteKernel
	err   error
}

func (m *mockPeerDiscoverer) GetPeers(ctx context.Context) ([]mesh.RemoteKernel, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.peers, nil
}

func TestEngine_ReconcileNow(t *testing.T) {
	ctx := context.Background()

	t.Run("no peers discovered", func(t *testing.T) {
		syncer := &mockStateSyncer{}
		provider := &mockStateProvider{
			events: []mesh.SyncEvent{{ItemID: "123"}},
		}
		discoverer := &mockPeerDiscoverer{
			peers: nil, // empty peers
		}

		engine := NewEngine(syncer, provider, discoverer)
		err := engine.ReconcileNow(ctx)
		if err != nil {
			t.Fatalf("expected nil err, got: %v", err)
		}
		if len(syncer.events) != 0 {
			t.Fatalf("expected 0 broadcast events since no peers, got %d", len(syncer.events))
		}
	})

	t.Run("peers discovered, successful sync", func(t *testing.T) {
		syncer := &mockStateSyncer{}
		provider := &mockStateProvider{
			events: []mesh.SyncEvent{{ItemID: "123"}, {ItemID: "456"}},
		}
		discoverer := &mockPeerDiscoverer{
			peers: []mesh.RemoteKernel{{ID: "peer1"}},
		}

		engine := NewEngine(syncer, provider, discoverer)
		err := engine.ReconcileNow(ctx)
		if err != nil {
			t.Fatalf("expected nil err, got: %v", err)
		}
		if len(syncer.events) != 2 {
			t.Fatalf("expected 2 broadcast events, got %d", len(syncer.events))
		}
		if len(provider.synced) != 2 {
			t.Fatalf("expected 2 synced marks, got %d", len(provider.synced))
		}
	})

	t.Run("provider returns error on get", func(t *testing.T) {
		syncer := &mockStateSyncer{}
		provider := &mockStateProvider{
			getErr: errors.New("get error"),
		}
		discoverer := &mockPeerDiscoverer{
			peers: []mesh.RemoteKernel{{ID: "peer1"}},
		}

		engine := NewEngine(syncer, provider, discoverer)
		err := engine.ReconcileNow(ctx)
		if err == nil || err.Error() != "get error" {
			t.Fatalf("expected 'get error', got: %v", err)
		}
	})

	t.Run("syncer error ignores sync mark", func(t *testing.T) {
		syncer := &mockStateSyncer{
			err: errors.New("broadcast failed"),
		}
		provider := &mockStateProvider{
			events: []mesh.SyncEvent{{ItemID: "123"}},
		}
		discoverer := &mockPeerDiscoverer{
			peers: []mesh.RemoteKernel{{ID: "peer1"}},
		}

		engine := NewEngine(syncer, provider, discoverer)
		err := engine.ReconcileNow(ctx)
		if err != nil {
			t.Fatalf("expected nil err (errors are logged), got: %v", err)
		}

		if len(provider.synced) != 0 {
			t.Fatalf("expected 0 synced marks due to broadcast error, got %d", len(provider.synced))
		}
	})
}

func TestEngine_StartStop(t *testing.T) {
	syncer := &mockStateSyncer{}
	provider := &mockStateProvider{}
	engine := NewEngine(syncer, provider, nil)

	ctx := context.Background()
	engine.Start(ctx, 10*time.Millisecond)

	time.Sleep(30 * time.Millisecond)
	engine.Stop()

	// Just ensures it doesn't panic and gracefully stops.
}
