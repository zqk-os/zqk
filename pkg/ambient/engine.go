// Package ambient provides the autonomous holon runtime engine and conscious swarm coordination.
package ambient

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/liveness"
	"github.com/zqk-os/zqk/pkg/paths"
)

// EngineState represents the lifecycle state automaton of an AmbientEngine.
type EngineState string

const (
	// EngineStateStopped indicates the engine is halted and inactive.
	EngineStateStopped EngineState = "Stopped"
	// EngineStateStarting indicates the engine is initializing sub-components and background loops.
	EngineStateStarting EngineState = "Starting"
	// EngineStateRunning indicates the engine is actively executing ticks and coordinating workloads.
	EngineStateRunning EngineState = "Running"
	// EngineStateDegraded indicates the engine is operating with degraded health or isolated faults.
	EngineStateDegraded EngineState = "Degraded"
	// EngineStateStopping indicates the engine is undergoing graceful shutdown.
	EngineStateStopping EngineState = "Stopping"
)

// String returns the string representation of the EngineState.
func (s EngineState) String() string {
	return string(s)
}

// Default timing constants for ambient engine execution.
const (
	// DefaultTickInterval is the default tick duration for ambient engine loops.
	DefaultTickInterval = 100 * time.Millisecond
	// DefaultGracefulStopTimeout is the default duration to await graceful stop.
	DefaultGracefulStopTimeout = 5 * time.Second
)

// Sentinel errors for ambient engine lifecycle, configuration, and state transitions.
var (
	ErrNilEngine              = errfmt.Errorf("ambient engine cannot be nil")
	ErrEngineStopped          = errfmt.Errorf("ambient engine is stopped")
	ErrEngineNotRunning       = ErrEngineStopped
	ErrEngineAlreadyRunning   = errfmt.Errorf("ambient engine is already running")
	ErrEngineDegraded         = errfmt.Errorf("ambient engine is degraded")
	ErrInvalidTickInterval    = errfmt.Errorf("tick interval must be strictly positive")
	ErrInvalidGracefulTimeout = errfmt.Errorf("graceful stop timeout must be strictly positive")
	ErrNilSubConfig           = errfmt.Errorf("sub-component configuration cannot be nil")
)

// AmbientEngineConfig defines configuration parameters and sub-component configs for AmbientEngine.
type AmbientEngineConfig struct {
	TickInterval           time.Duration            `json:"tick_interval"`
	GracefulStopTimeout    time.Duration            `json:"graceful_stop_timeout"`
	IntentStreamConfig     *IntentStreamConfig      `json:"intent_stream_config"`
	EpochConfig            *EpochSynchronizerConfig `json:"epoch_config"`
	BackpressureConfig     *BackpressureConfig      `json:"backpressure_config"`
	FaultIsolatorConfig    *FaultIsolatorConfig     `json:"fault_isolator_config"`
	ConsensusConfig        *ConsensusArbiterConfig  `json:"consensus_config,omitempty"`
	ConsensusArbiterConfig *ConsensusArbiterConfig  `json:"consensus_arbiter_config,omitempty"`
	ReflexConfig           *ReflexLoopConfig        `json:"reflex_config,omitempty"`
	ReflexLoopConfig       *ReflexLoopConfig        `json:"reflex_loop_config,omitempty"`
	SwarmDispatchConfig    *SwarmDispatchConfig     `json:"swarm_dispatch_config,omitempty"`
	SwarmDispatcherConfig  *SwarmDispatcherConfig   `json:"swarm_dispatcher_config,omitempty"`
	StatePath              string                   `json:"state_path,omitempty"`
	ProjectRoot            string                   `json:"project_root,omitempty"`
	LivenessSocketPath     string                   `json:"liveness_socket_path,omitempty"`
	EnableLiveness         bool                     `json:"enable_liveness,omitempty"`
}

