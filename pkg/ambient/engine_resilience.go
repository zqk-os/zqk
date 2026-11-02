// Traceability: REQ-1793613697590604000-1c258cb3, CRIT-1793613743965493000-e1a3610a, BLI-1793614300774076000-bb32884e
package ambient

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Sentinel errors for engine resilience, circuit breaker tripping, and backpressure admission.
var (
	ErrCircuitTripped = errfmt.Errorf("ambient circuit breaker open: requests rejected")
	ErrEventRejected  = errfmt.Errorf("ambient event rejected by admission control")
)

// SafeExecute executes fn within a panic recovery boundary, isolating any panic.
func (e *AmbientEngine) SafeExecute(fn func()) (recovered any) {
	defer func() {
		if r := recover(); r != nil {
			recovered = r
			if e != nil {
				_ = e.Transition(EngineStateDegraded)
			}
		}
	}()
	if fn != nil {
		fn()
	}
	return nil
}

// Recover attempts to transition the engine back to EngineStateRunning if degraded.
func (e *AmbientEngine) Recover() error {
	if e == nil {
		return ErrNilEngine
	}
	if e.State() == EngineStateDegraded {
		return e.Transition(EngineStateRunning)
	}
	return nil
}

// SubmitIntent validates, evaluates admission and circuit protection, and pushes an intent event.
func (e *AmbientEngine) SubmitIntent(event IntentEvent) error {
	if e == nil {
		return ErrNilEngine
	}
	if e.State() != EngineStateRunning {
		return ErrEngineStopped
	}
	if err := event.Validate(); err != nil {
		return err
	}

	e.mu.RLock()
	iso := e.faultIsolator
	reg := e.backpressureRegulator
	stream := e.intentStream
	e.mu.RUnlock()

	if iso != nil {
		if iso.State() == StateOpen {
			return ErrCircuitTripped
		}
		if err := iso.Allow(); err != nil {
			return ErrCircuitTripped
		}
	}

	if reg != nil {
		if !reg.Admit(1) {
			if iso != nil {
				_ = iso.RecordFailure()
			}
			return ErrEventRejected
		}
	}

	if stream != nil {
		if err := stream.Push(event); err != nil {
			if iso != nil {
				_ = iso.RecordFailure()
			}
			return err
		}
	}

	if iso != nil {
		_ = iso.RecordSuccess()
	}

	return nil
}
