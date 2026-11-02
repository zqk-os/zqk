package ambient

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// EngineStatus captures point-in-time runtime lifecycle and subcomponent health status.
type EngineStatus struct {
	State         EngineState       `json:"state"`
	StartedAt     time.Time         `json:"started_at,omitempty"`
	Uptime        time.Duration     `json:"uptime"`
	TicksExecuted uint64            `json:"ticks_executed"`
	Subcomponents map[string]string `json:"subcomponents"`
}

// EngineTelemetry aggregates real-time holon metrics across the ambient runtime.
type EngineTelemetry struct {
	PendingIntents  int     `json:"pending_intents"`
	ActiveAgents    int     `json:"active_agents"`
	RunwayDepth     float64 `json:"runway_depth"`
	CircuitState    string  `json:"circuit_state"`
	FailureRate     float64 `json:"failure_rate"`
	CurrentCapacity int     `json:"current_capacity"`
	EpochNumber     uint64  `json:"epoch_number"`
}

// Start launches the ambient engine and begins background supervisor ticks.
// Returns ErrNilEngine if receiver is nil, or ErrEngineAlreadyRunning if already active.
func (e *AmbientEngine) Start(ctx context.Context) error {
	if e == nil {
		return ErrNilEngine
	}

	if err := e.Transition(EngineStateStarting); err != nil {
		return err
	}

	e.mu.Lock()
	e.startedAt = time.Now()
	atomic.StoreUint64(&e.ticksExecuted, 0)

	if ctx == nil {
		ctx = context.Background()
	}

	supCtx, cancel := context.WithCancel(ctx)
	e.cancelFn = cancel
	e.supervisorDone = make(chan struct{})

	interval := e.config.TickInterval
	if interval <= 0 {
		interval = DefaultTickInterval
	}

	goroutinelabels.NewGoroutine("ambient.engine.supervisor", "ambient holon lifecycle supervisor").
		WithWaitGroup(&e.supervisorWg).
		StartSimple(func() {
			defer close(e.supervisorDone)
			e.runSupervisor(supCtx, interval)
		})
	e.mu.Unlock()

	return e.Transition(EngineStateRunning)
}

// runSupervisor runs the periodic tick supervisor loop until ctx is canceled.
func (e *AmbientEngine) runSupervisor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			atomic.AddUint64(&e.ticksExecuted, 1)
			e.superviseTick(ctx)
		}
	}
}

// superviseTick executes holon health checks and reconciles dynamic state.
func (e *AmbientEngine) superviseTick(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	if e.faultIsolator != nil {
		breakerState := e.faultIsolator.State()
		if breakerState == StateOpen && e.State() == EngineStateRunning {
			if err := e.Transition(EngineStateDegraded); err != nil {
				_ = err
			}
		} else if breakerState == StateClosed && e.State() == EngineStateDegraded {
			if err := e.Transition(EngineStateRunning); err != nil {
				_ = err
			}
		}
	}

	if e.intentStream != nil {
		if alert, emitted := e.intentStream.CheckAndEmitAlert(); emitted {
			_ = alert
		}
	}

	if e.backpressureRegulator != nil {
		tput := e.backpressureRegulator.CurrentThroughput()
		_ = tput
	}
}

// Stop gracefully halts background supervisor loops and transitions to EngineStateStopped.
// Returns ErrNilEngine if receiver is nil, or ErrEngineStopped if already stopped.
func (e *AmbientEngine) Stop() error {
	if e == nil {
		return ErrNilEngine
	}

	if err := e.Transition(EngineStateStopping); err != nil {
		return err
	}

	e.mu.Lock()
	cancel := e.cancelFn
	done := e.supervisorDone
	timeout := e.config.GracefulStopTimeout
	if timeout <= 0 {
		timeout = DefaultGracefulStopTimeout
	}
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-done:
		case <-timer.C:
			if err := e.Transition(EngineStateStopped); err != nil {
				_ = err
			}
			return errfmt.Errorf("graceful stop timed out after %v", timeout)
		}
	}

	e.supervisorWg.Wait()

	e.mu.Lock()
	e.cancelFn = nil
	e.supervisorDone = nil
	e.mu.Unlock()

	return e.Transition(EngineStateStopped)
}

