package community

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	defaultMaxDistanceCutoff     = 1.0
	defaultStarvationThreshold   = 3
	defaultTopologyAffinityBonus = 0.1
	maxCosineDistance            = 2.0
)

var (
	// ErrNonFiniteVector indicates that a vector contains NaN or Inf values.
	ErrNonFiniteVector = errors.New("vector embedding contains non-finite values")

	// ErrInvalidRouterConfig indicates an invalid HyperSemanticRouter configuration.
	ErrInvalidRouterConfig = errors.New("invalid semantic router configuration")

	// ErrSeatAlreadyRegistered indicates a duplicate cluster seat registration.
	ErrSeatAlreadyRegistered = errors.New("semantic cluster seat already registered")

	// ErrSeatNotFound indicates the requested cluster seat does not exist.
	ErrSeatNotFound = errors.New("semantic cluster seat not found")

	// ErrNoSeatsAvailable indicates no healthy cluster seats are available for routing.
	ErrNoSeatsAvailable = errors.New("no healthy semantic cluster seats available")

	// ErrDistanceCutoffExceeded indicates all candidate seats exceeded MaxDistanceCutoff.
	ErrDistanceCutoffExceeded = errors.New("semantic cosine distance exceeded cutoff")

	// ErrStarvationCircuitOpen indicates the starvation circuit breaker is open.
	ErrStarvationCircuitOpen = errors.New("starvation circuit breaker open")
)

// SemanticClusterSeat represents a routable cluster seat with its semantic vector and topology metadata.
type SemanticClusterSeat struct {
	SeatID         string    `json:"seat_id"`
	TopologyZone   string    `json:"topology_zone"`
	Vector         []float64 `json:"vector"`
	Healthy        bool      `json:"healthy"`
	AffinityWeight float64   `json:"affinity_weight"`
}

// RoutingPacket represents a tenant-scoped task packet to be routed along semantic proximity vectors.
type RoutingPacket struct {
	PacketID      string    `json:"packet_id"`
	TenantID      string    `json:"tenant_id"`
	PreferredZone string    `json:"preferred_zone"`
	Vector        []float64 `json:"vector"`
	Payload       []byte    `json:"payload,omitempty"`
}

// RoutingDecision describes the selected cluster seat and scoring metrics for a routed packet.
type RoutingDecision struct {
	PacketID        string  `json:"packet_id"`
	TenantID        string  `json:"tenant_id"`
	SelectedSeatID  string  `json:"selected_seat_id"`
	TopologyZone    string  `json:"topology_zone"`
	Distance        float64 `json:"distance"`
	Proximity       float64 `json:"proximity"`
	AffinityScore   float64 `json:"affinity_score"`
	TenantBufferLen int     `json:"tenant_buffer_len"`
}

// RouterConfig configures vector dimensionality, distance cutoffs, and starvation circuit breaking.
type RouterConfig struct {
	Dimensions            int     `json:"dimensions"`
	MaxDistanceCutoff     float64 `json:"max_distance_cutoff"`
	StarvationThreshold   int     `json:"starvation_threshold"`
	TopologyAffinityBonus float64 `json:"topology_affinity_bonus"`
}

type seatState struct {
	seat                  SemanticClusterSeat
	consecutiveRejections int
	circuitOpen           bool
}

type seatCandidate struct {
	seatID   string
	zone     string
	distance float64
	score    float64
}

// HyperSemanticRouter performs thread-safe, multi-tenant semantic routing with starvation circuit breaking.
type HyperSemanticRouter struct {
	mu            sync.RWMutex
	config        RouterConfig
	seats         map[string]*seatState
	tenantBuffers map[string][]RoutingPacket
}

// CosineDistance computes 1.0 - CosineSimilarity(a, b) after enforcing non-empty, equal-dimension, finite vectors.
func CosineDistance(a, b []float64) (float64, error) {
	if err := validateVectorPair(a, b); err != nil {
		return 0, err
	}

	sim := CosineSimilarity(a, b)
	dist := 1.0 - sim
	if dist < 0.0 {
		dist = 0.0
	}
	if dist > maxCosineDistance {
		dist = maxCosineDistance
	}
	return dist, nil
}

