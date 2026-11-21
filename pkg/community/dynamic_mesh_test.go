// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func mustNewTestMesh(t *testing.T, cfg MeshConfig) *DynamicMesh {
	t.Helper()
	mesh, err := NewDynamicMesh(cfg)
	if err != nil {
		t.Fatalf("NewDynamicMesh failed: %v", err)
	}
	return mesh
}

func mustRegisterMeshNodes(t *testing.T, mesh *DynamicMesh, maxNeighbors int, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := mesh.RegisterNode(MeshNode{ID: id, MaxNeighbors: maxNeighbors, EpistemicDomain: "swarm"}); err != nil {
			t.Fatalf("RegisterNode(%q) failed: %v", id, err)
		}
	}
}

func TestDynamicMesh_StaticFloorInvariant(t *testing.T) {
	if _, err := NewDynamicMesh(MeshConfig{MaxNeighbors: 0}); !errors.Is(err, ErrInvalidMaxNeighbors) {
		t.Fatalf("expected ErrInvalidMaxNeighbors, got %v", err)
	}
	if _, err := NewDynamicMesh(MeshConfig{MaxNeighbors: 2, LatencyWeight: math.NaN()}); !errors.Is(err, ErrInvalidLinkWeight) {
		t.Fatalf("expected ErrInvalidLinkWeight for NaN weight, got %v", err)
	}

	mesh := mustNewTestMesh(t, MeshConfig{MaxNeighbors: 2})
	if err := mesh.RegisterNode(MeshNode{ID: "   ", MaxNeighbors: 2}); !errors.Is(err, ErrInvalidNodeID) {
		t.Fatalf("expected ErrInvalidNodeID for blank node, got %v", err)
	}
	if err := mesh.RegisterNode(MeshNode{ID: "node-a", MaxNeighbors: 0}); !errors.Is(err, ErrInvalidMaxNeighbors) {
		t.Fatalf("expected ErrInvalidMaxNeighbors, got %v", err)
	}

	mustRegisterMeshNodes(t, mesh, 1, "node-a", "node-b", "node-c")
	if err := mesh.RegisterNode(MeshNode{ID: "node-a", MaxNeighbors: 1}); !errors.Is(err, ErrNodeAlreadyExists) {
		t.Fatalf("expected ErrNodeAlreadyExists, got %v", err)
	}

	invalidMetrics := []LinkMetrics{
		{LatencyMs: 0, BandwidthMbps: 100, EpistemicProximity: 1},
		{LatencyMs: 5, BandwidthMbps: math.Inf(1), EpistemicProximity: 1},
		{LatencyMs: 5, BandwidthMbps: 100, EpistemicProximity: -0.1},
	}
	for _, m := range invalidMetrics {
		if err := mesh.UpsertLink("node-a", "node-b", m); !errors.Is(err, ErrInvalidLinkWeight) {
			t.Fatalf("expected ErrInvalidLinkWeight for %+v, got %v", m, err)
		}
	}

	if err := mesh.UpsertLink("node-a", "node-b", LinkMetrics{LatencyMs: 5, BandwidthMbps: 100, EpistemicProximity: 2}); err != nil {
		t.Fatalf("UpsertLink failed: %v", err)
	}
	if err := mesh.UpsertLink("node-a", "node-c", LinkMetrics{LatencyMs: 5, BandwidthMbps: 100, EpistemicProximity: 2}); !errors.Is(err, ErrMaxNeighborsExceeded) {
		t.Fatalf("expected ErrMaxNeighborsExceeded, got %v", err)
	}
}

