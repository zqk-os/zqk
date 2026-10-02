package metabolism

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/swarm/pack"
)

var (
	// ErrReceptorSaturated is returned when an active execution session is already in progress for the holon URN (HTTP 409).
	ErrReceptorSaturated = errors.New("receptor saturated: active execution session already in progress for holon URN")

	// ErrInvalidHolonURN is returned when a holon URN is malformed.
	ErrInvalidHolonURN = errors.New("invalid holon URN format")

	// ErrLeaseNotFound is returned when attempting to release or close an unknown lease.
	ErrLeaseNotFound = errors.New("receptor lease not found or mismatched")
)

// HolonURN represents a parsed Uniform Resource Name for a holonic swarm pack.
// Canonical format: urn:zqk:pack:<name>:<version>:<fingerprint>
type HolonURN struct {
	Name        string
	Version     pack.SemVer
	Fingerprint string
	Raw         string
}

// String returns the canonical URN string.
func (u HolonURN) String() string {
	return u.Raw
}

// ComputeHolonURN deterministically computes the Holon URN from pack metadata and invocation parameters.
func ComputeHolonURN(name, version, contentDigest string, params map[string]interface{}) (HolonURN, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return HolonURN{}, fmt.Errorf("pack name is required")
	}

	sv, err := pack.ParseSemVer(version)
	if err != nil {
		return HolonURN{}, fmt.Errorf("invalid semver for holon URN: %w", err)
	}

	// Canonical sorted JSON for params
	var paramKeys []string
	for k := range params {
		paramKeys = append(paramKeys, k)
	}
	sort.Strings(paramKeys)

	type kv struct {
		K string      `json:"k"`
		V interface{} `json:"v"`
	}
	var canonicalParams []kv
	for _, k := range paramKeys {
		canonicalParams = append(canonicalParams, kv{K: k, V: params[k]})
	}

	paramBytes, err := json.Marshal(canonicalParams)
	if err != nil {
		return HolonURN{}, fmt.Errorf("failed to serialize params for holon URN: %w", err)
	}

	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(":"))
	h.Write([]byte(sv.String()))
	h.Write([]byte(":"))
	h.Write([]byte(contentDigest))
	h.Write([]byte(":"))
	h.Write(paramBytes)

	fingerprint := hex.EncodeToString(h.Sum(nil))[:16]
	raw := fmt.Sprintf("urn:zqk:pack:%s:%s:%s", name, sv.String(), fingerprint)

	return HolonURN{
		Name:        name,
		Version:     sv,
		Fingerprint: fingerprint,
		Raw:         raw,
	}, nil
}

// ParseHolonURN parses and validates a raw holon URN string.
func ParseHolonURN(raw string) (HolonURN, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 6 || parts[0] != "urn" || parts[1] != "zqk" || parts[2] != "pack" {
		return HolonURN{}, fmt.Errorf("%w: %q (expected urn:zqk:pack:<name>:<version>:<fingerprint>)", ErrInvalidHolonURN, raw)
	}

	name := parts[3]
	verStr := parts[4]
	fp := parts[5]

	if name == "" || fp == "" {
		return HolonURN{}, fmt.Errorf("%w: missing components in %q", ErrInvalidHolonURN, raw)
	}

	sv, err := pack.ParseSemVer(verStr)
	if err != nil {
		return HolonURN{}, fmt.Errorf("%w: %v", ErrInvalidHolonURN, err)
	}

	return HolonURN{
		Name:        name,
		Version:     sv,
		Fingerprint: fp,
		Raw:         raw,
	}, nil
}

// ReceptorState represents the execution lifecycle state of a Holon URN receptor.
type ReceptorState string

const (
	StateIdle   ReceptorState = "idle"
	StateActive ReceptorState = "active"
	StateClosed ReceptorState = "closed"
)

