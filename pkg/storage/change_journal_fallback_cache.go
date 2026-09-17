package storage

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	idgen "github.com/lanceman/zqk/pkg/storage/id_generation"
)

var (
	fallbackJournalGenerators      = make(map[string]*idgen.BatchIDGenerator)
	fallbackJournalGeneratorsMu    sync.RWMutex
	fallbackGeneratorsCreatedTotal atomic.Int64
	fallbackGeneratorsReusedTotal  atomic.Int64
)

// GetFallbackJournalGeneratorStats returns lifetime counters for created and reused fallback generators.
func GetFallbackJournalGeneratorStats() (created, reused int64) {
	return fallbackGeneratorsCreatedTotal.Load(), fallbackGeneratorsReusedTotal.Load()
}

// getFallbackChangeJournalGenerator returns a cached BatchIDGenerator for the given journalDir.
// It uses a Cross-Process File Lock (via sequence file) to prevent CAS collisions (DDOS) during concurrent tests.
func getFallbackChangeJournalGenerator(ctx context.Context, projectRoot, journalDir string) *idgen.BatchIDGenerator {
	var g *idgen.BatchIDGenerator
	var exists bool

	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))

	_ = concurrency.RunInRLockOrLog(
		&fallbackJournalGeneratorsMu,
		"fallback_journal_generator_get",
		logger,
		func() error {
			g, exists = fallbackJournalGenerators[journalDir]
			return nil
		},
	)
	if exists {
		fallbackGeneratorsReusedTotal.Add(1)
		return g
	}

	_ = concurrency.RunInLockOrLog(
		&fallbackJournalGeneratorsMu,
		"fallback_journal_generator_create",
		logger,
		func() error {
			if existing, ok := fallbackJournalGenerators[journalDir]; ok {
				g = existing
				fallbackGeneratorsReusedTotal.Add(1)
				return nil
			}
			g = idgen.GetBatchIDGenerator(ctx, journalDir, objects.KindChangeJournalEntry, "CHA", 3, 1)

			// Enforce Cross-Process File Locking (Pattern 6) by using the state directory for the sequence file.
			// This completely eliminates the CAS DDOS collisions during concurrent test execution.
			if projectRoot != "" {
				stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
				g.SetSequenceFileDir(stateDir)
			}

			fallbackJournalGenerators[journalDir] = g
			fallbackGeneratorsCreatedTotal.Add(1)
			return nil
		},
	)
	return g
}
