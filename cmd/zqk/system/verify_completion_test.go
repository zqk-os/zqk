package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/kernel/verification"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestComputeTestCaseHash(t *testing.T) {
	tempRoot := t.TempDir()

	// Create dummy artifacts
	artifact1 := filepath.Join(tempRoot, "dummy1.txt")
	err := fileutil.WriteStandardFile(artifact1, []byte("hello world 1"))
	require.NoError(t, err)

	artifact2 := filepath.Join(tempRoot, "dummy2.txt")
	err = fileutil.WriteStandardFile(artifact2, []byte("hello world 2"))
	require.NoError(t, err)

	hash, err := computeTestCaseHash(tempRoot, []string{"dummy2.txt", "dummy1.txt"})
	require.NoError(t, err)

	// compute expected hash manually to verify identical logic
	hasher := sha256.New()

	h1 := sha256.New()
	h1.Write([]byte("hello world 1"))
	h1Str := hex.EncodeToString(h1.Sum(nil))

	h2 := sha256.New()
	h2.Write([]byte("hello world 2"))
	h2Str := hex.EncodeToString(h2.Sum(nil))

	// Should be sorted alphabetically by artifact path
	hasher.Write([]byte("dummy1.txt:" + h1Str + "\n"))
	hasher.Write([]byte("dummy2.txt:" + h2Str + "\n"))

	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	require.Equal(t, expectedHash, hash)
}

func TestExecuteVerificationTopology_Sequential(t *testing.T) {
	ctx := context.Background()
	stage1Ran := false
	stage2Ran := false

	org := &verification.ExecutionOrganizer{
		TestCaseID: "TST-SAMPLE-SEQ",
		Mode:       verification.TopologySequential,
		Stages: []verification.VerificationStage{
			{
				ID:       "CRIT-SEQ-001",
				Category: verification.CategoryStaticFloor,
				Verify: func(ctx context.Context) error {
					stage1Ran = true
					return nil
				},
			},
			{
				ID:       "CRIT-SEQ-002",
				Category: verification.CategoryOperationalProof,
				Verify: func(ctx context.Context) error {
					stage2Ran = true
					return nil
				},
			},
		},
	}

	report, err := ExecuteVerificationTopology(ctx, org)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.Passed)
	require.True(t, stage1Ran)
	require.True(t, stage2Ran)
	require.Equal(t, 2, len(report.StageResults))
}

func TestExecuteVerificationTopology_Concurrent(t *testing.T) {
	ctx := context.Background()

	org := &verification.ExecutionOrganizer{
		TestCaseID: "TST-SAMPLE-CONC",
		Mode:       verification.TopologyConcurrent,
		Stages: []verification.VerificationStage{
			{
				ID:       "CRIT-CONC-001",
				Category: verification.CategoryStaticFloor,
				Verify: func(ctx context.Context) error {
					return nil
				},
			},
			{
				ID:       "CRIT-CONC-002",
				Category: verification.CategoryOperationalProof,
				Verify: func(ctx context.Context) error {
					return nil
				},
			},
		},
	}

	report, err := ExecuteVerificationTopology(ctx, org)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.Passed)
	require.Equal(t, 2, len(report.StageResults))
}

func TestExecuteVerificationTopology_HybridDAG(t *testing.T) {
	ctx := context.Background()
	var order []string

	org := &verification.ExecutionOrganizer{
		TestCaseID: "TST-SAMPLE-DAG",
		Mode:       verification.TopologyHybridDAG,
		Stages: []verification.VerificationStage{
			{
				ID:       "CRIT-DAG-ROOT",
				Category: verification.CategoryStaticFloor,
				Verify: func(ctx context.Context) error {
					order = append(order, "CRIT-DAG-ROOT")
					return nil
				},
			},
			{
				ID:        "CRIT-DAG-LEAF",
				Category:  verification.CategoryOperationalProof,
				DependsOn: []string{"CRIT-DAG-ROOT"},
				Verify: func(ctx context.Context) error {
					order = append(order, "CRIT-DAG-LEAF")
					return nil
				},
			},
		},
	}

	report, err := ExecuteVerificationTopology(ctx, org)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.Passed)
	require.Equal(t, []string{"CRIT-DAG-ROOT", "CRIT-DAG-LEAF"}, order)
}

func TestExecuteVerificationTopology_PanicIsolation(t *testing.T) {
	ctx := context.Background()

	org := &verification.ExecutionOrganizer{
		TestCaseID: "TST-SAMPLE-PANIC",
		Mode:       verification.TopologySequential,
		Stages: []verification.VerificationStage{
			{
				ID:       "CRIT-PANIC-001",
				Category: verification.CategoryNegativeInvariant,
				Verify: func(ctx context.Context) error {
					panic("simulated fatal stage panic")
				},
			},
			{
				ID:       "CRIT-SAFE-002",
				Category: verification.CategoryStaticFloor,
				Verify: func(ctx context.Context) error {
					return nil
				},
			},
		},
	}

	report, err := ExecuteVerificationTopology(ctx, org)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.False(t, report.Passed)
	require.Equal(t, 1, report.PanicsCaught)
	res, ok := report.StageResults["CRIT-PANIC-001"]
	require.True(t, ok)
	require.True(t, res.Panicked)
	require.NotNil(t, res.Error)
	require.Contains(t, res.Error.Error(), "simulated fatal stage panic")
}

func TestVerifyCompletionCmdFlags(t *testing.T) {
	cmd := NewVerifyCompletionCmd()
	require.NotNil(t, cmd)

	organizerFlag := cmd.Flag("organizer")
	require.NotNil(t, organizerFlag)
	require.Equal(t, "false", organizerFlag.DefValue)

	topologyFlag := cmd.Flag("topology")
	require.NotNil(t, topologyFlag)
	require.Equal(t, "sequential", topologyFlag.DefValue)
}