// DefaultAmbientEngineConfig returns production defaults for AmbientEngine.
func DefaultAmbientEngineConfig() AmbientEngineConfig {
	intentCfg := DefaultAmbientIntentStreamConfig()
	epochCfg := DefaultEpochSynchronizerConfig()
	backpressureCfg := DefaultBackpressureConfig()
	faultCfg := DefaultFaultIsolatorConfig()
	consensusCfg := DefaultConsensusArbiterConfig()
	reflexCfg := DefaultReflexConfig()
	swarmCfg := DefaultSwarmDispatchConfig()

	return AmbientEngineConfig{
		TickInterval:           DefaultTickInterval,
		GracefulStopTimeout:    DefaultGracefulStopTimeout,
		IntentStreamConfig:     &intentCfg,
		EpochConfig:            &epochCfg,
		BackpressureConfig:     &backpressureCfg,
		FaultIsolatorConfig:    &faultCfg,
		ConsensusConfig:        &consensusCfg,
		ConsensusArbiterConfig: &consensusCfg,
		ReflexConfig:           &reflexCfg,
		ReflexLoopConfig:       &reflexCfg,
		SwarmDispatchConfig:    &swarmCfg,
		SwarmDispatcherConfig:  &swarmCfg,
	}
}

// Validate verifies that the AmbientEngineConfig satisfies static floor invariants.
func (c AmbientEngineConfig) Validate() error {
	if c.TickInterval <= 0 {
		return ErrInvalidTickInterval
	}
	if c.GracefulStopTimeout <= 0 {
		return ErrInvalidGracefulTimeout
	}
	if c.IntentStreamConfig == nil || c.EpochConfig == nil || c.BackpressureConfig == nil || c.FaultIsolatorConfig == nil {
		return ErrNilSubConfig
	}
	if err := c.BackpressureConfig.Validate(); err != nil {
		return err
	}
	if err := c.FaultIsolatorConfig.Validate(); err != nil {
		return err
	}
	if c.ConsensusConfig == nil && c.ConsensusArbiterConfig == nil {
		return ErrNilSubConfig
	}
	if c.ConsensusConfig != nil {
		if err := c.ConsensusConfig.Validate(); err != nil {
			return err
		}
	}
	if c.ConsensusArbiterConfig != nil {
		if err := c.ConsensusArbiterConfig.Validate(); err != nil {
			return err
		}
	}
	if c.ReflexConfig == nil && c.ReflexLoopConfig == nil {
		return ErrNilSubConfig
	}
	if c.ReflexConfig != nil {
		if err := c.ReflexConfig.Validate(); err != nil {
			return err
		}
	}
	if c.ReflexLoopConfig != nil {
		if err := c.ReflexLoopConfig.Validate(); err != nil {
			return err
		}
	}
	if c.SwarmDispatchConfig != nil {
		if err := c.SwarmDispatchConfig.Validate(); err != nil {
			return err
		}
	}
	if c.SwarmDispatcherConfig != nil {
		if err := c.SwarmDispatcherConfig.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AmbientEngine coordinates the autonomous runtime microkernel loops, orchestrating
// the AmbientIntentStream, EpochSynchronizer, BackpressureRegulator, FaultIsolator,
// ConsensusArbiter, ReflexLoop, and SwarmDispatcher.
type AmbientEngine struct {
	mu                    sync.RWMutex
	config                AmbientEngineConfig
	state                 EngineState
	statePath             string
	intentStream          *AmbientIntentStream
	epochSynchronizer     *EpochSynchronizer
	backpressureRegulator *BackpressureRegulator
	faultIsolator         *FaultIsolator
	consensusArbiter      *ConsensusArbiter
	reflexLoop            *ReflexLoop
	swarmDispatcher       *SwarmDispatcher
	livenessListener      *liveness.PulseListener
	decayTracker          *liveness.DecayTracker

	startedAt      time.Time
	ticksExecuted  uint64
	cancelFn       context.CancelFunc
	supervisorDone chan struct{}
	supervisorWg   sync.WaitGroup
}

// NewAmbientEngine constructs a new AmbientEngine with validated configuration and initialized sub-components.
func NewAmbientEngine(cfg AmbientEngineConfig) (*AmbientEngine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	stream := NewAmbientIntentStream(*cfg.IntentStreamConfig)
	epochSyncer := NewEpochSynchronizer(*cfg.EpochConfig)
	regulator, err := NewBackpressureRegulator(*cfg.BackpressureConfig)
	if err != nil {
		return nil, err
	}
	isolator, err := NewFaultIsolator(*cfg.FaultIsolatorConfig)
	if err != nil {
		return nil, err
	}

	consensusCfg := cfg.ConsensusConfig
	if consensusCfg == nil {
		consensusCfg = cfg.ConsensusArbiterConfig
	}
	arbiter, err := NewConsensusArbiterWithConfig(*consensusCfg)
	if err != nil {
		return nil, err
	}

	reflexCfg := cfg.ReflexConfig
	if reflexCfg == nil {
		reflexCfg = cfg.ReflexLoopConfig
	}
	reflex, err := NewReflexLoopWithConfig(*reflexCfg)
	if err != nil {
		return nil, err
	}

	swarmCfg := cfg.SwarmDispatchConfig
	if swarmCfg == nil {
		swarmCfg = cfg.SwarmDispatcherConfig
	}
	if swarmCfg == nil {
		defaultCfg := DefaultSwarmDispatchConfig()
		swarmCfg = &defaultCfg
	}
	dispatcher, err := NewSwarmDispatcher(arbiter, *swarmCfg)
	if err != nil {
		return nil, err
	}

	pulseListener, decayTracker := initLivenessSubsystem(cfg, reflex)

	return &AmbientEngine{
		config:                cfg,
		state:                 EngineStateStopped,
		statePath:             cfg.StatePath,
		intentStream:          stream,
		epochSynchronizer:     epochSyncer,
		backpressureRegulator: regulator,
		faultIsolator:         isolator,
		consensusArbiter:      arbiter,
		reflexLoop:            reflex,
		swarmDispatcher:       dispatcher,
		livenessListener:      pulseListener,
		decayTracker:          decayTracker,
	}, nil
}

func resolveLivenessSocketPath(cfg AmbientEngineConfig) string {
	if cfg.LivenessSocketPath != "" {
		return cfg.LivenessSocketPath
	}
	if cfg.ProjectRoot != "" {
		return paths.LivenessSocketPath(cfg.ProjectRoot)
	}
	if cfg.StatePath != "" {
		return filepath.Join(cfg.StatePath, paths.LivenessSocketFile)
	}
	return paths.LivenessSocketPath("")
}

func initLivenessSubsystem(cfg AmbientEngineConfig, reflex *ReflexLoop) (*liveness.PulseListener, *liveness.DecayTracker) {
	if !cfg.EnableLiveness && cfg.LivenessSocketPath == "" {
		return nil, nil
	}

	tracker := liveness.NewDecayTracker(
		func(rec *liveness.HolonRecord) {
			handleIschemiaReflex(rec, reflex)
		},
		func(rec *liveness.HolonRecord) {
			// Reperfusion recovery
		},
	)

	socketPath := resolveLivenessSocketPath(cfg)
	listener := liveness.NewPulseListener(socketPath, tracker)
	return listener, tracker
}

func handleIschemiaReflex(rec *liveness.HolonRecord, reflex *ReflexLoop) {
	if reflex == nil || rec == nil || rec.LastPacket == nil {
		return
	}
	act := ReflexAction{
		ID:          fmt.Sprintf("ischemia-%s-%d", rec.LastPacket.HolonID, time.Now().UnixNano()),
		Type:        ReflexActionRestartHolon,
		Target:      rec.LastPacket.HolonID,
		TargetHolon: rec.LastPacket.HolonID,
		Reason:      fmt.Sprintf("holon %s pulse deadline breached (ischemic)", rec.LastPacket.HolonID),
		Payload: map[string]any{
			"holon_id": rec.LastPacket.HolonID,
			"pid":      rec.LastPacket.PID,
			"cadence":  rec.LastPacket.CadenceMS,
		},
		Timestamp: time.Now(),
	}
	if trigErr := reflex.TriggerAction(context.Background(), act); trigErr != nil {
		// Log or record action trigger error
	}
}

// DefaultAmbientEngine initializes an AmbientEngine with default production configuration.
func DefaultAmbientEngine() (*AmbientEngine, error) {
	return NewAmbientEngine(DefaultAmbientEngineConfig())
}

// State returns the current lifecycle state of the engine. Returns EngineStateStopped if receiver is nil.
func (e *AmbientEngine) State() EngineState {
	if e == nil {
		return EngineStateStopped
	}
	e.mu.RLock()
	s := e.state
	e.mu.RUnlock()
	return s
}

// Config returns a copy of the engine's configuration. Returns empty AmbientEngineConfig if receiver is nil.
func (e *AmbientEngine) Config() AmbientEngineConfig {
	if e == nil {
		return AmbientEngineConfig{}
	}
	e.mu.RLock()
	cfg := e.config
	e.mu.RUnlock()
	return cfg
}

// IsRunning returns true if the engine is currently in the Running state. Returns false if receiver is nil.
func (e *AmbientEngine) IsRunning() bool {
	if e == nil {
		return false
	}
	e.mu.RLock()
	running := e.state == EngineStateRunning
	e.mu.RUnlock()
	return running
}

// IntentStream returns the sub-component AmbientIntentStream. Returns nil if receiver is nil.
func (e *AmbientEngine) IntentStream() *AmbientIntentStream {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	s := e.intentStream
	e.mu.RUnlock()
	return s
}

// EpochSynchronizer returns the sub-component EpochSynchronizer. Returns nil if receiver is nil.
func (e *AmbientEngine) EpochSynchronizer() *EpochSynchronizer {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	s := e.epochSynchronizer
	e.mu.RUnlock()
	return s
}

// BackpressureRegulator returns the sub-component BackpressureRegulator. Returns nil if receiver is nil.
func (e *AmbientEngine) BackpressureRegulator() *BackpressureRegulator {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	s := e.backpressureRegulator
	e.mu.RUnlock()
	return s
}

// FaultIsolator returns the sub-component FaultIsolator. Returns nil if receiver is nil.
func (e *AmbientEngine) FaultIsolator() *FaultIsolator {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	s := e.faultIsolator
	e.mu.RUnlock()
	return s
}

func withEngineRLock[T any](e *AmbientEngine, extractor func(*AmbientEngine) T) T {
	var zero T
	if e == nil {
		return zero
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return extractor(e)
}

// ConsensusArbiter returns the sub-component ConsensusArbiter. Returns nil if receiver is nil.
func (e *AmbientEngine) ConsensusArbiter() *ConsensusArbiter {
	return withEngineRLock(e, func(eng *AmbientEngine) *ConsensusArbiter {
		return eng.consensusArbiter
	})
}

// ReflexLoop returns the sub-component ReflexLoop. Returns nil if receiver is nil.
func (e *AmbientEngine) ReflexLoop() *ReflexLoop {
	return withEngineRLock(e, func(eng *AmbientEngine) *ReflexLoop {
		return eng.reflexLoop
	})
}

// SwarmDispatcher returns the sub-component SwarmDispatcher. Returns nil if receiver is nil.
func (e *AmbientEngine) SwarmDispatcher() *SwarmDispatcher {
	return withEngineRLock(e, func(eng *AmbientEngine) *SwarmDispatcher {
		return eng.swarmDispatcher
	})
}

// BacklogObserver returns the bound BacklogObserver. Returns nil if receiver is nil.
func (e *AmbientEngine) BacklogObserver() *BacklogObserver {
	return e.SwarmDispatcher()
}

// LivenessListener returns the pulse listener hosted by the engine, or nil if not enabled.
func (e *AmbientEngine) LivenessListener() *liveness.PulseListener {
	return withEngineRLock(e, func(eng *AmbientEngine) *liveness.PulseListener {
		return eng.livenessListener
	})
}

// DecayTracker returns the underlying decay tracker, or nil if liveness is not enabled.
func (e *AmbientEngine) DecayTracker() *liveness.DecayTracker {
	return withEngineRLock(e, func(eng *AmbientEngine) *liveness.DecayTracker {
		return eng.decayTracker
	})
}

// VitalitySnapshot returns a snapshot of active holon vitalities from the decay tracker.
func (e *AmbientEngine) VitalitySnapshot() []liveness.HolonVitality {
	dt := e.DecayTracker()
	if dt == nil {
		return nil
	}
	return dt.VitalitySnapshot()
}

// VitalityMap returns active holon vitalities keyed by holon ID.
func (e *AmbientEngine) VitalityMap() map[string]liveness.HolonVitality {
	vitals := e.VitalitySnapshot()
	if vitals == nil {
		return nil
	}
	res := make(map[string]liveness.HolonVitality, len(vitals))
	for _, v := range vitals {
		res[v.HolonID] = v
	}
	return res
}

// RemoveHolon purges a terminated or decommissioned holon from the liveness decay tracker.
func (e *AmbientEngine) RemoveHolon(holonID string) bool {
	dt := e.DecayTracker()
	if dt == nil {
		return false
	}
	dt.Remove(holonID)
	return true
}

// HolonVitality returns point-in-time vitality status for a specific holon, or false if not tracked.
func (e *AmbientEngine) HolonVitality(holonID string) (liveness.HolonVitality, bool) {
	vitMap := e.VitalityMap()
	if vitMap == nil {
		return liveness.HolonVitality{}, false
	}
	v, ok := vitMap[holonID]
	return v, ok
}

// TaskLeaseChannel returns the receive-only channel for published swarm task leases. Returns nil if receiver or observer is nil.
func (e *AmbientEngine) TaskLeaseChannel() <-chan SwarmTaskLease {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.swarmDispatcher != nil {
		return e.swarmDispatcher.LeaseChannel()
	}
	return nil
}

func withSwarmDispatcher(e *AmbientEngine, action func(*SwarmDispatcher) error) error {
	if e == nil {
		return ErrNilEngine
	}
	disp := e.SwarmDispatcher()
	if disp == nil {
		return ErrNilSwarmDispatcher
	}
	return action(disp)
}

// DispatchWorkClaim delegates dynamic work-claiming to the engine's SwarmDispatcher.
func (e *AmbientEngine) DispatchWorkClaim(seatID, bliID string) (*SwarmTaskLease, error) {
	if e == nil {
		return nil, ErrNilEngine
	}
	disp := e.SwarmDispatcher()
	if disp == nil {
		return nil, ErrNilSwarmDispatcher
	}
	return disp.DispatchWorkClaim(seatID, bliID)
}

// RegisterPeerSeat registers a peer seat in the engine's SwarmDispatcher.
func (e *AmbientEngine) RegisterPeerSeat(seat PeerSeat) error {
	return withSwarmDispatcher(e, func(disp *SwarmDispatcher) error {
		return disp.RegisterSeat(seat)
	})
}

// RecordSeatHeartbeat updates a peer seat heartbeat in the engine's SwarmDispatcher.
func (e *AmbientEngine) RecordSeatHeartbeat(seatID string) error {
	return withSwarmDispatcher(e, func(disp *SwarmDispatcher) error {
		return disp.RecordHeartbeat(seatID)
	})
}

// EnqueuePlannedBLI adds a planned backlog item to the engine's SwarmDispatcher.
func (e *AmbientEngine) EnqueuePlannedBLI(bliID string) error {
	return withSwarmDispatcher(e, func(disp *SwarmDispatcher) error {
		return disp.EnqueuePlannedBLI(bliID)
	})
}

// GetActiveLease returns the active lease for a given task/BLI from the SwarmDispatcher.
func (e *AmbientEngine) GetActiveLease(taskID string) (*SwarmTaskLease, bool) {
	if e == nil {
		return nil, false
	}
	disp := e.SwarmDispatcher()
	if disp == nil {
		return nil, false
	}
	return disp.GetTaskLease(taskID)
}

// CompleteTask finalizes work-claiming and commits consensus state.
func (e *AmbientEngine) CompleteTask(taskID, seatID string) error {
	return withSwarmDispatcher(e, func(disp *SwarmDispatcher) error {
		return disp.CompleteTask(taskID, seatID)
	})
}

// YieldTask relinquishes work-claiming and handles task retry or quarantine.
func (e *AmbientEngine) YieldTask(taskID, seatID, reason string) error {
	return withSwarmDispatcher(e, func(disp *SwarmDispatcher) error {
		return disp.YieldTask(taskID, seatID, reason)
	})
}

// TaskEvents returns the receive-only channel for published swarm task lifecycle events.
func (e *AmbientEngine) TaskEvents() <-chan SwarmTaskEvent {
	if e == nil {
		return nil
	}
	disp := e.SwarmDispatcher()
	if disp == nil {
		return nil
	}
	return disp.EventStream()
}

// EnqueueTask adds a task to the engine's unassigned queue.
func (e *AmbientEngine) EnqueueTask(taskID string) bool {
	if e == nil {
		return false
	}
	disp := e.SwarmDispatcher()
	if disp == nil {
		return false
	}
	return disp.EnqueueTask(taskID)
}

// Transition attempts to transition the engine to a new lifecycle state following valid FSM paths.
func (e *AmbientEngine) Transition(to EngineState) error {
	if e == nil {
		return ErrNilEngine
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	switch to {
	case EngineStateStarting:
		if e.state == EngineStateRunning {
			return ErrEngineAlreadyRunning
		}
		if e.state != EngineStateStopped {
			return errfmt.Errorf("cannot transition from %s to %s", e.state, to)
		}
	case EngineStateRunning:
		if e.state != EngineStateStarting && e.state != EngineStateDegraded {
			return errfmt.Errorf("cannot transition from %s to %s", e.state, to)
		}
	case EngineStateDegraded:
		if e.state != EngineStateRunning {
			return errfmt.Errorf("cannot transition from %s to %s", e.state, to)
		}
	case EngineStateStopping:
		if e.state == EngineStateStopped {
			return ErrEngineStopped
		}
		if e.state != EngineStateRunning && e.state != EngineStateDegraded && e.state != EngineStateStarting {
			return errfmt.Errorf("cannot transition from %s to %s", e.state, to)
		}
	case EngineStateStopped:
		if e.state != EngineStateStopping && e.state != EngineStateStarting {
			return errfmt.Errorf("cannot transition from %s to %s", e.state, to)
		}
	default:
		return errfmt.Errorf("unknown engine state: %s", to)
	}

	e.state = to
	return nil
}

// TransitionState is an alias for Transition.
func (e *AmbientEngine) TransitionState(to EngineState) error {
	return e.Transition(to)
}

// StatePath returns the configured state path for telemetry snapshots.
func (e *AmbientEngine) StatePath() string {
	if e == nil {
		return ""
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.statePath
}

// SetStatePath updates the state path for telemetry snapshots.
func (e *AmbientEngine) SetStatePath(p string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.statePath = p
}

// SaveTelemetryReport persists the engine's current telemetry report to the specified paths or default statePath.
func (e *AmbientEngine) SaveTelemetryReport(customPaths ...string) error {
	if e == nil {
		return ErrNilEngine
	}
	report := e.TelemetryReport()
	if report == nil {
		return ErrNilTelemetryReport
	}

	var targets []string
	for _, p := range customPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		e.mu.RLock()
		sp := e.statePath
		e.mu.RUnlock()
		if sp != "" {
			targets = append(targets, sp)
		}
	}
	if len(targets) == 0 {
		return ErrStatePathRequired
	}

	var errs []error
	for _, target := range targets {
		if err := SaveTelemetryReport(target, report); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
