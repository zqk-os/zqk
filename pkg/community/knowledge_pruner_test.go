// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func mustNewPruner(t *testing.T, cfg KnowledgePrunerConfig) *KnowledgePruner {
	t.Helper()
	pruner, err := NewKnowledgePruner(cfg)
	if err != nil {
		t.Fatalf("NewKnowledgePruner failed: %v", err)
	}
	return pruner
}

func mustRegisterNodes(t *testing.T, pruner *KnowledgePruner, nodes ...KnowledgeNode) {
	t.Helper()
	for _, node := range nodes {
		if err := pruner.RegisterNode(node); err != nil {
			t.Fatalf("RegisterNode(%s) failed: %v", node.ID, err)
		}
	}
}

func TestKnowledgePruner_StaticFloorInvariant(t *testing.T) {
	if _, err := NewKnowledgePruner(KnowledgePrunerConfig{SimilarityThreshold: -0.1}); !errors.Is(err, ErrInvalidThreshold) {
		t.Fatalf("expected ErrInvalidThreshold for negative threshold, got %v", err)
	}
	if _, err := NewKnowledgePruner(KnowledgePrunerConfig{SimilarityThreshold: 1.5}); !errors.Is(err, ErrInvalidThreshold) {
		t.Fatalf("expected ErrInvalidThreshold for >1 threshold, got %v", err)
	}
	if _, err := NewKnowledgePruner(KnowledgePrunerConfig{ProtectedRootIDs: []string{"  "}}); err == nil {
		t.Fatal("expected error for blank protected root ID")
	}

	pruner := mustNewPruner(t, KnowledgePrunerConfig{ProtectedRootIDs: []string{"root-compass"}})

	if err := pruner.RegisterNode(KnowledgeNode{ID: "   ", Kind: "goal"}); err == nil {
		t.Fatal("expected error for blank node ID")
	}
	if err := pruner.RegisterNode(KnowledgeNode{ID: "n1", Kind: "req", Refs: []string{"\t"}}); err == nil {
		t.Fatal("expected error for blank edge target ID")
	}

	mustRegisterNodes(t, pruner, KnowledgeNode{ID: "root-compass", Kind: "goal", Title: "North Star"})
	if err := pruner.RegisterNode(KnowledgeNode{ID: "root-compass", Kind: "goal"}); !errors.Is(err, ErrDuplicateKnowledgeNode) {
		t.Fatalf("expected ErrDuplicateKnowledgeNode, got %v", err)
	}

	if err := pruner.ValidateGraph(); err != nil {
		t.Fatalf("ValidateGraph failed on valid graph: %v", err)
	}

	snap := pruner.SnapshotNodes()
	if !snap["root-compass"].IsRoot {
		t.Fatal("expected ProtectedRootIDs entry to be marked IsRoot")
	}
}

