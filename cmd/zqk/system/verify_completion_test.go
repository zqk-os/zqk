package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernel/verification"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
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

func executeAndAssertPassingReport(t *testing.T, org *verification.ExecutionOrganizer) *verification.VerificationReport {
	t.Helper()
	report, err := ExecuteVerificationTopology(context.Background(), org)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.Passed)
	return report
}

func TestExecuteVerificationTopology_Sequential(t *testing.T) {
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

	report := executeAndAssertPassingReport(t, org)
	require.True(t, stage1Ran)
	require.True(t, stage2Ran)
	require.Equal(t, 2, len(report.StageResults))
}

func TestExecuteVerificationTopology_Concurrent(t *testing.T) {
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

	report := executeAndAssertPassingReport(t, org)
	require.Equal(t, 2, len(report.StageResults))
}

func TestExecuteVerificationTopology_HybridDAG(t *testing.T) {
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

	report := executeAndAssertPassingReport(t, org)
	require.Equal(t, []string{"CRIT-DAG-ROOT", "CRIT-DAG-LEAF"}, order)
	require.Equal(t, 2, len(report.StageResults))
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

func setupVerifyCompletionCmd(projectRoot string, store storage.ObjectStorageProvider) *cobra.Command {
	cmd := NewVerifyCompletionCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	ctx := context.Background()
	cliCtx := cli.ContextForProjectRoot(projectRoot)
	cli.SetContext(cmd, cliCtx)
	if store != nil {
		ctx = cli.WithStorageProvider(ctx, store)
	}
	cmd.SetContext(ctx)
	return cmd
}

func setupVerifyTestEnvironment(t *testing.T) (string, string, string) {
	t.Helper()
	tempRoot := t.TempDir()
	artifactPath := filepath.Join(tempRoot, "proof.txt")
	writeErr := fileutil.WriteStandardFile(artifactPath, []byte("proof payload"))
	require.NoError(t, writeErr)

	tcHash, hashErr := computeTestCaseHash(tempRoot, []string{"proof.txt"})
	require.NoError(t, hashErr)

	return tempRoot, "proof.txt", tcHash
}

func TestRunVerifyCompletion_NegativeBoundaries(t *testing.T) {
	tempRoot := t.TempDir()

	// 1. Object not found
	notFoundStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, storage.ErrObjectNotFound
		},
	}
	cmd1 := setupVerifyCompletionCmd(tempRoot, notFoundStore)
	err1 := runVerifyCompletion(cmd1, "OBJ-NOTFOUND")
	assert.Error(t, err1)
	assert.Contains(t, err1.Error(), "object OBJ-NOTFOUND not found")

	// 2. Storage read failure
	readErrStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, errors.New("underlying disk failure")
		},
	}
	cmd2 := setupVerifyCompletionCmd(tempRoot, readErrStore)
	err2 := runVerifyCompletion(cmd2, "OBJ-ERR")
	assert.Error(t, err2)
	assert.Contains(t, err2.Error(), "failed to read object")

	// 3. Unsupported kind
	unsupportedStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{objects.FieldKeyKind: "goal"}, nil
		},
	}
	cmd3 := setupVerifyCompletionCmd(tempRoot, unsupportedStore)
	err3 := runVerifyCompletion(cmd3, "GOAL-1")
	assert.Error(t, err3)
	assert.Contains(t, err3.Error(), "verification not supported for kind goal")
}