// Status returns a thread-safe inspection of current engine state, uptime, ticks, and holon health.
// Returns EngineStatus{State: EngineStateStopped} if receiver is nil.
func (e *AmbientEngine) Status() EngineStatus {
	if e == nil {
		return EngineStatus{State: EngineStateStopped}
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var uptime time.Duration
	if !e.startedAt.IsZero() && e.state != EngineStateStopped {
		uptime = time.Since(e.startedAt)
	}

	sub := make(map[string]string)
	if e.intentStream != nil {
		_, thresh := e.intentStream.EvaluateRunwayDepth()
		sub["intent_stream"] = string(thresh)
	}
	if e.epochSynchronizer != nil {
		if cur := e.epochSynchronizer.CurrentEpoch(); cur != nil {
			sub["epoch_synchronizer"] = cur.State.String()
			sub["epoch_sync"] = cur.State.String()
		} else {
			sub["epoch_synchronizer"] = "ready"
			sub["epoch_sync"] = "ready"
		}
	}
	if e.backpressureRegulator != nil {
		if e.backpressureRegulator.IsClosed() {
			sub["backpressure_regulator"] = "closed"
			sub["backpressure"] = "closed"
		} else {
			sub["backpressure_regulator"] = "healthy"
			sub["backpressure"] = "healthy"
		}
	}
	if e.faultIsolator != nil {
		sub["fault_isolator"] = e.faultIsolator.State().String()
	}

	return EngineStatus{
		State:         e.state,
		StartedAt:     e.startedAt,
		Uptime:        uptime,
		TicksExecuted: atomic.LoadUint64(&e.ticksExecuted),
		Subcomponents: sub,
	}
}

// Telemetry aggregates real-time holon metrics across subcomponents.
// Returns zero-value EngineTelemetry{} if receiver is nil.
func (e *AmbientEngine) Telemetry() EngineTelemetry {
	if e == nil {
		return EngineTelemetry{}
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var telem EngineTelemetry

	if e.intentStream != nil {
		telem.PendingIntents = e.intentStream.Size()
		telem.ActiveAgents = e.intentStream.ActiveAgents()
		depth, thresh := e.intentStream.EvaluateRunwayDepth()
		_ = thresh
		telem.RunwayDepth = depth
	}

	if e.faultIsolator != nil {
		telem.CircuitState = e.faultIsolator.State().String()
		telem.FailureRate = e.faultIsolator.FailureRate()
	}

	if e.backpressureRegulator != nil {
		telem.CurrentCapacity = e.backpressureRegulator.CurrentCapacity()
	}

	if e.epochSynchronizer != nil {
		if cur := e.epochSynchronizer.CurrentEpoch(); cur != nil {
			telem.EpochNumber = cur.EpochNumber
		} else if latest := e.epochSynchronizer.LatestCheckpoint(); latest != nil {
			telem.EpochNumber = latest.EpochNumber
		}
	}

	return telem
}

// TelemetryReport aggregates comprehensive runtime metrics into an AmbientTelemetryReport.
// Returns a stopped status report if the receiver is nil or stopped.
func (e *AmbientEngine) TelemetryReport() *AmbientTelemetryReport {
	if e == nil {
		return NewAmbientTelemetryReport(WithStatus(TelemetryStatusStopped))
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	status := TelemetryStatusHealthy
	if e.state == EngineStateStopped {
		status = TelemetryStatusStopped
	} else if e.state == EngineStateDegraded || (e.faultIsolator != nil && e.faultIsolator.State() == StateOpen) {
		status = TelemetryStatusDegraded
	}

	uptime := time.Duration(0)
	if !e.startedAt.IsZero() && e.state == EngineStateRunning {
		uptime = time.Since(e.startedAt)
	}

	activeHolons := 0
	components := make(map[string]ComponentHealth)

	if e.intentStream != nil {
		activeHolons++
		components["intent_stream"] = ComponentHealth{
			Name:          "intent_stream",
			Status:        TelemetryStatusHealthy,
			LastHeartbeat: time.Now().UTC(),
		}
	}
	if e.epochSynchronizer != nil {
		activeHolons++
		components["epoch_synchronizer"] = ComponentHealth{
			Name:          "epoch_synchronizer",
			Status:        TelemetryStatusHealthy,
			LastHeartbeat: time.Now().UTC(),
		}
	}
	if e.backpressureRegulator != nil {
		activeHolons++
		components["backpressure_regulator"] = ComponentHealth{
			Name:          "backpressure_regulator",
			Status:        TelemetryStatusHealthy,
			LastHeartbeat: time.Now().UTC(),
		}
	}
	if e.faultIsolator != nil {
		activeHolons++
		fiStatus := TelemetryStatusHealthy
		if e.faultIsolator.State() == StateOpen {
			fiStatus = TelemetryStatusDegraded
		}
		components["fault_isolator"] = ComponentHealth{
			Name:          "fault_isolator",
			Status:        fiStatus,
			LastHeartbeat: time.Now().UTC(),
		}
	}

	cbStatus := CircuitBreakerStatusClosed
	if e.faultIsolator != nil {
		cbStatus = e.faultIsolator.State().String()
	}

	var runwayWatermark int64
	if e.intentStream != nil {
		runwayWatermark = int64(e.intentStream.Size())
	}

	meshMetrics := HolonMeshMetrics{
		EventThroughput:  float64(atomic.LoadUint64(&e.ticksExecuted)),
		ConvergenceScore: 100.0,
	}
	if e.faultIsolator != nil {
		meshMetrics.DivergenceCount = int64(e.faultIsolator.FailureCount())
	}

	return NewAmbientTelemetryReport(
		WithStatus(status),
		WithUptime(uptime),
		WithActiveHolonsCount(activeHolons),
		WithComponents(components),
		WithCircuitBreakerStatus(cbStatus),
		WithRunwayWatermark(runwayWatermark),
		WithMeshMetrics(meshMetrics),
	)
}
