package community

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestSemanticRouter(t *testing.T, cfg RouterConfig) *HyperSemanticRouter {
	t.Helper()
	router, err := NewHyperSemanticRouter(cfg)
	if err != nil {
		t.Fatalf("NewHyperSemanticRouter failed: %v", err)
	}
	return router
}

func mustRegisterSeat(t *testing.T, router *HyperSemanticRouter, seat SemanticClusterSeat) {
	t.Helper()
	if err := router.RegisterSeat(seat); err != nil {
		t.Fatalf("RegisterSeat(%s) failed: %v", seat.SeatID, err)
	}
}

func mustRoutePacket(t *testing.T, router *HyperSemanticRouter, pkt RoutingPacket) RoutingDecision {
	t.Helper()
	decision, err := router.MultiplexAndRoute(pkt)
	if err != nil {
		t.Fatalf("MultiplexAndRoute(%s) failed: %v", pkt.PacketID, err)
	}
	return decision
}

func assertCosineDistanceApprox(t *testing.T, a, b []float64, want float64) {
	t.Helper()
	got, err := CosineDistance(a, b)
	if err != nil {
		t.Fatalf("CosineDistance failed: %v", err)
	}
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("CosineDistance(%v, %v) = %f, want %f", a, b, got, want)
	}
}

func TestSemanticRouter_StaticFloorInvariant(t *testing.T) {
	assertCosineDistanceApprox(t, []float64{1.0, 2.0, 3.0}, []float64{1.0, 2.0, 3.0}, 0.0)
	assertCosineDistanceApprox(t, []float64{1.0, 0.0, 0.0}, []float64{0.0, 1.0, 0.0}, 1.0)
	assertCosineDistanceApprox(t, []float64{1.0, 0.0, 0.0}, []float64{-1.0, 0.0, 0.0}, 2.0)

	_, errEmpty := CosineDistance([]float64{}, []float64{1.0})
	assertErrorIs(t, errEmpty, ErrEmptyVector, "empty vector")

	_, errDim := CosineDistance([]float64{1.0, 0.0}, []float64{1.0, 0.0, 0.0})
	assertErrorIs(t, errDim, ErrDimensionMismatch, "dimension mismatch")

	_, errNaN := CosineDistance([]float64{math.NaN(), 1.0}, []float64{1.0, 0.0})
	assertErrorIs(t, errNaN, ErrNonFiniteVector, "NaN vector")

	_, errInf := CosineDistance([]float64{1.0, 0.0}, []float64{math.Inf(1), 0.0})
	assertErrorIs(t, errInf, ErrNonFiniteVector, "Inf vector")

	router := newTestSemanticRouter(t, RouterConfig{Dimensions: 3})
	if router.SeatCount() != 0 {
		t.Fatalf("expected 0 initial seats, got %d", router.SeatCount())
	}
}

func TestSemanticRouter_OperationalDynamicProof(t *testing.T) {
	router := newTestSemanticRouter(t, RouterConfig{
		Dimensions:            3,
		MaxDistanceCutoff:     0.6,
		StarvationThreshold:   3,
		TopologyAffinityBonus: 0.15,
	})

	mustRegisterSeat(t, router, SemanticClusterSeat{
		SeatID:       "seat-alpha-z1",
		TopologyZone: "zone-1",
		Vector:       []float64{1.0, 0.0, 0.0},
		Healthy:      true,
	})
	mustRegisterSeat(t, router, SemanticClusterSeat{
		SeatID:       "seat-alpha-z2",
		TopologyZone: "zone-2",
		Vector:       []float64{1.0, 0.0, 0.0},
		Healthy:      true,
	})
	mustRegisterSeat(t, router, SemanticClusterSeat{
		SeatID:       "seat-beta-z1",
		TopologyZone: "zone-1",
		Vector:       []float64{0.0, 1.0, 0.0},
		Healthy:      true,
	})
	mustRegisterSeat(t, router, SemanticClusterSeat{
		SeatID:       "seat-unhealthy",
		TopologyZone: "zone-1",
		Vector:       []float64{1.0, 0.0, 0.0},
		Healthy:      false,
	})

	if router.SeatCount() != 4 {
		t.Fatalf("expected 4 registered seats, got %d", router.SeatCount())
	}

	decision := mustRoutePacket(t, router, RoutingPacket{
		PacketID:      "pkt-zone-pref",
		TenantID:      "tenant-main",
		PreferredZone: "zone-2",
		Vector:        []float64{1.0, 0.0, 0.0},
		Payload:       []byte("zone-aware"),
	})
	if decision.SelectedSeatID != "seat-alpha-z2" {
		t.Fatalf("expected topology-preferred seat-alpha-z2, got %s", decision.SelectedSeatID)
	}

	const workers = 4
	const perWorker = 5
	errCh := make(chan error, workers*perWorker)

	wg := goroutinelabels.StartTestGoroutines(t, workers, "semantic_router_worker", "concurrent multi-tenant routing", func(id int) {
		tenant := fmt.Sprintf("tenant-%d", id%2)
		vec := []float64{1.0, 0.0, 0.0}
		zone := "zone-1"
		if id%2 == 1 {
			vec = []float64{0.0, 1.0, 0.0}
		}
		for i := 0; i < perWorker; i++ {
			_, err := router.MultiplexAndRoute(RoutingPacket{
				PacketID:      fmt.Sprintf("pkt-%d-%d", id, i),
				TenantID:      tenant,
				PreferredZone: zone,
				Vector:        vec,
			})
			if err != nil {
				errCh <- err
			}
		}
	})
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent MultiplexAndRoute failed: %v", err)
	}

	for _, tenant := range []string{"tenant-0", "tenant-1"} {
		buf, err := router.TenantBufferSnapshot(tenant)
		if err != nil {
			t.Fatalf("TenantBufferSnapshot(%s) failed: %v", tenant, err)
		}
		if len(buf) != 10 {
			t.Fatalf("expected 10 isolated packets for %s, got %d", tenant, len(buf))
		}
		for _, pkt := range buf {
			if pkt.TenantID != tenant {
				t.Fatalf("tenant isolation violation: buffer %s contains packet for %s", tenant, pkt.TenantID)
			}
		}
	}
}