// RunTick records an execution epoch/tick in the pack's lifecycle lineage.
type RunTick struct {
	Epoch       int       `json:"epoch"`
	SessionID   string    `json:"session_id"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Outcome     string    `json:"outcome,omitempty"`
}

// ReceptorLineage tracks active leases and historical execution runs for a Holon URN.
type ReceptorLineage struct {
	URN           string        `json:"urn"`
	State         ReceptorState `json:"state"`
	ActiveLeaseID string        `json:"active_lease_id,omitempty"`
	ActiveSession string        `json:"active_session,omitempty"`
	Epoch         int           `json:"epoch"`
	History       []RunTick     `json:"history"`
}

// ReceptorLease represents an active execution lease held by an in-flight session.
type ReceptorLease struct {
	LeaseID   string
	URN       string
	SessionID string
	Epoch     int
	Acquired  time.Time
}

// ReceptorRegistry manages receptor saturation locks and historical lineages to ensure overdose immunity.
type ReceptorRegistry struct {
	mu       sync.RWMutex
	lineages map[string]*ReceptorLineage
}

// NewReceptorRegistry initializes an empty receptor registry.
func NewReceptorRegistry() *ReceptorRegistry {
	return &ReceptorRegistry{
		lineages: make(map[string]*ReceptorLineage),
	}
}

// Acquire attempts to acquire an active execution lease on the given Holon URN.
// If an active session is currently running for this URN, it fails closed with ErrReceptorSaturated (HTTP 409).
// If previous runs exist and are closed, it advances the epoch and reconciles the lineage.
func (r *ReceptorRegistry) Acquire(urn string, sessionID string) (*ReceptorLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	lineage, exists := r.lineages[urn]
	if !exists {
		lineage = &ReceptorLineage{
			URN:     urn,
			State:   StateIdle,
			Epoch:   0,
			History: make([]RunTick, 0),
		}
		r.lineages[urn] = lineage
	}

	if lineage.State == StateActive {
		return nil, fmt.Errorf("%w: active session %q is holding lease %q for %s",
			ErrReceptorSaturated, lineage.ActiveSession, lineage.ActiveLeaseID, urn)
	}

	lineage.Epoch++
	leaseID := fmt.Sprintf("lease-%d-%d", lineage.Epoch, time.Now().UnixNano())
	lineage.State = StateActive
	lineage.ActiveLeaseID = leaseID
	lineage.ActiveSession = sessionID

	tick := RunTick{
		Epoch:     lineage.Epoch,
		SessionID: sessionID,
		StartedAt: time.Now().UTC(),
	}
	lineage.History = append(lineage.History, tick)

	return &ReceptorLease{
		LeaseID:   leaseID,
		URN:       urn,
		SessionID: sessionID,
		Epoch:     lineage.Epoch,
		Acquired:  tick.StartedAt,
	}, nil
}

// Close marks the active lease as completed with a terminal outcome and transitions receptor state to StateClosed.
func (r *ReceptorRegistry) Close(urn string, leaseID string, outcome string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	lineage, exists := r.lineages[urn]
	if !exists || lineage.ActiveLeaseID != leaseID {
		return ErrLeaseNotFound
	}

	lineage.State = StateClosed
	lineage.ActiveLeaseID = ""
	lineage.ActiveSession = ""

	if len(lineage.History) > 0 {
		lastIdx := len(lineage.History) - 1
		lineage.History[lastIdx].CompletedAt = time.Now().UTC()
		lineage.History[lastIdx].Outcome = outcome
	}

	return nil
}

// Release cancels/aborts an active lease without recording a completed outcome, returning the receptor to idle/closed.
func (r *ReceptorRegistry) Release(urn string, leaseID string) error {
	return r.Close(urn, leaseID, "aborted")
}

// GetLineage retrieves a snapshot of the receptor lineage for a URN.
func (r *ReceptorRegistry) GetLineage(urn string) (*ReceptorLineage, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lineage, exists := r.lineages[urn]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid data races
	copyLineage := *lineage
	copyLineage.History = make([]RunTick, len(lineage.History))
	copy(copyLineage.History, lineage.History)
	return &copyLineage, true
}