func TestRunVerifyCompletion_TestCase_DirectUnitProofs(t *testing.T) {
	tempRoot, artifactRel, tcHash := setupVerifyTestEnvironment(t)

	// 1. Project root not set
	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")
	noRootStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:                "test_case",
				objects.FieldKeyVerificationHash:    "dummy-hash",
				objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
			}, nil
		},
	}
	cmd1 := setupVerifyCompletionCmd("", noRootStore)
	err1 := runVerifyCompletion(cmd1, "TST-1")
	assert.Error(t, err1)
	assert.Contains(t, err1.Error(), "project root not set")

	// 2. Missing verification_hash
	t.Setenv("ZQK_TEST_ROOT", tempRoot)
	noHashStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:                "test_case",
				objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
			}, nil
		},
	}
	cmd2 := setupVerifyCompletionCmd(tempRoot, noHashStore)
	err2 := runVerifyCompletion(cmd2, "TST-1")
	assert.Error(t, err2)
	assert.Contains(t, err2.Error(), "has no verification_hash")

	// 3. Missing verified_artifact_refs
	noArtifactStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:             "test_case",
				objects.FieldKeyVerificationHash: tcHash,
			}, nil
		},
	}
	cmd3 := setupVerifyCompletionCmd(tempRoot, noArtifactStore)
	err3 := runVerifyCompletion(cmd3, "TST-1")
	assert.Error(t, err3)
	assert.Contains(t, err3.Error(), "has no verified_artifact_refs")

	// 4. Verification hash mismatch
	mismatchStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:                "test_case",
				objects.FieldKeyVerificationHash:    "mismatched-test-hash",
				objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
			}, nil
		},
	}
	cmd4 := setupVerifyCompletionCmd(tempRoot, mismatchStore)
	err4 := runVerifyCompletion(cmd4, "TST-1")
	assert.Error(t, err4)
	assert.Contains(t, err4.Error(), "verification failed for test_case")

	// 5. Successful verification without organizer
	validStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:                "test_case",
				objects.FieldKeyVerificationHash:    tcHash,
				objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
			}, nil
		},
	}
	cmd5 := setupVerifyCompletionCmd(tempRoot, validStore)
	err5 := runVerifyCompletion(cmd5, "TST-1")
	assert.NoError(t, err5)
}

func TestRunVerifyCompletion_TestCase_WithOrganizer(t *testing.T) {
	tempRoot, artifactRel, tcHash := setupVerifyTestEnvironment(t)
	t.Setenv("ZQK_TEST_ROOT", tempRoot)

	// 1. Success sequential, concurrent, and hybrid_dag
	validStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			if id == "TST-ORG" {
				return map[string]any{
					objects.FieldKeyKind:                "test_case",
					objects.FieldKeyVerificationHash:    tcHash,
					objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
					objects.FieldKeyCriteriaRefs:        []any{"CRIT-1", "CRIT-2"},
				}, nil
			}
			return map[string]any{objects.FieldKeyKind: "criteria"}, nil
		},
	}

	topologies := []string{"sequential", "concurrent", "hybrid_dag"}
	for _, topo := range topologies {
		cmd := setupVerifyCompletionCmd(tempRoot, validStore)
		setOrgErr := cmd.Flags().Set("organizer", "true")
		require.NoError(t, setOrgErr)
		setTopoErr := cmd.Flags().Set("topology", topo)
		require.NoError(t, setTopoErr)
		err := runVerifyCompletion(cmd, "TST-ORG")
		assert.NoError(t, err, "topology %s should succeed", topo)
	}

	// 2. Criteria read failure in organizer
	failStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			if id == "TST-ORG" {
				return map[string]any{
					objects.FieldKeyKind:                "test_case",
					objects.FieldKeyVerificationHash:    tcHash,
					objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
					objects.FieldKeyCriteriaRefs:        []string{"CRIT-FAIL", "CRIT-SAFE"},
				}, nil
			}
			if id == "CRIT-FAIL" {
				return nil, errors.New("read criteria failed")
			}
			return map[string]any{objects.FieldKeyKind: "criteria"}, nil
		},
	}
	failCmd := setupVerifyCompletionCmd(tempRoot, failStore)
	failOrgErr := failCmd.Flags().Set("organizer", "true")
	require.NoError(t, failOrgErr)
	failErr := runVerifyCompletion(failCmd, "TST-ORG")
	assert.Error(t, failErr)

	// 3. Criteria simulated panic caught in organizer
	panicStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			if id == "TST-ORG" {
				return map[string]any{
					objects.FieldKeyKind:                "test_case",
					objects.FieldKeyVerificationHash:    tcHash,
					objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
					objects.FieldKeyCriteriaRefs:        []any{"CRIT-PANIC", "CRIT-SAFE"},
				}, nil
			}
			if id == "CRIT-PANIC" {
				return map[string]any{
					objects.FieldKeyKind: "criteria",
					"simulate_panic":     true,
				}, nil
			}
			return map[string]any{
				objects.FieldKeyKind: "criteria",
			}, nil
		},
	}
	panicCmd := setupVerifyCompletionCmd(tempRoot, panicStore)
	panicOrgErr := panicCmd.Flags().Set("organizer", "true")
	require.NoError(t, panicOrgErr)
	panicErr := runVerifyCompletion(panicCmd, "TST-ORG")
	assert.Error(t, panicErr)
	assert.Contains(t, panicErr.Error(), "composite verification failed")
}

