package liveness

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// HolonVitality captures the real-time metabolic status of a monitored holon or worker.
type HolonVitality struct {
	HolonID       string            `json:"holon_id"`
	PID           int               `json:"pid"`
	CadenceMS     int               `json:"cadence_ms"`
	Phase         string            `json:"phase"`
	LastSeenAgoMS int64             `json:"last_seen_ago_ms"`
	Ischemic      bool              `json:"ischemic"`
	PulseCount    int64             `json:"pulse_count"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// HolonRecord stores in-memory state and decay deadlines for a holon.
type HolonRecord struct {
	LastPacket *PulsePacket
	ReceivedAt time.Time
	Deadline   time.Time
	PulseCount int64
	Ischemic   bool
}

// IschemiaHandler is invoked when a holon misses its pulse deadline.
type IschemiaHandler func(record *HolonRecord)

// ReperfusionHandler is invoked when an ischemic holon resumes pulsing.
type ReperfusionHandler func(record *HolonRecord)

// DecayTracker manages in-memory decay state and dead-mans-switch detection.
type DecayTracker struct {
	mu            sync.RWMutex
	records       map[string]*HolonRecord
	onIschemia    IschemiaHandler
	onReperfusion ReperfusionHandler
}

// NewDecayTracker initializes a thread-safe decay tracker.
func NewDecayTracker(onIschemia IschemiaHandler, onReperfusion ReperfusionHandler) *DecayTracker {
	return &DecayTracker{
		records:       make(map[string]*HolonRecord),
		onIschemia:    onIschemia,
		onReperfusion: onReperfusion,
	}
}

// RecordPulse ingests an incoming PulsePacket, updating the holon's decay deadline.
func (dt *DecayTracker) RecordPulse(p *PulsePacket) {
	if p == nil || p.HolonID == "" {
		return
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()

	now := time.Now()
	timeoutDur := time.Duration(p.CadenceMS*DefaultIschemicMultiplier) * time.Millisecond
	deadline := now.Add(timeoutDur)

	rec, exists := dt.records[p.HolonID]
	if !exists {
		dt.records[p.HolonID] = &HolonRecord{
			LastPacket: p,
			ReceivedAt: now,
			Deadline:   deadline,
			PulseCount: 1,
			Ischemic:   false,
		}
		return
	}

	wasIschemic := rec.Ischemic
	rec.LastPacket = p
	rec.ReceivedAt = now
	rec.Deadline = deadline
	rec.PulseCount++
	rec.Ischemic = false

	if wasIschemic && dt.onReperfusion != nil {
		goroutinelabels.NewGoroutine("liveness_reperfusion", "handle holon reperfusion").StartSimple(func() { dt.onReperfusion(rec) })
	}
}

// Sweep checks all tracked holons against their deadlines and fires ischemia handlers for breaches.
func (dt *DecayTracker) Sweep(now time.Time) []string {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	var ischemicIDs []string
	for id, rec := range dt.records {
		if !rec.Ischemic && now.After(rec.Deadline) {
			rec.Ischemic = true
			ischemicIDs = append(ischemicIDs, id)
			if dt.onIschemia != nil {
				goroutinelabels.NewGoroutine("liveness_ischemia", "handle holon ischemia").StartSimple(func() { dt.onIschemia(rec) })
			}
		}
	}
	return ischemicIDs
}

// VitalitySnapshot returns a point-in-time slice of vitality metrics across all tracked holons.
func (dt *DecayTracker) VitalitySnapshot() []HolonVitality {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	now := time.Now()
	res := make([]HolonVitality, 0, len(dt.records))
	for id, rec := range dt.records {
		v := HolonVitality{
			HolonID:       id,
			PID:           rec.LastPacket.PID,
			CadenceMS:     rec.LastPacket.CadenceMS,
			Phase:         rec.LastPacket.Phase,
			LastSeenAgoMS: now.Sub(rec.ReceivedAt).Milliseconds(),
			Ischemic:      rec.Ischemic,
			PulseCount:    rec.PulseCount,
			Metadata:      rec.LastPacket.Metadata,
		}
		res = append(res, v)
	}
	return res
}

// Remove cleans up a terminated or completed holon from the decay table.
func (dt *DecayTracker) Remove(holonID string) {
	dt.mu.Lock()
	delete(dt.records, holonID)
	dt.mu.Unlock()
}

// PulseListener hosts the Unix domain datagram socket and routes received pulses to the decay tracker.
type PulseListener struct {
	socketPath string
	tracker    *DecayTracker
	conn       net.PacketConn
	mu         sync.Mutex
	stopCh     chan struct{}
	doneCh     chan struct{}
}

// NewPulseListener creates a pulse listener bound to socketPath.
func NewPulseListener(socketPath string, tracker *DecayTracker) *PulseListener {
	if tracker == nil {
		tracker = NewDecayTracker(nil, nil)
	}
	return &PulseListener{
		socketPath: socketPath,
		tracker:    tracker,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// Start begins listening on the Unix domain datagram socket and starts background decay sweeping.
func (pl *PulseListener) Start(ctx context.Context) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	// Ensure parent directory exists
	dir := filepath.Dir(pl.socketPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("create socket directory: %w", err)
	}

	// Remove stale socket file if present
	if rmErr := fileutil.Remove(pl.socketPath); rmErr != nil && !fileutil.IsNotExist(rmErr) {
		// Non-fatal stale socket cleanup
	}

	addr := &net.UnixAddr{Name: pl.socketPath, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return errfmt.Errorf("listen unixgram on %s: %w", pl.socketPath, err)
	}

	// Set socket file permissions
	if chErr := fileutil.Chmod(pl.socketPath, paths.SocketPerm666); chErr != nil {
		// Non-fatal if environment denies chmod
	}

	pl.conn = conn

	goroutinelabels.NewGoroutine("liveness_read_loop", "read incoming liveness datagrams").StartSimple(pl.readLoop)
	goroutinelabels.NewGoroutine("liveness_sweep_loop", "sweep decay table for ischemic holons").StartSimple(func() { pl.sweepLoop(ctx) })
	return nil
}

// Tracker returns the underlying DecayTracker.
func (pl *PulseListener) Tracker() *DecayTracker {
	return pl.tracker
}

// Stop cleanly unbinds the socket and terminates processing loops.
func (pl *PulseListener) Stop() error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	select {
	case <-pl.stopCh:
		return nil
	default:
		close(pl.stopCh)
	}

	if pl.conn != nil {
		if closeErr := pl.conn.Close(); closeErr != nil {
			// Best-effort socket close.
		}
	}
	<-pl.doneCh

	if rmErr := fileutil.Remove(pl.socketPath); rmErr != nil && !fileutil.IsNotExist(rmErr) {
		// Cleanup is best-effort.
	}
	return nil
}

func (pl *PulseListener) readLoop() {
	defer close(pl.doneCh)
	buf := make([]byte, MaxPulsePacketBytes)

	for {
		select {
		case <-pl.stopCh:
			return
		default:
		}

		if dlErr := pl.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); dlErr != nil {
			return
		}
		n, _, err := pl.conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-pl.stopCh:
				return
			default:
				// transient read error; continue listening
				continue
			}
		}

		if n <= 0 {
			continue
		}

		pkt, err := DecodePulsePacket(buf[:n])
		if err != nil {
			// discard malformed datagrams cleanly
			continue
		}

		pl.tracker.RecordPulse(pkt)
	}
}

func (pl *PulseListener) sweepLoop(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pl.stopCh:
			return
		case t := <-ticker.C:
			pl.tracker.Sweep(t)
		}
	}
}

// NewProjectPulseListener creates a pulse listener wired to the project's canonical socket path.
func NewProjectPulseListener(projectRoot string, tracker *DecayTracker) *PulseListener {
	return NewPulseListener(paths.LivenessSocketPath(projectRoot), tracker)
}
