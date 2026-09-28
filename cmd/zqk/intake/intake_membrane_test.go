package intake

import (
	"errors"
	"testing"

	kernelintake "github.com/zqk-os/zqk/pkg/kernel/intake"
)

// TestSynthesizeIntakeMembrane_ClusteringGroupsRelatedRequests tests that semantic clustering
// aggregates requests touching common domains/paths into cohesive workstreams.
func TestSynthesizeIntakeMembrane_ClusteringGroupsRelatedRequests(t *testing.T) {
	objs := []IntakeObject{
		{
			Kind:        "requirement",
			Title:       "Add sqlite query timeout handling",
			Description: "Configure statement timeout on sqlite database connection pool in pkg/storage/sqlite",
		},
		{
			Kind:        "requirement",
			Title:       "Support sqlite busy retry handler",
			Description: "Add exponential backoff when database is locked in pkg/storage/sqlite",
		},
		{
			Kind:        "requirement",
			Title:       "Add Prometheus metrics exporter",
			Description: "Export prometheus HTTP telemetry in pkg/telemetry/metrics",
		},
	}

	result, err := SynthesizeIntakeMembrane(objs, false)
	if err != nil {
		t.Fatalf("SynthesizeIntakeMembrane failed: %v", err)
	}

	if result.TotalInputCount != 3 {
		t.Errorf("expected 3 total input items, got %d", result.TotalInputCount)
	}

	if len(result.Clusters) != 2 {
		t.Errorf("expected 2 clusters (sqlite group and telemetry), got %d", len(result.Clusters))
	}

	if len(result.Topologies) != len(result.Clusters) {
		t.Errorf("topologies count %d must match clusters count %d", len(result.Topologies), len(result.Clusters))
	}
}

// TestSynthesizeIntakeMembrane_AntiChainingRejection ensures redundant 1:1 micro-chains
// across the same domain are rejected fail-closed.
func TestSynthesizeIntakeMembrane_AntiChainingRejection(t *testing.T) {
	objs := []IntakeObject{
		{
			Kind:        "micro_domain",
			Title:       "Alpha initialization routine",
			Description: "Bootstraps isolated subsystem in pkg/alpha/file1.go",
		},
		{
			Kind:        "micro_domain",
			Title:       "Beta calculation engine",
			Description: "Computes mathematical matrix in pkg/beta/file2.go",
		},
		{
			Kind:        "micro_domain",
			Title:       "Gamma network telemetry",
			Description: "Transmits remote telemetry packet in pkg/gamma/file3.go",
		},
	}

	_, err := SynthesizeIntakeMembrane(objs, true)
	if err == nil {
		t.Fatal("expected anti-chain rejection error, got nil")
	}

	if !errors.Is(err, kernelintake.ErrRedundantOneToOneChains) {
		t.Errorf("expected ErrRedundantOneToOneChains, got %v", err)
	}
}

// TestSynthesizeIntakeMembrane_EmptyInputReturnsError verifies empty input safety.
func TestSynthesizeIntakeMembrane_EmptyInputReturnsError(t *testing.T) {
	_, err := SynthesizeIntakeMembrane(nil, true)
	if err == nil {
		t.Fatal("expected error on empty input, got nil")
	}
	if !errors.Is(err, kernelintake.ErrEmptyIntakeRequests) {
		t.Errorf("expected ErrEmptyIntakeRequests, got %v", err)
	}
}

// TestExtractTargetPaths validates path extraction from descriptions.
func TestExtractTargetPaths(t *testing.T) {
	desc := "Modify pkg/kernel/intake/membrane.go and cmd/zqk/intake/intake.go to support clustering."
	paths := extractTargetPaths(desc)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "pkg/kernel/intake/membrane.go" || paths[1] != "cmd/zqk/intake/intake.go" {
		t.Errorf("unexpected extracted paths: %v", paths)
	}
}