func TestRunVerifyCompletion_BacklogItem_DirectUnitProofs(t *testing.T) {
	tempRoot, artifactRel, tcHash := setupVerifyTestEnvironment(t)
	t.Setenv("ZQK_TEST_ROOT", tempRoot)

	bliHasher := sha256.New()
	bliHasher.Write([]byte("TST-1:" + tcHash + "\n"))
	expectedBLIHash := hex.EncodeToString(bliHasher.Sum(nil))

	// 1. Missing verification_hash
	noHashStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{objects.FieldKeyKind: "backlog_item"}, nil
		},
	}
	cmd1 := setupVerifyCompletionCmd(tempRoot, noHashStore)
	err1 := runVerifyCompletion(cmd1, "BLI-1")
	assert.Error(t, err1)
	assert.Contains(t, err1.Error(), "has no verification_hash")

	// 2. List test cases error
	listErrStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:             "backlog_item",
				objects.FieldKeyVerificationHash: expectedBLIHash,
			}, nil
		},
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, errors.New("list error")
		},
	}
	cmd2 := setupVerifyCompletionCmd(tempRoot, listErrStore)
	err2 := runVerifyCompletion(cmd2, "BLI-1")
	assert.Error(t, err2)
	assert.Contains(t, err2.Error(), "failed to list test cases")

	// 3. Zero test cases
	emptyStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:             "backlog_item",
				objects.FieldKeyVerificationHash: expectedBLIHash,
			}, nil
		},
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{Objects: nil}, nil
		},
	}
	cmd3 := setupVerifyCompletionCmd(tempRoot, emptyStore)
	err3 := runVerifyCompletion(cmd3, "BLI-1")
	assert.Error(t, err3)
	assert.Contains(t, err3.Error(), "has no test cases")

	// 4. Hash mismatch
	mismatchStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:             "backlog_item",
				objects.FieldKeyVerificationHash: "mismatched-bli-hash",
			}, nil
		},
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyID:                  "TST-1",
						objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
					},
				},
			}, nil
		},
	}
	cmd4 := setupVerifyCompletionCmd(tempRoot, mismatchStore)
	err4 := runVerifyCompletion(cmd4, "BLI-1")
	assert.Error(t, err4)
	assert.Contains(t, err4.Error(), "verification failed for backlog_item")

	// 5. Successful backlog item verification
	validStore := &testStubStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:             "backlog_item",
				objects.FieldKeyVerificationHash: expectedBLIHash,
			}, nil
		},
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, sCtx *sysStorageCtx, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{
				Objects: []map[string]any{
					{
						objects.FieldKeyID:                  "TST-1",
						objects.FieldKeyVerifiedArtifactRefs: []any{artifactRel},
					},
				},
			}, nil
		},
	}
	cmd5 := setupVerifyCompletionCmd(tempRoot, validStore)
	err5 := runVerifyCompletion(cmd5, "BLI-1")
	assert.NoError(t, err5)
}
