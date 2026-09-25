package intake_test

import (
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/kernel/intake"
)

// TST-KERNEL-INTAKE-CLUSTERING-ORGANIZER: Composite Execution Organizer Test Harness
// Verifies:
// 1. Static Floor: CRIT-INTAKE-STATIC-SEMANTIC-GROUPING
// 2. Operational Proof: CRIT-INTAKE-OPERATIONAL-OVERLAP-SCRUTINY
// 3. Negative Invariant: CRIT-INTAKE-ADVERSARIAL-1TO1-CHAIN-REJECTION

// TestIntakeStaticSemanticGrouping verifies that requests sharing domain vocabulary
// and package paths are grouped into a cohesive cluster rather than individual micro-chains.
func TestIntakeStaticSemanticGrouping(t *testing.T) {
	membrane := intake.NewIntakeMembrane()

	incoming := []intake.IntakeRequest{
		{
			ID:             "REQ-RAW-001",
			Title:          "Harden storage write-behind flush concurrency",
			Description:    "Ensure write-behind queues flush atomic journals without data loss under concurrent contention",
			DomainCategory: "storage",
			TargetPaths:    []string{"pkg/storage/wal/journal.go", "pkg/storage/flush.go"},
		},
		{
			ID:             "REQ-RAW-002",
			Title:          "Implement atomic journal rollback on WAL failure",
			Description:    "Rollback staging journals cleanly when WAL sync fails or disk is full",
			DomainCategory: "storage",
			TargetPaths:    []string{"pkg/storage/wal/journal.go", "pkg/storage/wal/rollback.go"},
		},
		{
			ID:             "REQ-RAW-003",
			Title:          "Improve CLI output table formatting",
			Description:    "Add colorful table borders and compact column widths for human readability",
			DomainCategory: "cli",
			TargetPaths:    []string{"pkg/cli/table.go"},
		},
	}

	res, err := membrane.ProcessIntake(incoming)
	if err != nil {
		t.Fatalf("unexpected error during intake processing: %v", err)
	}

	if res == nil {
		t.Fatal("expected non-nil synthesis result")
	}

	// Should produce exactly 2 clusters: 1 for storage (containing REQ-RAW-001 and REQ-RAW-002), 1 for CLI
	if len(res.Clusters) != 2 {
		t.Fatalf("expected 2 clustered workstreams, got %d", len(res.Clusters))
	}

	var storageCluster *intake.ClusteredWorkstream
	for _, c := range res.Clusters {
		if c.DomainCategory == "storage" {
			storageCluster = c
			break
		}
	}

	if storageCluster == nil {
		t.Fatal("expected storage cluster to be present")
	}

	if len(storageCluster.Requests) != 2 {
		t.Errorf("expected storage cluster to group 2 requests, got %d", len(storageCluster.Requests))
	}

	// Assert common paths detected
	if len(storageCluster.CommonPaths) == 0 || storageCluster.CommonPaths[0] != "pkg/storage/wal/journal.go" {
		t.Errorf("expected common path 'pkg/storage/wal/journal.go', got %v", storageCluster.CommonPaths)
	}
}

