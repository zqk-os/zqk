package intake

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestIntakeCommand_NoArgs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tempDir, nil))
		_ = fileutil.RemoveAll(filepath.Join(tempDir, paths.ProjectDataDir))
	})
	_, err := testenvroot.Setup(tempDir)
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	cmd := NewIntakeCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = cmd.Execute()
	if err == nil {
		t.Fatal("Expected error when no arguments or stdin is provided, got nil")
	}

	if !strings.Contains(err.Error(), "no intent context provided") {
		t.Fatalf("Expected 'no intent context provided' error, got %v", err)
	}
}

func TestIntakeCommand_Flags(t *testing.T) {
	cmd := NewIntakeCmd()

	clusterFlag := cmd.Flags().Lookup("cluster")
	if clusterFlag == nil {
		t.Fatal("expected --cluster flag to be defined")
	}
	if clusterFlag.DefValue != "false" {
		t.Fatalf("expected --cluster default to be 'false', got %s", clusterFlag.DefValue)
	}

	concurrencyFlag := cmd.Flags().Lookup("concurrency")
	if concurrencyFlag == nil {
		t.Fatal("expected --concurrency flag to be defined")
	}
	if concurrencyFlag.DefValue != "false" {
		t.Fatalf("expected --concurrency default to be 'false', got %s", concurrencyFlag.DefValue)
	}

	strictAntiChainFlag := cmd.Flags().Lookup("strict-anti-chain")
	if strictAntiChainFlag == nil {
		t.Fatal("expected --strict-anti-chain flag to be defined")
	}
	if strictAntiChainFlag.DefValue != "true" {
		t.Fatalf("expected --strict-anti-chain default to be 'true', got %s", strictAntiChainFlag.DefValue)
	}
}

func TestSynthesizeIntakeMembrane_ClusteringAndTopologies(t *testing.T) {
	items := []IntakeObject{
		{
			Kind:        "requirement",
			Title:       "Harden storage write-behind flush concurrency",
			Description: "Ensure write-behind queues flush atomic journals in pkg/storage/wal/journal.go without loss",
		},
		{
			Kind:        "requirement",
			Title:       "Implement atomic journal rollback on WAL failure",
			Description: "Rollback staging journals cleanly in pkg/storage/wal/rollback.go and pkg/storage/wal/journal.go",
		},
		{
			Kind:        "requirement",
			Title:       "Improve CLI output table formatting",
			Description: "Add colorful table borders and compact column widths in pkg/cli/table.go",
		},
	}

	result, err := SynthesizeIntakeMembrane(items, false)
	if err != nil {
		t.Fatalf("SynthesizeIntakeMembrane failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil synthesis result")
	}

	if result.TotalInputCount != 3 {
		t.Fatalf("expected 3 total inputs, got %d", result.TotalInputCount)
	}
	if len(result.Clusters) < 1 {
		t.Fatalf("expected at least 1 cluster, got %d", len(result.Clusters))
	}
	if len(result.Topologies) != len(result.Clusters) {
		t.Fatalf("expected topologies count %d to match clusters count %d", len(result.Topologies), len(result.Clusters))
	}
}

func TestSynthesizeIntakeMembrane_EmptyInput(t *testing.T) {
	_, err := SynthesizeIntakeMembrane(nil, false)
	if err == nil {
		t.Fatal("expected error on empty input slice, got nil")
	}
}
