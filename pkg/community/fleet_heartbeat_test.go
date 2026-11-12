package community

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestFleetHeartbeat_StaticFloorInvariant verifies zero-value handling, capacity floors, and constructor preconditions.
func TestFleetHeartbeat_StaticFloorInvariant(t *testing.T) {
	// 1. NewSwarmFleetAggregator with invalid timeouts
	if _, err := NewSwarmFleetAggregator(0); err != ErrInvalidFleetTimeout {
		t.Fatalf("expected ErrInvalidFleetTimeout for zero duration, got %v", err)
	}
	if _, err := NewSwarmFleetAggregator(-5 * time.Second); err != ErrInvalidFleetTimeout {
		t.Fatalf("expected ErrInvalidFleetTimeout for negative duration, got %v", err)
	}

	agg, err := NewSwarmFleetAggregator(10 * time.Second)
	if err != nil {
		t.Fatalf("failed to initialize aggregator: %v", err)
	}

	// 2. Empty cluster aggregation invariant
	summary := agg.AggregateHealth(time.Now())
	if summary.TotalNodes != 0 || summary.ClusterStatus != "empty" {
		t.Fatalf("expected empty cluster summary, got %+v", summary)
	}

	// 3. Negative capacity rejection
	hb := FleetHeartbeat{
		NodeID:   "node-1",
		Role:     "worker",
		Capacity: -1,
	}
	if err := hb.Validate(); err != ErrInvalidCapacity {
		t.Fatalf("expected ErrInvalidCapacity, got %v", err)
	}
}

// TestFleetHeartbeat_OperationalDynamicProof tests concurrent heartbeats, dynamic status transitions, and health summaries.
func TestFleetHeartbeat_OperationalDynamicProof(t *testing.T) {
	staleTimeout := 2 * time.Second
	agg, err := NewSwarmFleetAggregator(staleTimeout)
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	baseTime := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

	// Concurrently record heartbeats for 20 nodes
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		wID := i
		goroutinelabels.NewGoroutine("test_fleet_heartbeat_worker", fmt.Sprintf("worker-%d", wID)).StartSimple(func() {
			defer wg.Done()
			hb := FleetHeartbeat{
				NodeID:          fmt.Sprintf("node-%02d", wID),
				Role:            "craftsman",
				Capacity:        10,
				CurrentWorkload: wID % 5,
				ReportedAt:      baseTime,
			}
			if err := agg.RecordHeartbeat(hb); err != nil {
				t.Errorf("failed to record heartbeat: %v", err)
			}
		})
	}
	wg.Wait()

	// Immediate evaluation: all nodes should be healthy
	summary := agg.AggregateHealth(baseTime)
	if summary.TotalNodes != 20 || summary.HealthyNodes != 20 || summary.StaleNodes != 0 {
		t.Fatalf("expected 20 healthy nodes, got %+v", summary)
	}
	if summary.ClusterStatus != "healthy" {
		t.Fatalf("expected healthy cluster status, got %s", summary.ClusterStatus)
	}

	// Single node check
	status, err := agg.GetNodeStatus("node-05", baseTime.Add(1*time.Second))
	if err != nil || status != StatusHealthy {
		t.Fatalf("expected StatusHealthy for node-05, got %v (err: %v)", status, err)
	}

	// Advance time past staleTimeout for all nodes
	staleTime := baseTime.Add(3 * time.Second)
	summaryStale := agg.AggregateHealth(staleTime)
	if summaryStale.HealthyNodes != 0 || summaryStale.StaleNodes != 20 {
		t.Fatalf("expected 20 stale nodes, got %+v", summaryStale)
	}
	if summaryStale.ClusterStatus != "critical" {
		t.Fatalf("expected critical cluster status when all nodes stale, got %s", summaryStale.ClusterStatus)
	}

	// Heartbeat update for node-01 resets freshness
	refreshedHB := FleetHeartbeat{
		NodeID:          "node-01",
		Role:            "craftsman",
		Capacity:        10,
		CurrentWorkload: 2,
		ReportedAt:      staleTime,
	}
	if err := agg.RecordHeartbeat(refreshedHB); err != nil {
		t.Fatalf("failed to record refresh heartbeat: %v", err)
	}

	summaryDegraded := agg.AggregateHealth(staleTime)
	if summaryDegraded.HealthyNodes != 1 || summaryDegraded.StaleNodes != 19 {
		t.Fatalf("expected 1 healthy and 19 stale, got %+v", summaryDegraded)
	}
	if summaryDegraded.ClusterStatus != "degraded" {
		t.Fatalf("expected degraded cluster status, got %s", summaryDegraded.ClusterStatus)
	}

	// Prune stale nodes
	prunedCount := agg.PruneStaleNodes(staleTime)
	if prunedCount != 19 {
		t.Fatalf("expected 19 pruned nodes, got %d", prunedCount)
	}

	postPruneSummary := agg.AggregateHealth(staleTime)
	if postPruneSummary.TotalNodes != 1 || postPruneSummary.HealthyNodes != 1 {
		t.Fatalf("expected 1 healthy node remaining, got %+v", postPruneSummary)
	}
	if postPruneSummary.ClusterStatus != "healthy" {
		t.Fatalf("expected healthy status after prune, got %s", postPruneSummary.ClusterStatus)
	}
}

// TestFleetHeartbeat_NegativeAdversarialBoundary tests invalid input rejection, blank IDs/roles, and non-existent queries.
func TestFleetHeartbeat_NegativeAdversarialBoundary(t *testing.T) {
	agg, err := NewSwarmFleetAggregator(5 * time.Second)
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	// 1. Blank node ID
	blankIDTests := []string{"", "   ", "\t\n"}
	for _, blankID := range blankIDTests {
		hb := FleetHeartbeat{
			NodeID: blankID,
			Role:   "tpm",
		}
		if err := agg.RecordHeartbeat(hb); err != ErrBlankNodeID {
			t.Fatalf("expected ErrBlankNodeID for %q, got %v", blankID, err)
		}

		if _, err := agg.GetNodeStatus(blankID, time.Now()); err != ErrBlankNodeID {
			t.Fatalf("expected ErrBlankNodeID on GetNodeStatus for %q, got %v", blankID, err)
		}
	}

	// 2. Blank role
	blankRoleTests := []string{"", "   ", "\t"}
	for _, blankRole := range blankRoleTests {
		hb := FleetHeartbeat{
			NodeID: "valid-node",
			Role:   blankRole,
		}
		if err := agg.RecordHeartbeat(hb); err != ErrBlankRole {
			t.Fatalf("expected ErrBlankRole for %q, got %v", blankRole, err)
		}
	}

	// 3. Query non-existent node
	if _, err := agg.GetNodeStatus("missing-node-404", time.Now()); err != ErrNodeNotFound {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}