func validateFiniteVector(vec []float64) error {
	if len(vec) == 0 {
		return ErrEmptyVector
	}
	for _, v := range vec {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrNonFiniteVector
		}
	}
	return nil
}

func validateVectorPair(a, b []float64) error {
	if err := validateFiniteVector(a); err != nil {
		return err
	}
	if err := validateFiniteVector(b); err != nil {
		return err
	}
	if len(a) != len(b) {
		return ErrDimensionMismatch
	}
	return nil
}

func cloneVector(vec []float64) []float64 {
	cp := make([]float64, len(vec))
	copy(cp, vec)
	return cp
}

func clonePacket(p RoutingPacket) RoutingPacket {
	cp := p
	cp.Vector = cloneVector(p.Vector)
	if len(p.Payload) > 0 {
		cp.Payload = make([]byte, len(p.Payload))
		copy(cp.Payload, p.Payload)
	}
	return cp
}

// NewHyperSemanticRouter initializes a HyperSemanticRouter with validated configuration defaults.
func NewHyperSemanticRouter(cfg RouterConfig) (*HyperSemanticRouter, error) {
	if cfg.Dimensions < 0 {
		return nil, fmt.Errorf("%w: dimensions cannot be negative", ErrInvalidRouterConfig)
	}
	if math.IsNaN(cfg.MaxDistanceCutoff) || math.IsInf(cfg.MaxDistanceCutoff, 0) ||
		cfg.MaxDistanceCutoff < 0 || cfg.MaxDistanceCutoff > maxCosineDistance {
		return nil, fmt.Errorf("%w: max_distance_cutoff must be within [0, 2]", ErrInvalidRouterConfig)
	}
	if math.IsNaN(cfg.TopologyAffinityBonus) || math.IsInf(cfg.TopologyAffinityBonus, 0) || cfg.TopologyAffinityBonus < 0 {
		return nil, fmt.Errorf("%w: topology_affinity_bonus must be finite and non-negative", ErrInvalidRouterConfig)
	}
	if cfg.StarvationThreshold < 0 {
		return nil, fmt.Errorf("%w: starvation_threshold cannot be negative", ErrInvalidRouterConfig)
	}

	if cfg.MaxDistanceCutoff == 0 {
		cfg.MaxDistanceCutoff = defaultMaxDistanceCutoff
	}
	if cfg.StarvationThreshold == 0 {
		cfg.StarvationThreshold = defaultStarvationThreshold
	}
	if cfg.TopologyAffinityBonus == 0 {
		cfg.TopologyAffinityBonus = defaultTopologyAffinityBonus
	}

	return &HyperSemanticRouter{
		config:        cfg,
		seats:         make(map[string]*seatState),
		tenantBuffers: make(map[string][]RoutingPacket),
	}, nil
}

func (r *HyperSemanticRouter) validateSeatOrPacketVector(vec []float64) error {
	if err := validateFiniteVector(vec); err != nil {
		return err
	}
	if r.config.Dimensions > 0 && len(vec) != r.config.Dimensions {
		return ErrDimensionMismatch
	}
	return nil
}

// RegisterSeat registers a semantic cluster seat in a thread-safe manner.
func (r *HyperSemanticRouter) RegisterSeat(seat SemanticClusterSeat) error {
	if err := strutil.ErrorOnBlank(seat.SeatID, "seat_id cannot be blank"); err != nil {
		return err
	}
	if err := r.validateSeatOrPacketVector(seat.Vector); err != nil {
		return err
	}
	if math.IsNaN(seat.AffinityWeight) || math.IsInf(seat.AffinityWeight, 0) {
		return fmt.Errorf("%w: seat affinity_weight must be finite", ErrInvalidRouterConfig)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.seats[seat.SeatID]; exists {
		return ErrSeatAlreadyRegistered
	}

	if r.config.Dimensions == 0 {
		r.config.Dimensions = len(seat.Vector)
	}

	stored := seat
	stored.Vector = cloneVector(seat.Vector)
	r.seats[seat.SeatID] = &seatState{seat: stored}
	return nil
}

// SeatCount returns the number of registered cluster seats.
func (r *HyperSemanticRouter) SeatCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.seats)
}

