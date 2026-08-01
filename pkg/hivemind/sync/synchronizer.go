package sync

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/hivemind"
	"github.com/lanceman/zqk/pkg/logging"
)

// Indexer represents the dependency capable of indexing a document
type Indexer interface {
	Index(ctx context.Context, id string, text string) error
}

// ObjectLoader abstracts loading object content
type ObjectLoader interface {
	LoadContent(ctx context.Context, id string) (string, error)
}

// HiveMindSynchronizer reconciles the graph store and vector indices
type HiveMindSynchronizer struct {
	store  hivemind.MemoryStore
	worker Indexer
	loader ObjectLoader
	cancel context.CancelFunc
}

// NewHiveMindSynchronizer creates a new synchronizer instance
func NewHiveMindSynchronizer(store hivemind.MemoryStore, worker Indexer, loader ObjectLoader) *HiveMindSynchronizer {
	return &HiveMindSynchronizer{
		store:  store,
		worker: worker,
		loader: loader,
	}
}

// Start begins the periodic synchronization loop in the background
func (s *HiveMindSynchronizer) Start(ctx context.Context, interval time.Duration) {
	syncCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	goroutinelabels.NewGoroutine("HiveMindSynchronizer", "Periodic reconciliation of graph and vector indices").StartSimple(func() {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-syncCtx.Done():
				return
			case <-ticker.C:
				if err := s.SyncNow(syncCtx); err != nil {
					logging.Fluent(logger).Error("Synchronizer failed to complete cycle", err).Log()
				}
			}
		}
	})
}

// Stop halts the background synchronizer loop
func (s *HiveMindSynchronizer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// SyncNow performs a single reconciliation pass immediately
func (s *HiveMindSynchronizer) SyncNow(ctx context.Context) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Limit to a reasonable batch size per pass
	missingIDs, err := s.store.FindObjectsMissingVectors(ctx, 100)
	if err != nil {
		return err
	}

	if len(missingIDs) == 0 {
		return nil
	}

	logging.Fluent(logger).Info("Reconciling objects missing vector indices").Int("count", len(missingIDs)).Log()

	for _, id := range missingIDs {
		// Load the object content
		content, err := s.loader.LoadContent(ctx, id)
		if err != nil {
			logging.Fluent(logger).Error("Failed to load object content for indexing", err).String("id", id).Log()
			continue
		}

		// Index the object
		if err := s.worker.Index(ctx, id, content); err != nil {
			logging.Fluent(logger).Error("Failed to index object", err).String("id", id).Log()
			continue
		}

		// Link vector ID in the graph store (assuming vectorID is the same as objectID)
		vectorID := id
		if err := s.store.LinkVectorID(ctx, id, vectorID); err != nil {
			logging.Fluent(logger).Error("Failed to link vector ID in graph store", err).String("id", id).Log()
			continue
		}
	}

	return nil
}
