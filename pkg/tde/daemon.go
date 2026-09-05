package tde

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// Mutator is the interface for applying high-risk mutations.
type Mutator interface {
	ApplyEnvelope(ctx context.Context, env Envelope) error
}

// Daemon runs the background loop to commit expired TDE envelopes.
type Daemon struct {
	projectRoot string
	wal         *StagingWAL
	mutator     Mutator
	interval    time.Duration
	stopCh      chan struct{}
}

// NewDaemon creates a new TDE daemon.
func NewDaemon(projectRoot string, wal *StagingWAL, mutator Mutator, interval time.Duration) *Daemon {
	return &Daemon{
		projectRoot: projectRoot,
		wal:         wal,
		mutator:     mutator,
		interval:    interval,
		stopCh:      make(chan struct{}),
	}
}

// Start begins the background loop.
func (d *Daemon) Start() {
	go d.loop()
}

// Stop halts the background loop.
func (d *Daemon) Stop() {
	close(d.stopCh)
}

func (d *Daemon) loop() {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.processExpired()
		}
	}
}

// ProcessExpired runs a single pass of the daemon loop (useful for testing).
func (d *Daemon) ProcessExpired(ctx context.Context) error {
	return d.processExpiredContext(ctx)
}

func (d *Daemon) processExpired() {
	if err := d.processExpiredContext(context.Background()); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logger.Error("TDE Daemon process expired failed", err)
	}
}

func (d *Daemon) processExpiredContext(ctx context.Context) error {
	active, err := LoadActive(d.projectRoot)
	if err != nil {
		return errfmt.Newf("load active envelopes").Wrap(err)
	}

	now := time.Now().UTC()
	for _, env := range active {
		if now.After(env.ExecuteAt) || now.Equal(env.ExecuteAt) {
			// REQ-503: Hard-Block Governance (Crystalline Fortress)
			// High-risk mutations (policy, keystore, requirement) require explicit human authorization.
			if isHighRisk(env) {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logger.Warn("ABORT: High-risk TDE envelope blocked from auto-commit. Requires manual authorization.",
					logging.String("envelope_id", env.ID),
					logging.String("kind", env.Kind))

				// Move to 'governance_block' status in pulse (simulated via log)
				logger.Info(fmt.Sprintf("⚡ [HIVE PULSE] [governance_block] system: Blocked auto-commit of %s %s (%s)\n", env.Operation, env.TargetID, env.Kind))
				continue
			}

			// Apply mutation
			if err := d.mutator.ApplyEnvelope(ctx, env); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logger.Error("Failed to apply TDE envelope", err, logging.String("envelope_id", env.ID))
				continue
			}

			// Mark committed
			if err := d.wal.MarkCommitted(env.ID); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logger.Error("Failed to mark TDE envelope committed", err, logging.String("envelope_id", env.ID))
				continue
			}
		}
	}
	return d.wal.Sync()
}

func isHighRisk(env Envelope) bool {
	highRiskKinds := map[string]bool{
		"policy":      true,
		"keystore":    true,
		"requirement": true,
		"reputation":  true,
	}

	if highRiskKinds[env.Kind] {
		return true
	}

	// REQ-503: Harden check by ID prefix (Defense in Depth)
	// Even if Kind is lied about, we block known system ID patterns.
	idPrefixes := []string{"POL-", "REQ-", "GOAL-", "KEY-", "REP-"}
	for _, prefix := range idPrefixes {
		if len(env.TargetID) >= len(prefix) && env.TargetID[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}