func TestKnowledgePruner_OperationalDynamicProof(t *testing.T) {
	baseTime := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	pruner := mustNewPruner(t, KnowledgePrunerConfig{SimilarityThreshold: 0.95})

	mustRegisterNodes(t, pruner,
		KnowledgeNode{ID: "root-goal", Kind: "goal", Title: "Core Mission", IsRoot: true, Refs: []string{"req-1", "req-dup"}, CreatedAt: baseTime},
		KnowledgeNode{ID: "req-1", Kind: "requirement", Title: "Auth Contract", ContentHash: "sha256:auth", Embedding: []float64{1, 0, 0}, Refs: []string{"crit-1"}, CreatedAt: baseTime.Add(time.Minute)},
		KnowledgeNode{ID: "req-dup", Kind: "requirement", Title: "Auth Contract Copy", ContentHash: "sha256:auth", CreatedAt: baseTime.Add(2 * time.Minute)},
		KnowledgeNode{ID: "crit-1", Kind: "criteria", Title: "Static Floor", Embedding: []float64{0, 1, 0}, CreatedAt: baseTime.Add(3 * time.Minute)},
		KnowledgeNode{ID: "crit-vec-dup", Kind: "criteria", Title: "Static Floor Near-Dup", Embedding: []float64{0.01, 0.9999, 0}, CreatedAt: baseTime.Add(4 * time.Minute)},
		KnowledgeNode{ID: "obsolete-req", Kind: "requirement", Title: "Legacy", Obsolete: true, CreatedAt: baseTime.Add(5 * time.Minute)},
		KnowledgeNode{ID: "orphan-a", Kind: "artifact", Title: "Dangling A", Refs: []string{"orphan-b"}, CreatedAt: baseTime.Add(6 * time.Minute)},
		KnowledgeNode{ID: "orphan-b", Kind: "artifact", Title: "Dangling B", CreatedAt: baseTime.Add(7 * time.Minute)},
	)

	// Rewire root-goal to also reference crit-vec-dup and obsolete-req so they are reachable before compaction.
	if err := pruner.DeleteNode("non-existent"); !errors.Is(err, ErrKnowledgeNodeNotFound) {
		t.Fatalf("expected ErrKnowledgeNodeNotFound, got %v", err)
	}

	orphans, err := pruner.DetectOrphans()
	if err != nil {
		t.Fatalf("DetectOrphans failed: %v", err)
	}
	expectedOrphans := []string{"crit-vec-dup", "obsolete-req", "orphan-a", "orphan-b"}
	if !reflect.DeepEqual(orphans, expectedOrphans) {
		t.Fatalf("expected orphans %v, got %v", expectedOrphans, orphans)
	}

	// Prune only orphan-a and orphan-b by registering a fresh pruner for clean separation of orphan vs compaction.
	compPruner := mustNewPruner(t, KnowledgePrunerConfig{SimilarityThreshold: 0.95})
	mustRegisterNodes(t, compPruner,
		KnowledgeNode{ID: "root-goal", Kind: "goal", IsRoot: true, Refs: []string{"req-1", "req-dup", "crit-vec-dup", "obsolete-req"}, CreatedAt: baseTime},
		KnowledgeNode{ID: "req-1", Kind: "requirement", ContentHash: "sha256:auth", Refs: []string{"crit-1"}, CreatedAt: baseTime.Add(time.Minute)},
		KnowledgeNode{ID: "req-dup", Kind: "requirement", ContentHash: "sha256:auth", CreatedAt: baseTime.Add(2 * time.Minute)},
		KnowledgeNode{ID: "crit-1", Kind: "criteria", Embedding: []float64{0, 1, 0}, CreatedAt: baseTime.Add(3 * time.Minute)},
		KnowledgeNode{ID: "crit-vec-dup", Kind: "criteria", Embedding: []float64{0.01, 0.9999, 0}, CreatedAt: baseTime.Add(4 * time.Minute)},
		KnowledgeNode{ID: "obsolete-req", Kind: "requirement", Obsolete: true, CreatedAt: baseTime.Add(5 * time.Minute)},
		KnowledgeNode{ID: "orphan-1", Kind: "artifact", Refs: []string{"orphan-2"}, CreatedAt: baseTime.Add(6 * time.Minute)},
		KnowledgeNode{ID: "orphan-2", Kind: "artifact", CreatedAt: baseTime.Add(7 * time.Minute)},
	)

	pruneReceipt, err := compPruner.PruneOrphans()
	if err != nil {
		t.Fatalf("PruneOrphans failed: %v", err)
	}
	if !reflect.DeepEqual(pruneReceipt.PrunedIDs, []string{"orphan-1", "orphan-2"}) {
		t.Fatalf("unexpected pruned IDs: %v", pruneReceipt.PrunedIDs)
	}
	if pruneReceipt.CascadedEdges != 1 {
		t.Fatalf("expected 1 cascaded edge, got %d", pruneReceipt.CascadedEdges)
	}

	compactReceipt, err := compPruner.CompactDuplicates()
	if err != nil {
		t.Fatalf("CompactDuplicates failed: %v", err)
	}
	if compactReceipt.CollapsedMap["req-dup"] != "req-1" {
		t.Fatalf("expected req-dup -> req-1, got %v", compactReceipt.CollapsedMap)
	}
	if compactReceipt.CollapsedMap["crit-vec-dup"] != "crit-1" {
		t.Fatalf("expected crit-vec-dup -> crit-1, got %v", compactReceipt.CollapsedMap)
	}
	if !reflect.DeepEqual(compactReceipt.RemovedObsolete, []string{"obsolete-req"}) {
		t.Fatalf("expected obsolete-req removed, got %v", compactReceipt.RemovedObsolete)
	}

	snap := compPruner.SnapshotNodes()
	if !reflect.DeepEqual(snap["root-goal"].Refs, []string{"req-1", "crit-1"}) {
		t.Fatalf("expected root-goal refs rewired to [req-1 crit-1], got %v", snap["root-goal"].Refs)
	}

	wg := goroutinelabels.StartTestGoroutines(t, 4, "pruner-reader", "concurrent snapshot", func(_ int) {
		if got := compPruner.SnapshotNodes(); len(got) == 0 {
			t.Errorf("expected non-empty snapshot during concurrent reads")
		}
	})
	wg.Wait()
}