// ResetCircuit resets the starvation circuit breaker and consecutive rejection counter for a seat.
func (r *HyperSemanticRouter) ResetCircuit(seatID string) error {
	if err := strutil.ErrorOnBlank(seatID, "seat_id cannot be blank"); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	state, exists := r.seats[seatID]
	if !exists {
		return ErrSeatNotFound
	}
	state.consecutiveRejections = 0
	state.circuitOpen = false
	return nil
}

// TenantBufferSnapshot returns a copy of the isolated context buffer for the given tenant.
func (r *HyperSemanticRouter) TenantBufferSnapshot(tenantID string) ([]RoutingPacket, error) {
	if err := strutil.ErrorOnBlank(tenantID, "tenant_id cannot be blank"); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	buf := r.tenantBuffers[tenantID]
	snapshot := make([]RoutingPacket, len(buf))
	for i, pkt := range buf {
		snapshot[i] = clonePacket(pkt)
	}
	return snapshot, nil
}

// MultiplexAndRoute isolates packet context per tenant, ranks healthy seats by semantic proximity
// and topology affinity, enforces MaxDistanceCutoff, and manages starvation circuit breaking.
func (r *HyperSemanticRouter) MultiplexAndRoute(packet RoutingPacket) (RoutingDecision, error) {
	if err := strutil.ErrorOnBlank(packet.PacketID, "packet_id cannot be blank"); err != nil {
		return RoutingDecision{}, err
	}
	if err := strutil.ErrorOnBlank(packet.TenantID, "tenant_id cannot be blank"); err != nil {
		return RoutingDecision{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.validateSeatOrPacketVector(packet.Vector); err != nil {
		return RoutingDecision{}, err
	}

	candidates, healthyCount, err := r.rankHealthyCandidatesLocked(packet)
	if err != nil {
		return RoutingDecision{}, err
	}
	if healthyCount == 0 {
		return RoutingDecision{}, ErrNoSeatsAvailable
	}
	if len(candidates) == 0 {
		return RoutingDecision{}, ErrStarvationCircuitOpen
	}

	best := candidates[0]
	if best.distance > r.config.MaxDistanceCutoff {
		if r.recordRejectionLocked(candidates) {
			return RoutingDecision{}, ErrStarvationCircuitOpen
		}
		return RoutingDecision{}, ErrDistanceCutoffExceeded
	}

	r.seats[best.seatID].consecutiveRejections = 0
	buf := append(r.tenantBuffers[packet.TenantID], clonePacket(packet))
	r.tenantBuffers[packet.TenantID] = buf

	return RoutingDecision{
		PacketID:        packet.PacketID,
		TenantID:        packet.TenantID,
		SelectedSeatID:  best.seatID,
		TopologyZone:    best.zone,
		Distance:        best.distance,
		Proximity:       1.0 - best.distance,
		AffinityScore:   best.score,
		TenantBufferLen: len(buf),
	}, nil
}

func (r *HyperSemanticRouter) rankHealthyCandidatesLocked(packet RoutingPacket) ([]seatCandidate, int, error) {
	healthyCount := 0
	candidates := make([]seatCandidate, 0, len(r.seats))

	for _, state := range r.seats {
		if !state.seat.Healthy {
			continue
		}
		healthyCount++
		if state.circuitOpen {
			continue
		}

		dist, err := CosineDistance(packet.Vector, state.seat.Vector)
		if err != nil {
			return nil, healthyCount, err
		}

		score := (1.0 - dist) + state.seat.AffinityWeight
		if packet.PreferredZone != "" && state.seat.TopologyZone == packet.PreferredZone {
			score += r.config.TopologyAffinityBonus
		}

		candidates = append(candidates, seatCandidate{
			seatID:   state.seat.SeatID,
			zone:     state.seat.TopologyZone,
			distance: dist,
			score:    score,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].seatID < candidates[j].seatID
	})

	return candidates, healthyCount, nil
}

func (r *HyperSemanticRouter) recordRejectionLocked(candidates []seatCandidate) bool {
	tripped := false
	for _, cand := range candidates {
		state := r.seats[cand.seatID]
		state.consecutiveRejections++
		if state.consecutiveRejections >= r.config.StarvationThreshold {
			state.circuitOpen = true
			tripped = true
		}
	}
	return tripped
}