func TestDynamicMesh_OperationalDynamicProof(t *testing.T) {
	mesh := mustNewTestMesh(t, MeshConfig{
		MaxNeighbors:    4,
		LatencyWeight:   1.0,
		BandwidthWeight: 100.0,
		EpistemicWeight: 20.0,
	})
	mustRegisterMeshNodes(t, mesh, 4, "alpha", "beta", "gamma", "delta")

	links := []struct {
		src, dst string
		metrics  LinkMetrics
	}{
		{"alpha", "beta", LinkMetrics{LatencyMs: 4, BandwidthMbps: 200, EpistemicProximity: 4}},
		{"beta", "delta", LinkMetrics{LatencyMs: 4, BandwidthMbps: 200, EpistemicProximity: 4}},
		{"alpha", "gamma", LinkMetrics{LatencyMs: 25, BandwidthMbps: 50, EpistemicProximity: 0}},
		{"gamma", "delta", LinkMetrics{LatencyMs: 25, BandwidthMbps: 50, EpistemicProximity: 0}},
	}
	for _, l := range links {
		if err := mesh.UpsertLink(l.src, l.dst, l.metrics); err != nil {
			t.Fatalf("UpsertLink(%s->%s) failed: %v", l.src, l.dst, err)
		}
	}

	route, err := mesh.ComputeRoute("alpha", "delta")
	if err != nil {
		t.Fatalf("initial ComputeRoute failed: %v", err)
	}
	if len(route.Hops) != 3 || route.Hops[1] != "beta" {
		t.Fatalf("expected route via beta, got %v", route.Hops)
	}

	// Degrade beta path and converge onto high-epistemic-proximity gamma path.
	if err := mesh.UpsertLink("alpha", "beta", LinkMetrics{LatencyMs: 80, BandwidthMbps: 10, EpistemicProximity: 0}); err != nil {
		t.Fatalf("UpsertLink update failed: %v", err)
	}
	if err := mesh.UpsertLink("alpha", "gamma", LinkMetrics{LatencyMs: 3, BandwidthMbps: 500, EpistemicProximity: 9}); err != nil {
		t.Fatalf("UpsertLink gamma1 update failed: %v", err)
	}
	if err := mesh.UpsertLink("gamma", "delta", LinkMetrics{LatencyMs: 3, BandwidthMbps: 500, EpistemicProximity: 9}); err != nil {
		t.Fatalf("UpsertLink gamma2 update failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_dynamic_mesh_router", "concurrent dynamic route verification").StartSimple(func() {
			defer wg.Done()
			r, rErr := mesh.ComputeRoute("alpha", "delta")
			if rErr != nil {
				t.Errorf("concurrent ComputeRoute failed: %v", rErr)
				return
			}
			if len(r.Hops) != 3 || r.Hops[1] != "gamma" {
				t.Errorf("expected converged route via gamma, got %v", r.Hops)
			}
		})
	}
	wg.Wait()

	snap := mesh.Snapshot()
	if snap.NodeCount != 4 || snap.LinkCount != 4 {
		t.Fatalf("unexpected snapshot counts: nodes=%d links=%d", snap.NodeCount, snap.LinkCount)
	}
	if math.IsInf(snap.DistanceMatrix["alpha"]["delta"], 1) {
		t.Fatal("expected finite distance matrix entry for alpha->delta")
	}
}

func TestDynamicMesh_NegativeAdversarialBoundary(t *testing.T) {
	mesh := mustNewTestMesh(t, MeshConfig{
		MaxNeighbors:                4,
		BlackholeLatencyThresholdMs: 500.0,
	})
	mustRegisterMeshNodes(t, mesh, 4, "src", "mid", "dst", "isolated")

	if _, err := mesh.ComputeRoute("", "dst"); !errors.Is(err, ErrInvalidNodeID) {
		t.Fatalf("expected ErrInvalidNodeID, got %v", err)
	}
	if _, err := mesh.ComputeRoute("src", "src"); !errors.Is(err, ErrSelfRoute) {
		t.Fatalf("expected ErrSelfRoute, got %v", err)
	}
	if _, err := mesh.ComputeRoute("src", "unknown"); !errors.Is(err, ErrMeshNodeNotFound) {
		t.Fatalf("expected ErrMeshNodeNotFound, got %v", err)
	}
	if _, err := mesh.ComputeRoute("src", "isolated"); !errors.Is(err, ErrDestinationUnreachable) {
		t.Fatalf("expected ErrDestinationUnreachable, got %v", err)
	}

	// Asymmetric latency spike on direct link src->dst triggers fallback via mid.
	if err := mesh.UpsertLink("src", "dst", LinkMetrics{LatencyMs: 750, BandwidthMbps: 100, EpistemicProximity: 1}); err != nil {
		t.Fatalf("UpsertLink direct failed: %v", err)
	}
	if err := mesh.UpsertLink("src", "mid", LinkMetrics{LatencyMs: 5, BandwidthMbps: 100, EpistemicProximity: 2}); err != nil {
		t.Fatalf("UpsertLink src->mid failed: %v", err)
	}
	if err := mesh.UpsertLink("mid", "dst", LinkMetrics{LatencyMs: 5, BandwidthMbps: 100, EpistemicProximity: 2}); err != nil {
		t.Fatalf("UpsertLink mid->dst failed: %v", err)
	}

	fallbackRoute, err := mesh.ComputeRoute("src", "dst")
	if err != nil {
		t.Fatalf("expected fallback route around asymmetric spike, got %v", err)
	}
	if !fallbackRoute.FallbackUsed || len(fallbackRoute.Hops) != 3 || fallbackRoute.Hops[1] != "mid" {
		t.Fatalf("expected fallback route via mid, got %+v", fallbackRoute)
	}

	// Blackhole remaining path to dst to verify ErrPartitionBlackhole and telemetry.
	if err := mesh.MarkLinkBlackholed("mid", "dst", true); err != nil {
		t.Fatalf("MarkLinkBlackholed failed: %v", err)
	}
	if _, err := mesh.ComputeRoute("src", "dst"); !errors.Is(err, ErrPartitionBlackhole) {
		t.Fatalf("expected ErrPartitionBlackhole, got %v", err)
	}

	snap := mesh.Snapshot()
	if snap.Telemetry.FallbackReroutes == 0 || snap.Telemetry.BlackholesDetected == 0 || snap.Telemetry.RouteFailures == 0 {
		t.Fatalf("expected non-zero failure/fallback telemetry, got %+v", snap.Telemetry)
	}
}
