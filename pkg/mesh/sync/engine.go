package sync

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mesh"
)

// StateProvider abstracts the fetching of local state that needs to be synchronized.
type StateProvider interface {
	GetUnsyncedState(ctx context.Context, limit int) ([]mesh.SyncEvent, error)
	MarkSynced(ctx context.Context, itemID string) error
}

// PeerDiscoverer abstracts the discovery of peers.
type PeerDiscoverer interface {
	GetPeers(ctx context.Context) ([]mesh.RemoteKernel, error)
}

// Engine reconciles local state with discovered peers in the background.
type Engine struct {
	syncer     mesh.StateSyncer
	provider   StateProvider
	discoverer PeerDiscoverer
	cancel     context.CancelFunc
}

// NewEngine creates a new Engine.
func NewEngine(syncer mesh.StateSyncer, provider StateProvider, discoverer PeerDiscoverer) *Engine {
	return &Engine{
		syncer:     syncer,
		provider:   provider,
		discoverer: discoverer,
	}
}

// Start begins the periodic synchronization loop in the background.
func (e *Engine) Start(ctx context.Context, interval time.Duration) {
	syncCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	goroutinelabels.NewGoroutine("MeshSyncEngine", "Periodic reconciliation of mesh state").StartSimple(func() {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-syncCtx.Done():
				return
			case <-ticker.C:
				if err := e.ReconcileNow(syncCtx); err != nil {
					logging.Fluent(logger).Error("Mesh sync engine failed to complete cycle", err).Log()
				}
			}
		}
	})
}

// Stop halts the background synchronizer loop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// ReconcileNow performs a single reconciliation pass immediately.
func (e *Engine) ReconcileNow(ctx context.Context) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Ensure we have discovered peers before broadcasting (optional check)
	if e.discoverer != nil {
		peers, err := e.discoverer.GetPeers(ctx)
		if err != nil {
			logging.Fluent(logger).Error("Failed to discover peers", err).Log()
			// We might choose to proceed anyway or return.
			// Let's proceed as gossip might still work if peers are managed internally by syncer.
		} else if len(peers) == 0 {
			// No peers to sync with
			return nil
		}
	}

	events, err := e.provider.GetUnsyncedState(ctx, 100)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return nil
	}

	logging.Fluent(logger).Info("Reconciling local state to mesh").Int("count", len(events)).Log()

	for _, event := range events {
		if err := e.syncer.Broadcast(ctx, event); err != nil {
			logging.Fluent(logger).Error("Failed to broadcast state", err).ItemID(event.ItemID).Log()
			continue
		}

		if err := e.provider.MarkSynced(ctx, event.ItemID); err != nil {
			logging.Fluent(logger).Error("Failed to mark state as synced", err).ItemID(event.ItemID).Log()
		}
	}

	return nil
}