func TestKnowledgePruner_NegativeAdversarialBoundary(t *testing.T) {
	strictPruner := mustNewPruner(t, KnowledgePrunerConfig{StrictCycles: true})
	mustRegisterNodes(t, strictPruner,
		KnowledgeNode{ID: "root-1", Kind: "goal", IsRoot: true, Refs: []string{"node-a"}},
		KnowledgeNode{ID: "node-a", Kind: "req", Refs: []string{"node-b"}},
		KnowledgeNode{ID: "node-b", Kind: "req", Refs: []string{"node-a"}},
	)

	if err := strictPruner.ValidateGraph(); !errors.Is(err, ErrCircularReference) {
		t.Fatalf("expected ErrCircularReference from ValidateGraph, got %v", err)
	}
	if _, err := strictPruner.DetectOrphans(); !errors.Is(err, ErrCircularReference) {
		t.Fatalf("expected ErrCircularReference from DetectOrphans, got %v", err)
	}
	if _, err := strictPruner.PruneOrphans(); !errors.Is(err, ErrCircularReference) {
		t.Fatalf("expected ErrCircularReference from PruneOrphans, got %v", err)
	}
	if _, err := strictPruner.CompactDuplicates(); !errors.Is(err, ErrCircularReference) {
		t.Fatalf("expected ErrCircularReference from CompactDuplicates, got %v", err)
	}

	// Non-strict mode must terminate safely on cycles and never prune protected roots.
	lenientPruner := mustNewPruner(t, KnowledgePrunerConfig{StrictCycles: false})
	mustRegisterNodes(t, lenientPruner,
		KnowledgeNode{ID: "root-compass", Kind: "goal", IsRoot: true, Obsolete: true, ContentHash: "same", Refs: []string{"reach-a"}},
		KnowledgeNode{ID: "reach-a", Kind: "req", Refs: []string{"reach-b"}},
		KnowledgeNode{ID: "reach-b", Kind: "req", Refs: []string{"reach-a"}},
		KnowledgeNode{ID: "orphan-cycle-1", Kind: "req", Refs: []string{"orphan-cycle-2"}},
		KnowledgeNode{ID: "orphan-cycle-2", Kind: "req", Refs: []string{"orphan-cycle-1"}},
	)

	if err := lenientPruner.DeleteNode("root-compass"); !errors.Is(err, ErrProtectedRootNode) {
		t.Fatalf("expected ErrProtectedRootNode on DeleteNode, got %v", err)
	}

	receipt, err := lenientPruner.PruneOrphans()
	if err != nil {
		t.Fatalf("lenient PruneOrphans failed: %v", err)
	}
	if !reflect.DeepEqual(receipt.PrunedIDs, []string{"orphan-cycle-1", "orphan-cycle-2"}) {
		t.Fatalf("expected orphan cycle pruned, got %v", receipt.PrunedIDs)
	}

	compactReceipt, err := lenientPruner.CompactDuplicates()
	if err != nil {
		t.Fatalf("lenient CompactDuplicates failed: %v", err)
	}
	if len(compactReceipt.RemovedObsolete) != 0 {
		t.Fatalf("expected protected root not removed even when Obsolete=true, got %v", compactReceipt.RemovedObsolete)
	}
	if _, exists := lenientPruner.SnapshotNodes()["root-compass"]; !exists {
		t.Fatal("root-compass must remain protected after pruning and compaction")
	}
}
