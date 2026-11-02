package ambient

import (
	"errors"
	"testing"
	"time"
)

func TestAmbientEngine_StaticFloorInvariants(t *testing.T) {
	t.Run("DefaultConfiguration", func(t *testing.T) {
		cfg := DefaultAmbientEngineConfig()
		if cfg.TickInterval != 100*time.Millisecond {
			t.Fatalf("expected DefaultTickInterval 100ms, got %v", cfg.TickInterval)
		}
		if cfg.GracefulStopTimeout != 5*time.Second {
			t.Fatalf("expected DefaultGracefulStopTimeout 5s, got %v", cfg.GracefulStopTimeout)
		}
		if cfg.IntentStreamConfig == nil {
			t.Fatal("expected IntentStreamConfig not nil")
		}
		if cfg.EpochConfig == nil {
			t.Fatal("expected EpochConfig not nil")
		}
		if cfg.BackpressureConfig == nil {
			t.Fatal("expected BackpressureConfig not nil")
		}
		if cfg.FaultIsolatorConfig == nil {
			t.Fatal("expected FaultIsolatorConfig not nil")
		}
		if cfg.ConsensusConfig == nil || cfg.ConsensusArbiterConfig == nil {
			t.Fatal("expected ConsensusConfig not nil")
		}
		if cfg.ReflexConfig == nil || cfg.ReflexLoopConfig == nil {
			t.Fatal("expected ReflexConfig not nil")
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected DefaultAmbientEngineConfig to be valid, got: %v", err)
		}

		engine, err := DefaultAmbientEngine()
		if err != nil {
			t.Fatalf("expected DefaultAmbientEngine to succeed, got: %v", err)
		}
		if engine == nil {
			t.Fatal("expected engine not nil")
		}
		if engine.ConsensusArbiter() == nil {
			t.Fatal("expected ConsensusArbiter not nil")
		}
		if engine.ReflexLoop() == nil {
			t.Fatal("expected ReflexLoop not nil")
		}
		if engine.State() != EngineStateStopped {
			t.Fatalf("expected initial state %s, got %s", EngineStateStopped, engine.State())
		}
		if engine.IsRunning() {
			t.Fatal("expected engine not running initially")
		}
		if engine.IntentStream() == nil {
			t.Fatal("expected subcomponent IntentStream not nil")
		}
		if engine.EpochSynchronizer() == nil {
			t.Fatal("expected subcomponent EpochSynchronizer not nil")
		}
		if engine.BackpressureRegulator() == nil {
			t.Fatal("expected subcomponent BackpressureRegulator not nil")
		}
		if engine.FaultIsolator() == nil {
			t.Fatal("expected subcomponent FaultIsolator not nil")
		}
	})

	t.Run("ConfigValidationFailures", func(t *testing.T) {
		cases := []struct {
			name        string
			mutate      func(c *AmbientEngineConfig)
			expectedErr error
		}{
			{
				name:        "zero tick interval",
				mutate:      func(c *AmbientEngineConfig) { c.TickInterval = 0 },
				expectedErr: ErrInvalidTickInterval,
			},
			{
				name:        "negative tick interval",
				mutate:      func(c *AmbientEngineConfig) { c.TickInterval = -10 * time.Millisecond },
				expectedErr: ErrInvalidTickInterval,
			},
			{
				name:        "zero graceful stop timeout",
				mutate:      func(c *AmbientEngineConfig) { c.GracefulStopTimeout = 0 },
				expectedErr: ErrInvalidGracefulTimeout,
			},
			{
				name:        "negative graceful stop timeout",
				mutate:      func(c *AmbientEngineConfig) { c.GracefulStopTimeout = -1 * time.Second },
				expectedErr: ErrInvalidGracefulTimeout,
			},
			{
				name:        "nil intent stream config",
				mutate:      func(c *AmbientEngineConfig) { c.IntentStreamConfig = nil },
				expectedErr: ErrNilSubConfig,
			},
			{
				name:        "nil epoch config",
				mutate:      func(c *AmbientEngineConfig) { c.EpochConfig = nil },
				expectedErr: ErrNilSubConfig,
			},
			{
				name:        "nil backpressure config",
				mutate:      func(c *AmbientEngineConfig) { c.BackpressureConfig = nil },
				expectedErr: ErrNilSubConfig,
			},
			{
				name:        "nil fault isolator config",
				mutate:      func(c *AmbientEngineConfig) { c.FaultIsolatorConfig = nil },
				expectedErr: ErrNilSubConfig,
			},
			{
				name: "nil consensus config",
				mutate: func(c *AmbientEngineConfig) {
					c.ConsensusConfig = nil
					c.ConsensusArbiterConfig = nil
				},
				expectedErr: ErrNilSubConfig,
			},
			{
				name: "nil reflex loop config",
				mutate: func(c *AmbientEngineConfig) {
					c.ReflexConfig = nil
					c.ReflexLoopConfig = nil
				},
				expectedErr: ErrNilSubConfig,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := DefaultAmbientEngineConfig()
				tc.mutate(&cfg)
				err := cfg.Validate()
				if !errors.Is(err, tc.expectedErr) {
					t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
				}
				_, err = NewAmbientEngine(cfg)
				if !errors.Is(err, tc.expectedErr) {
					t.Fatalf("expected NewAmbientEngine error %v, got %v", tc.expectedErr, err)
				}
			})
		}
	})

	t.Run("StateStrings", func(t *testing.T) {
		states := map[EngineState]string{
			EngineStateStopped:  "Stopped",
			EngineStateStarting: "Starting",
			EngineStateRunning:  "Running",
			EngineStateDegraded: "Degraded",
			EngineStateStopping: "Stopping",
		}

		for state, expected := range states {
			if state.String() != expected {
				t.Fatalf("expected %s.String() to be %q, got %q", expected, expected, state.String())
			}
		}
	})

	t.Run("NilSafety", func(t *testing.T) {
		var nilEngine *AmbientEngine

		if state := nilEngine.State(); state != EngineStateStopped {
			t.Fatalf("expected nil State() %s, got %s", EngineStateStopped, state)
		}
		if cfg := nilEngine.Config(); cfg != (AmbientEngineConfig{}) {
			t.Fatalf("expected empty Config() on nil engine, got %+v", cfg)
		}
		if running := nilEngine.IsRunning(); running {
			t.Fatal("expected nil IsRunning() to be false")
		}
		if stream := nilEngine.IntentStream(); stream != nil {
			t.Fatal("expected nil IntentStream() on nil engine")
		}
		if syncer := nilEngine.EpochSynchronizer(); syncer != nil {
			t.Fatal("expected nil EpochSynchronizer() on nil engine")
		}
		if reg := nilEngine.BackpressureRegulator(); reg != nil {
			t.Fatal("expected nil BackpressureRegulator() on nil engine")
		}
		if iso := nilEngine.FaultIsolator(); iso != nil {
			t.Fatal("expected nil FaultIsolator() on nil engine")
		}
		if arb := nilEngine.ConsensusArbiter(); arb != nil {
			t.Fatal("expected nil ConsensusArbiter() on nil engine")
		}
		if ref := nilEngine.ReflexLoop(); ref != nil {
			t.Fatal("expected nil ReflexLoop() on nil engine")
		}
		if err := nilEngine.Transition(EngineStateRunning); !errors.Is(err, ErrNilEngine) {
			t.Fatalf("expected ErrNilEngine on Transition(), got %v", err)
		}
		if err := nilEngine.TransitionState(EngineStateRunning); !errors.Is(err, ErrNilEngine) {
			t.Fatalf("expected ErrNilEngine on TransitionState(), got %v", err)
		}
	})

	t.Run("ZeroAllocationsOnStateReads", func(t *testing.T) {
		engine, err := DefaultAmbientEngine()
		if err != nil {
			t.Fatalf("unexpected error creating default engine: %v", err)
		}

		allocs := testing.AllocsPerRun(1000, func() {
			_ = engine.State()
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocations on engine.State(), got %f", allocs)
		}

		allocsRunning := testing.AllocsPerRun(1000, func() {
			_ = engine.IsRunning()
		})
		if allocsRunning != 0 {
			t.Errorf("expected 0 allocations on engine.IsRunning(), got %f", allocsRunning)
		}

		var nilEngine *AmbientEngine
		allocsNil := testing.AllocsPerRun(1000, func() {
			_ = nilEngine.State()
		})
		if allocsNil != 0 {
			t.Errorf("expected 0 allocations on nilEngine.State(), got %f", allocsNil)
		}
	})

	t.Run("StateTransitions", func(t *testing.T) {
		engine, err := DefaultAmbientEngine()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Stopped -> Starting
		if err := engine.Transition(EngineStateStarting); err != nil {
			t.Fatalf("expected transition Stopped -> Starting, got %v", err)
		}
		if engine.State() != EngineStateStarting {
			t.Fatalf("expected state %s, got %s", EngineStateStarting, engine.State())
		}

		// Starting -> Running
		if err := engine.Transition(EngineStateRunning); err != nil {
			t.Fatalf("expected transition Starting -> Running, got %v", err)
		}
		if !engine.IsRunning() {
			t.Fatal("expected engine to be running")
		}

		// Running -> Starting should fail with ErrEngineAlreadyRunning
		if err := engine.Transition(EngineStateStarting); !errors.Is(err, ErrEngineAlreadyRunning) {
			t.Fatalf("expected ErrEngineAlreadyRunning, got %v", err)
		}

		// Running -> Degraded
		if err := engine.Transition(EngineStateDegraded); err != nil {
			t.Fatalf("expected transition Running -> Degraded, got %v", err)
		}
		if engine.State() != EngineStateDegraded {
			t.Fatalf("expected state %s, got %s", EngineStateDegraded, engine.State())
		}
		if engine.IsRunning() {
			t.Fatal("expected engine not running while degraded")
		}

		// Degraded -> Running (recovery)
		if err := engine.Transition(EngineStateRunning); err != nil {
			t.Fatalf("expected transition Degraded -> Running, got %v", err)
		}
		if !engine.IsRunning() {
			t.Fatal("expected engine to be running after recovery")
		}

		// Running -> Stopping
		if err := engine.Transition(EngineStateStopping); err != nil {
			t.Fatalf("expected transition Running -> Stopping, got %v", err)
		}
		if engine.State() != EngineStateStopping {
			t.Fatalf("expected state %s, got %s", EngineStateStopping, engine.State())
		}

		// Stopping -> Stopped
		if err := engine.Transition(EngineStateStopped); err != nil {
			t.Fatalf("expected transition Stopping -> Stopped, got %v", err)
		}
		if engine.State() != EngineStateStopped {
			t.Fatalf("expected state %s, got %s", EngineStateStopped, engine.State())
		}

		// Stopped -> Stopping should fail with ErrEngineStopped
		if err := engine.Transition(EngineStateStopping); !errors.Is(err, ErrEngineStopped) {
			t.Fatalf("expected ErrEngineStopped, got %v", err)
		}
	})
}
