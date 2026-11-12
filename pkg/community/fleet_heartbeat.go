package community

import (
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrBlankNodeID indicates that the node ID provided is empty or contains only whitespace.
	ErrBlankNodeID = errors.New("fleet node ID cannot be blank")

	// ErrBlankRole indicates that the node role is empty or contains only whitespace.
	ErrBlankRole = errors.New("fleet node role cannot be blank")

	// ErrInvalidFleetTimeout indicates that the configured stale node duration is non-positive.
	ErrInvalidFleetTimeout = errors.New("stale node timeout duration must be strictly positive")

	// ErrInvalidCapacity indicates that the advertised capacity is negative.
	ErrInvalidCapacity = errors.New("node capacity cannot be negative")

	// ErrNodeNotFound indicates that the requested node does not exist in the heartbeat registry.
	ErrNodeNotFound = errors.New("node not registered in fleet")
)

// FleetNodeStatus represents the operational availability of an individual fleet worker.
type FleetNodeStatus string

const (
	StatusHealthy FleetNodeStatus = "healthy"
	StatusStale   FleetNodeStatus = "stale"
)

// FleetHeartbeat captures instantaneous operational telemetry emitted by an active agent node.
type FleetHeartbeat struct {
	NodeID          string
	Role            string
	Capacity        int
	CurrentWorkload int
	ReportedAt      time.Time
}

// Validate ensures all fields satisfy kernel structural and declarative invariants.
func (h *FleetHeartbeat) Validate() error {
	if err := strutil.ErrorOnBlank(h.NodeID, "fleet node ID cannot be blank"); err != nil {
		return ErrBlankNodeID
	}
	if err := strutil.ErrorOnBlank(h.Role, "fleet node role cannot be blank"); err != nil {
		return ErrBlankRole
	}
	if h.Capacity < 0 {
		return ErrInvalidCapacity
	}
	if h.CurrentWorkload < 0 {
		h.CurrentWorkload = 0
	}
	if h.ReportedAt.IsZero() {
		h.ReportedAt = time.Now()
	}
	return nil
}

// FleetHealthSummary provides cluster-level operational metrics across all registered nodes.
type FleetHealthSummary struct {
	TotalNodes         int
	HealthyNodes       int
	StaleNodes         int
	TotalCapacity      int
	TotalWorkload      int
	AverageUtilization float64
	ClusterStatus      string
}

// SwarmFleetAggregator tracks live heartbeats and determines node lifecycle state across swarms.
type SwarmFleetAggregator struct {
	mu           sync.RWMutex
	staleTimeout time.Duration
	nodes        map[string]*FleetHeartbeat
}

// NewSwarmFleetAggregator initializes a thread-safe fleet aggregator with a given timeout.
func NewSwarmFleetAggregator(staleTimeout time.Duration) (*SwarmFleetAggregator, error) {
	if staleTimeout <= 0 {
		return nil, ErrInvalidFleetTimeout
	}
	return &SwarmFleetAggregator{
		staleTimeout: staleTimeout,
		nodes:        make(map[string]*FleetHeartbeat),
	}, nil
}

// RecordHeartbeat validates and records incoming agent telemetry.
func (a *SwarmFleetAggregator) RecordHeartbeat(hb FleetHeartbeat) error {
	if err := hb.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.nodes[hb.NodeID] = &hb
	return nil
}

// GetNodeStatus evaluates an individual node's operational status relative to the current time.
func (a *SwarmFleetAggregator) GetNodeStatus(nodeID string, now time.Time) (FleetNodeStatus, error) {
	if err := strutil.ErrorOnBlank(nodeID, "fleet node ID cannot be blank"); err != nil {
		return "", ErrBlankNodeID
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	hb, exists := a.nodes[nodeID]
	if !exists {
		return "", ErrNodeNotFound
	}

	if now.Sub(hb.ReportedAt) > a.staleTimeout {
		return StatusStale, nil
	}
	return StatusHealthy, nil
}

// PruneStaleNodes removes nodes that have failed to report within the stale timeout threshold.
func (a *SwarmFleetAggregator) PruneStaleNodes(now time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	pruned := 0
	for id, hb := range a.nodes {
		if now.Sub(hb.ReportedAt) > a.staleTimeout {
			delete(a.nodes, id)
			pruned++
		}
	}
	return pruned
}

// AggregateHealth computes cluster-wide health status and utilization.
func (a *SwarmFleetAggregator) AggregateHealth(now time.Time) FleetHealthSummary {
	a.mu.RLock()
	defer a.mu.RUnlock()

	summary := FleetHealthSummary{
		TotalNodes: len(a.nodes),
	}

	for _, hb := range a.nodes {
		summary.TotalCapacity += hb.Capacity
		summary.TotalWorkload += hb.CurrentWorkload

		if now.Sub(hb.ReportedAt) > a.staleTimeout {
			summary.StaleNodes++
		} else {
			summary.HealthyNodes++
		}
	}

	if summary.TotalCapacity > 0 {
		summary.AverageUtilization = float64(summary.TotalWorkload) / float64(summary.TotalCapacity)
	}

	if summary.TotalNodes == 0 {
		summary.ClusterStatus = "empty"
	} else if summary.HealthyNodes == 0 {
		summary.ClusterStatus = "critical"
	} else if summary.StaleNodes > 0 {
		summary.ClusterStatus = "degraded"
	} else {
		summary.ClusterStatus = "healthy"
	}

	return summary
}
