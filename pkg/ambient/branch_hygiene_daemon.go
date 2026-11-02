package ambient

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Traceability: BLI-1793578878406936000-670dab43, CRIT-1793578812700305000-d188af82
// Autonomous Branch Hygiene Daemon — middle-brain holon that periodically prunes
// merged integration branches via an injected BranchPruner (typically gitplumbing).

const (
	defaultHygieneHolonID   = "ambient-branch-hygiene"
	defaultHygieneHolonName = "Autonomous Branch Hygiene Daemon"
	defaultHygieneRole      = "branch-hygiene"
	defaultHygieneEcosystem = "spinal-cord"
	defaultHygieneInterval  = 5 * time.Minute
)

var (
	// ErrBranchHygieneEngineRequired is returned when constructing without a pruner.
	ErrBranchHygieneEngineRequired = errfmt.Errorf("ambient branch hygiene engine is required")
)

// BranchPruner is the prune surface used by the hygiene daemon (satisfied by
// gitplumbing.BranchHygieneEngine).
type BranchPruner interface {
	PruneMergedBranches(ctx context.Context, dryRun bool) ([]string, error)
}

// BranchHygieneDaemon is an ambient Holon that drives autonomous branch pruning.
type BranchHygieneDaemon struct {
	id        string
	name      string
	role      string
	ecosystem string
	engine    BranchPruner
	interval  time.Duration
	dryRun    bool

	mu     sync.RWMutex
	state  HolonState
	msg    string
	hb     time.Time
	cancel context.CancelFunc
}

// HygieneDaemonOption configures BranchHygieneDaemon construction.
type HygieneDaemonOption func(*BranchHygieneDaemon)

// WithHygieneInterval sets the poll interval (must be > 0).
func WithHygieneInterval(d time.Duration) HygieneDaemonOption {
	return func(daemon *BranchHygieneDaemon) {
		if d > 0 {
			daemon.interval = d
		}
	}
}

// WithHygieneDryRun enables dry-run pruning (evaluate without deleting).
func WithHygieneDryRun(dryRun bool) HygieneDaemonOption {
	return func(daemon *BranchHygieneDaemon) {
		daemon.dryRun = dryRun
	}
}

// WithHygieneIdentity overrides holon identity fields.
func WithHygieneIdentity(id, name, role, ecosystem string) HygieneDaemonOption {
	return func(daemon *BranchHygieneDaemon) {
		if id != "" {
			daemon.id = id
		}
		if name != "" {
			daemon.name = name
		}
		if role != "" {
			daemon.role = role
		}
		if ecosystem != "" {
			daemon.ecosystem = ecosystem
		}
	}
}

// NewBranchHygieneDaemon constructs a registered BranchHygieneDaemon holon.
func NewBranchHygieneDaemon(engine BranchPruner, opts ...HygieneDaemonOption) (*BranchHygieneDaemon, error) {
	if engine == nil {
		return nil, ErrBranchHygieneEngineRequired
	}
	d := &BranchHygieneDaemon{
		id:        defaultHygieneHolonID,
		name:      defaultHygieneHolonName,
		role:      defaultHygieneRole,
		ecosystem: defaultHygieneEcosystem,
		engine:    engine,
		interval:  defaultHygieneInterval,
		state:     HolonStateRegistered,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
	return d, nil
}

// ID implements Holon.
func (d *BranchHygieneDaemon) ID() string { return d.id }

// Name implements Holon.
func (d *BranchHygieneDaemon) Name() string { return d.name }

// Role implements Holon.
func (d *BranchHygieneDaemon) Role() string { return d.role }

// Ecosystem implements Holon.
func (d *BranchHygieneDaemon) Ecosystem() string { return d.ecosystem }

// Tick runs one prune cycle. Failures mark the holon degraded and return the error.
func (d *BranchHygieneDaemon) Tick(ctx context.Context) ([]string, error) {
	pruned, err := d.engine.PruneMergedBranches(ctx, d.dryRun)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hb = time.Now()
	if err != nil {
		d.state = HolonStateDegraded
		d.msg = err.Error()
		return nil, err
	}
	if d.state != HolonStateRunning && d.state != HolonStateStarting {
		// Keep registered until Start promotes; successful tick while registered stays healthy.
		if d.state == HolonStateRegistered {
			d.msg = "tick ok"
		}
	} else {
		d.state = HolonStateRunning
		d.msg = "tick ok"
	}
	return pruned, nil
}

// Start implements Holon: polls until ctx cancel. Returns ctx.Err() on cancel.
func (d *BranchHygieneDaemon) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.state = HolonStateStarting
	d.hb = time.Now()
	d.mu.Unlock()

	// Initial tick on start, then interval polls.
	d.mu.Lock()
	d.state = HolonStateRunning
	d.mu.Unlock()
	if _, err := d.Tick(runCtx); err != nil {
		d.mu.Lock()
		d.state = HolonStateDegraded
		d.mu.Unlock()
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			d.mu.Lock()
			if d.state != HolonStateStopped {
				d.state = HolonStateStopped
			}
			d.mu.Unlock()
			return runCtx.Err()
		case <-ticker.C:
			if _, err := d.Tick(runCtx); err != nil {
				d.mu.Lock()
				d.state = HolonStateDegraded
				d.mu.Unlock()
			}
		}
	}
}

// Stop implements Holon.
func (d *BranchHygieneDaemon) Stop() error {
	d.mu.Lock()
	cancel := d.cancel
	d.cancel = nil
	d.state = HolonStateStopped
	d.msg = "stopped"
	d.hb = time.Now()
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Status implements Holon.
func (d *BranchHygieneDaemon) Status() HolonStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return HolonStatus{
		ID:        d.id,
		Name:      d.name,
		Role:      d.role,
		Ecosystem: d.ecosystem,
		Health: HolonHealth{
			State:         d.state,
			Healthy:       d.state == HolonStateRunning || d.state == HolonStateRegistered,
			Message:       d.msg,
			LastHeartbeat: d.hb,
		},
	}
}

// Ensure BranchHygieneDaemon satisfies Holon for supervisor registration.
var _ Holon = (*BranchHygieneDaemon)(nil)