func TestSemanticRouter_NegativeAdversarialBoundary(t *testing.T) {
	_, errCfg := NewHyperSemanticRouter(RouterConfig{Dimensions: -1})
	assertErrorIs(t, errCfg, ErrInvalidRouterConfig, "negative dimensions")

	_, errCutoff := NewHyperSemanticRouter(RouterConfig{MaxDistanceCutoff: 2.5})
	assertErrorIs(t, errCutoff, ErrInvalidRouterConfig, "out-of-range cutoff")

	router := newTestSemanticRouter(t, RouterConfig{
		Dimensions:          2,
		MaxDistanceCutoff:   0.25,
		StarvationThreshold: 2,
	})

	if _, err := router.MultiplexAndRoute(RoutingPacket{
		PacketID: "pkt-no-seats",
		TenantID: "tenant-a",
		Vector:   []float64{1.0, 0.0},
	}); !errors.Is(err, ErrNoSeatsAvailable) {
		t.Fatalf("expected ErrNoSeatsAvailable, got %v", err)
	}

	if err := router.RegisterSeat(SemanticClusterSeat{
		SeatID:  "   ",
		Vector:  []float64{1.0, 0.0},
		Healthy: true,
	}); err == nil {
		t.Fatalf("expected error for blank seat_id")
	}

	mustRegisterSeat(t, router, SemanticClusterSeat{
		SeatID:  "seat-1",
		Vector:  []float64{1.0, 0.0},
		Healthy: true,
	})

	errDup := router.RegisterSeat(SemanticClusterSeat{
		SeatID:  "seat-1",
		Vector:  []float64{1.0, 0.0},
		Healthy: true,
	})
	assertErrorIs(t, errDup, ErrSeatAlreadyRegistered, "duplicate seat")

	if _, err := router.MultiplexAndRoute(RoutingPacket{
		PacketID: "",
		TenantID: "tenant-a",
		Vector:   []float64{1.0, 0.0},
	}); err == nil {
		t.Fatalf("expected error for blank packet_id")
	}

	if _, err := router.MultiplexAndRoute(RoutingPacket{
		PacketID: "pkt-bad-dim",
		TenantID: "tenant-a",
		Vector:   []float64{1.0, 0.0, 0.0},
	}); !errors.Is(err, ErrDimensionMismatch) {
		t.Fatalf("expected ErrDimensionMismatch, got %v", err)
	}

	// First orthogonal packet exceeds MaxDistanceCutoff (dist = 1.0 > 0.25) -> ErrDistanceCutoffExceeded.
	_, errFirstReject := router.MultiplexAndRoute(RoutingPacket{
		PacketID: "pkt-far-1",
		TenantID: "tenant-a",
		Vector:   []float64{0.0, 1.0},
	})
	assertErrorIs(t, errFirstReject, ErrDistanceCutoffExceeded, "first distance cutoff exceeded")

	// Second orthogonal packet reaches StarvationThreshold (2) -> ErrStarvationCircuitOpen.
	_, errTrip := router.MultiplexAndRoute(RoutingPacket{
		PacketID: "pkt-far-2",
		TenantID: "tenant-a",
		Vector:   []float64{0.0, 1.0},
	})
	assertErrorIs(t, errTrip, ErrStarvationCircuitOpen, "starvation circuit tripped")

	// Even a matching packet is rejected while circuit is open.
	_, errOpen := router.MultiplexAndRoute(RoutingPacket{
		PacketID: "pkt-blocked",
		TenantID: "tenant-a",
		Vector:   []float64{1.0, 0.0},
	})
	assertErrorIs(t, errOpen, ErrStarvationCircuitOpen, "circuit remains open")

	assertErrorIs(t, router.ResetCircuit("non-existent"), ErrSeatNotFound, "reset unknown seat")

	if err := router.ResetCircuit("seat-1"); err != nil {
		t.Fatalf("ResetCircuit(seat-1) failed: %v", err)
	}

	decision := mustRoutePacket(t, router, RoutingPacket{
		PacketID: "pkt-recovered",
		TenantID: "tenant-a",
		Vector:   []float64{1.0, 0.0},
	})
	if decision.SelectedSeatID != "seat-1" {
		t.Fatalf("expected recovered routing to seat-1, got %s", decision.SelectedSeatID)
	}
}
