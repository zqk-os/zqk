package liveness

import (
	"context"
	"net"
	"os"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
)

// Emitter emits connectionless neuro-metabolic datagram pulses to an ambient pulse listener.
type Emitter struct {
	socketPath string
	mu         sync.Mutex
	conn       net.Conn
}

// NewEmitter creates a pulse emitter targeting the specified Unix domain socket path.
func NewEmitter(socketPath string) *Emitter {
	return &Emitter{
		socketPath: socketPath,
	}
}

// NewEmitterForProject creates an emitter using the canonical project liveness socket path.
func NewEmitterForProject(projectRoot string) *Emitter {
	return NewEmitter(paths.LivenessSocketPath(projectRoot))
}

// Emit sends a single PulsePacket across the datagram bus.
// It is guaranteed to be non-blocking: write operations carry an immediate deadline.
// If the socket is missing or unmonitored, Emit returns an error without stalling the caller.
func (e *Emitter) Emit(p *PulsePacket) error {
	if p == nil {
		return errfmt.Errorf("cannot emit nil pulse packet")
	}
	payload, err := p.Encode()
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	raddr := &net.UnixAddr{Name: e.socketPath, Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", nil, raddr)
	if err != nil {
		return errfmt.Errorf("dial liveness pulse bus: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			// Best-effort close on ephemeral datagram socket.
		}
	}()

	if dlErr := conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond)); dlErr != nil {
		return errfmt.Errorf("set pulse deadline: %w", dlErr)
	}
	n, err := conn.Write(payload)
	if err != nil {
		return errfmt.Errorf("write pulse datagram: %w", err)
	}
	if n != len(payload) {
		return errfmt.Errorf("partial pulse write: wrote %d of %d bytes", n, len(payload))
	}
	return nil
}

// EmitQuick creates a PulsePacket for the current process and emits it in a single shot.
func EmitQuick(socketPath string, holonID string, phase string, cadenceMS int) error {
	p := NewPulsePacket(holonID, os.Getpid(), cadenceMS, phase)
	e := NewEmitter(socketPath)
	return e.Emit(p)
}

// EmitQuickForProject creates a PulsePacket for the current process using projectRoot resolution.
func EmitQuickForProject(projectRoot string, holonID string, phase string, cadenceMS int) error {
	return EmitQuick(paths.LivenessSocketPath(projectRoot), holonID, phase, cadenceMS)
}

// PulseTicker runs an autonomous background heartbeat loop that continuously reports
// liveness pulses at a specified cadence until stopped or its context is cancelled.
type PulseTicker struct {
	emitter   *Emitter
	holonID   string
	cadenceMS int
	mu        sync.RWMutex
	phase     string
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// StartPulseTicker starts an autonomous pulse ticker loop emitting heartbeats to socketPath.
func StartPulseTicker(ctx context.Context, socketPath string, holonID string, phase string, cadenceMS int) *PulseTicker {
	if cadenceMS <= 0 {
		cadenceMS = DefaultCadenceMS
	}
	pt := &PulseTicker{
		emitter:   NewEmitter(socketPath),
		holonID:   holonID,
		cadenceMS: cadenceMS,
		phase:     phase,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}

	goroutinelabels.NewGoroutine("pulse_ticker", "autonomous liveness pulse loop").StartSimple(func() { pt.run(ctx) })
	return pt
}

// StartPulseTickerForProject starts a pulse ticker using the project's canonical socket path.
func StartPulseTickerForProject(ctx context.Context, projectRoot string, holonID string, phase string, cadenceMS int) *PulseTicker {
	return StartPulseTicker(ctx, paths.LivenessSocketPath(projectRoot), holonID, phase, cadenceMS)
}

// UpdatePhase dynamically updates the execution phase reported on subsequent pulses.
func (pt *PulseTicker) UpdatePhase(phase string) {
	pt.mu.Lock()
	pt.phase = phase
	pt.mu.Unlock()
}

// Stop cleanly terminates the pulse ticker loop and waits for the goroutine to finish.
func (pt *PulseTicker) Stop() {
	select {
	case <-pt.stopCh:
		// already stopped
		return
	default:
		close(pt.stopCh)
		<-pt.doneCh
	}
}

func (pt *PulseTicker) run(ctx context.Context) {
	defer close(pt.doneCh)
	interval := time.Duration(pt.cadenceMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Emit initial pulse immediately upon startup
	pt.emitCurrent()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pt.stopCh:
			return
		case <-ticker.C:
			pt.emitCurrent()
		}
	}
}

func (pt *PulseTicker) emitCurrent() {
	pt.mu.RLock()
	curPhase := pt.phase
	pt.mu.RUnlock()

	p := NewPulsePacket(pt.holonID, os.Getpid(), pt.cadenceMS, curPhase)
	// Non-blocking fire-and-forget: do not block or crash if pulse bus is temporarily busy
	if err := pt.emitter.Emit(p); err != nil {
		return
	}
}