// TestIntakeOperationalOverlapScrutiny verifies dependency matrix analysis for
// concurrent execution vs sequential barriers vs hybrid DAGs.
func TestIntakeOperationalOverlapScrutiny(t *testing.T) {
	membrane := intake.NewIntakeMembrane()

	t.Run("Independent Non-Overlapping Tasks Yield ModeConcurrent", func(t *testing.T) {
		cluster := &intake.ClusteredWorkstream{
			ClusterID:      "CW-OPS-01",
			Title:          "Independent Tools",
			DomainCategory: "tooling",
			Requests: []intake.IntakeRequest{
				{
					ID:          "REQ-TOOL-A",
					Title:       "Update Secret Scanner patterns",
					TargetPaths: []string{"scripts/scan-secrets.sh"},
				},
				{
					ID:          "REQ-TOOL-B",
					Title:       "Enhance SBOM generator flags",
					TargetPaths: []string{"scripts/generate-sbom.sh"},
				},
			},
		}

		topo, err := membrane.SynthesizeTopology(cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if topo.Mode != intake.ModeConcurrent {
			t.Errorf("expected ModeConcurrent for independent tasks, got %s", topo.Mode)
		}
		if len(topo.Batches) != 1 || len(topo.Batches[0]) != 2 {
			t.Errorf("expected 1 batch with 2 parallel items, got %v", topo.Batches)
		}
	})

	t.Run("Conflicting Overlapping Target Paths Yield ModeHybridDAG", func(t *testing.T) {
		cluster := &intake.ClusteredWorkstream{
			ClusterID:      "CW-STORAGE-01",
			Title:          "Storage Mutations",
			DomainCategory: "storage",
			Requests: []intake.IntakeRequest{
				{
					ID:          "REQ-STAGE-0",
					Title:       "Add Base Storage Interface",
					TargetPaths: []string{"pkg/storage/interface.go"},
				},
				{
					ID:          "REQ-STAGE-1A",
					Title:       "Implement File Storage",
					TargetPaths: []string{"pkg/storage/interface.go", "pkg/storage/file.go"},
				},
				{
					ID:          "REQ-STAGE-1B",
					Title:       "Implement In-Memory Storage",
					TargetPaths: []string{"pkg/storage/mem.go"}, // Independent of interface.go
				},
			},
		}

		topo, err := membrane.SynthesizeTopology(cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if topo.Mode != intake.ModeHybridDAG {
			t.Errorf("expected ModeHybridDAG for mixed dependency tasks, got %s", topo.Mode)
		}

		// Stage 0 (REQ-STAGE-0 and REQ-STAGE-1B can start in Level 0)
		// REQ-STAGE-1A depends on REQ-STAGE-0 so must be in Level 1
		if len(topo.Batches) < 2 {
			t.Fatalf("expected at least 2 topological stages, got %d: %v", len(topo.Batches), topo.Batches)
		}

		if len(topo.Dependencies) != 1 {
			t.Fatalf("expected 1 dependency edge, got %d", len(topo.Dependencies))
		}
		if topo.Dependencies[0].RequestID != "REQ-STAGE-1A" || topo.Dependencies[0].DependsOn[0] != "REQ-STAGE-0" {
			t.Errorf("unexpected dependency: %v", topo.Dependencies[0])
		}
	})
}

// TestIntakeAdversarial1to1ChainRejection verifies that attempts to ingest redundant
// 1:1 micro-chains without clustering are rejected fail-closed.
func TestIntakeAdversarial1to1ChainRejection(t *testing.T) {
	membrane := intake.NewIntakeMembrane(intake.WithStrictAntiChain(true))

	t.Run("Rejects Empty Requests", func(t *testing.T) {
		_, err := membrane.ProcessIntake(nil)
		if !errors.Is(err, intake.ErrEmptyIntakeRequests) {
			t.Errorf("expected ErrEmptyIntakeRequests, got %v", err)
		}
	})

	t.Run("Rejects Fragmented Micro-chains in Same Domain", func(t *testing.T) {
		// 3 fragmented singletons in same domain with distinct titles and no clustering
		fragmented := []intake.IntakeRequest{
			{
				ID:             "REQ-FRAG-1",
				Title:          "Alpha initialization routine",
				Description:    "Bootstraps isolated subsystem",
				DomainCategory: "micro_domain",
				TargetPaths:    []string{"pkg/alpha/file1.go"},
			},
			{
				ID:             "REQ-FRAG-2",
				Title:          "Beta calculation engine",
				Description:    "Computes mathematical matrix",
				DomainCategory: "micro_domain",
				TargetPaths:    []string{"pkg/beta/file2.go"},
			},
			{
				ID:             "REQ-FRAG-3",
				Title:          "Gamma network telemetry",
				Description:    "Transmits remote telemetry packet",
				DomainCategory: "micro_domain",
				TargetPaths:    []string{"pkg/gamma/file3.go"},
			},
		}

		_, err := membrane.ProcessIntake(fragmented)
		if err == nil {
			t.Fatal("expected failure on fragmented 1:1 chains, got nil error")
		}
		if !errors.Is(err, intake.ErrRedundantOneToOneChains) {
			t.Errorf("expected ErrRedundantOneToOneChains, got %v", err)
		}
	})
}
