package ambient

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
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
	ErrEngineAlreadyRunning   = errfmt.Errorf("ambient engine is already running")
	ErrEngineDegraded         = errfmt.Errorf("ambient engine is degraded")
	ErrInvalidTickInterval    = errfmt.Errorf("tick interval must be strictly positive")
	ErrInvalidGracefulTimeout = errfmt.Errorf("graceful stop timeout must be strictly positive")
	ErrNilSubConfig           = errfmt.Errorf("sub-component configuration cannot be nil")
)

// AmbientEngineConfig defines configuration parameters and sub-component configs for AmbientEngine.
type AmbientEngineConfig struct {
	TickInterval        time.Duration            `json:"tick_interval"`
	GracefulStopTimeout time.Duration            `json:"graceful_stop_timeout"`
	IntentStreamConfig  *IntentStreamConfig      `json:"intent_stream_config"`
	EpochConfig         *EpochSynchronizerConfig `json:"epoch_config"`
	BackpressureConfig  *BackpressureConfig      `json:"backpressure_config"`
	FaultIsolatorConfig *FaultIsolatorConfig     `json:"fault_isolator_config"`
}

// DefaultAmbientEngineConfig returns production defaults for AmbientEngine.
func DefaultAmbientEngineConfig() AmbientEngineConfig {
	intentCfg := DefaultAmbientIntentStreamConfig()
	epochCfg := DefaultEpochSynchronizerConfig()
	backpressureCfg := DefaultBackpressureConfig()
	faultCfg := DefaultFaultIsolatorConfig()

	return AmbientEngineConfig{
		TickInterval:        DefaultTickInterval,
		GracefulStopTimeout: DefaultGracefulStopTimeout,
		IntentStreamConfig:  &intentCfg,
		EpochConfig:         &epochCfg,
		BackpressureConfig:  &backpressureCfg,
		FaultIsolatorConfig: &faultCfg,
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
	return nil
}

// AmbientEngine coordinates the autonomous runtime microkernel loops, orchestrating
// the AmbientIntentStream, EpochSynchronizer, BackpressureRegulator, and FaultIsolator.
type AmbientEngine struct {
	mu                    sync.RWMutex
	config                AmbientEngineConfig
	state                 EngineState
	intentStream          *AmbientIntentStream
	epochSynchronizer     *EpochSynchronizer
	backpressureRegulator *BackpressureRegulator
	faultIsolator         *FaultIsolator

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

	return &AmbientEngine{
		config:                cfg,
		state:                 EngineStateStopped,
		intentStream:          stream,
		epochSynchronizer:     epochSyncer,
		backpressureRegulator: regulator,
		faultIsolator:         isolator,
	}, nil
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
