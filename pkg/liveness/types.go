package liveness

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const (
	// DefaultCadenceMS is the default interval (3000ms) between liveness pulses.
	DefaultCadenceMS = 3000

	// MinCadenceMS is the minimum acceptable cadence to prevent packet flooding.
	MinCadenceMS = 50

	// MaxCadenceMS is the maximum allowable cadence (1 hour).
	MaxCadenceMS = 3600000

	// DefaultIschemicMultiplier is the multiple of cadence before a process is considered ischemic (deadlocked/stalled).
	DefaultIschemicMultiplier = 2

	// MaxPulsePacketBytes is the maximum allowed datagram packet size (4KB).
	MaxPulsePacketBytes = 4096
)

// PulsePacket represents a discrete, non-blocking neuro-metabolic liveness event.
type PulsePacket struct {
	HolonID   string            `json:"holon_id"`
	PID       int               `json:"pid"`
	CadenceMS int               `json:"cadence_ms"`
	Phase     string            `json:"phase,omitempty"`
	Timestamp int64             `json:"timestamp_ns"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewPulsePacket constructs a valid PulsePacket with current timestamp and default cadence if unspecified.
func NewPulsePacket(holonID string, pid int, cadenceMS int, phase string) *PulsePacket {
	if cadenceMS <= 0 {
		cadenceMS = DefaultCadenceMS
	}
	return &PulsePacket{
		HolonID:   strings.TrimSpace(holonID),
		PID:       pid,
		CadenceMS: cadenceMS,
		Phase:     strings.TrimSpace(phase),
		Timestamp: time.Now().UnixNano(),
		Metadata:  make(map[string]string),
	}
}

// Validate checks structural integrity and bounds of the pulse packet.
func (p *PulsePacket) Validate() error {
	if strings.TrimSpace(p.HolonID) == "" {
		return errfmt.Errorf("pulse packet requires non-empty holon_id")
	}
	if p.PID <= 0 {
		return errfmt.Errorf("pulse packet requires positive pid (got %d)", p.PID)
	}
	if p.CadenceMS < MinCadenceMS || p.CadenceMS > MaxCadenceMS {
		return errfmt.Errorf("cadence_ms %d out of bounds [%d, %d]", p.CadenceMS, MinCadenceMS, MaxCadenceMS)
	}
	if p.Timestamp <= 0 {
		return errfmt.Errorf("pulse packet requires positive timestamp_ns")
	}
	return nil
}

// Encode serializes the packet to JSON bytes, verifying byte limit bounds.
func (p *PulsePacket) Encode() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, errfmt.Errorf("encode pulse packet: %w", err)
	}
	if len(b) > MaxPulsePacketBytes {
		return nil, errfmt.Errorf("encoded pulse packet %d bytes exceeds maximum %d bytes", len(b), MaxPulsePacketBytes)
	}
	return b, nil
}

// DecodePulsePacket parses and validates raw datagram bytes into a PulsePacket.
func DecodePulsePacket(b []byte) (*PulsePacket, error) {
	if len(b) == 0 {
		return nil, errfmt.Errorf("empty pulse packet payload")
	}
	if len(b) > MaxPulsePacketBytes {
		return nil, errfmt.Errorf("payload %d bytes exceeds limit %d", len(b), MaxPulsePacketBytes)
	}
	var p PulsePacket
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, errfmt.Errorf("decode pulse packet: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}
