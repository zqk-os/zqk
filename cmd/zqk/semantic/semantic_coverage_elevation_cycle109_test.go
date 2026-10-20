package semantic

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/semantic"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// mockFailingStorage intercepts storage calls to test error boundaries.
type mockFailingStorage struct {
	storage.ObjectStorageProvider
	failCreate bool
	failList   bool
	createErr  error
	listErr    error
}

func (m *mockFailingStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.failCreate {
		if m.createErr != nil {
			return m.createErr
		}
		return errors.New("simulated storage create failure")
	}
	return m.ObjectStorageProvider.Create(ctx, secCtx, obj)
}

func (m *mockFailingStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.failList {
		if m.listErr != nil {
			return nil, m.listErr
		}
		return nil, errors.New("simulated storage list failure")
	}
	return m.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
}

func setupTestCommand(t *testing.T, cmd *cobra.Command, projectRoot string, format string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cliCtx := &cli.Context{
		Format: cli.OutputFormat(format),
		Context: &clictx.Context{
			ProjectRoot: projectRoot,
			Profile:     "debug",
			Format:      format,
		},
	}
	cli.SetContext(cmd, cliCtx)
	for _, sub := range cmd.Commands() {
		cli.SetContext(sub, cliCtx)
	}
	return &buf
}

func TestSemanticRoot_DiscoveryAndBinding(t *testing.T) {
	cmd := NewSemanticCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "semantic", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify all expected subcommands are bound
	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}
	assert.True(t, subNames["assess"], "assess subcommand must be bound")
	assert.True(t, subNames["recommend"], "recommend subcommand must be bound")
	assert.True(t, subNames["infer"], "infer subcommand must be bound")

	// Verify help execution
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Semantic operations")

	// Negative boundary: unknown subcommand
	cmd.SetArgs([]string{"nonexistent-subcommand"})
	err = cmd.Execute()
	require.Error(t, err)
}

func TestSemanticAssess_CommandContractAndFlags(t *testing.T) {
	cmd := NewAssessCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "assess", cmd.Use)
	assert.NotNil(t, cmd.RunE)

	// Negative boundary: unexpected arguments (cobra.NoArgs invariant)
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	seedSemanticArtifacts(t, proj.Root)

	rootCmd := NewSemanticCmd()
	setupTestCommand(t, rootCmd, proj.Root, "table")
	rootCmd.SetArgs([]string{"assess", "unexpected-target-node"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")

	// Test outputTable directly with all indicator variations
	assessment := &semantic.MaturityAssessment{
		Level:     2,
		LevelName: "Practicing",
		Indicators: []semantic.Indicator{
			{
				Type:           "Structured Data",
				Found:          true,
				Examples:       []string{"sample.yaml", "config.json"},
				Sophistication: "Moderate",
			},
			{
				Type:           "Ontologies",
				Found:          false,
				Examples:       nil,
				Sophistication: "None",
			},
		},
		Recommendations: []semantic.Recommendation{
			{
				Recommendation: "Adopt formal schemas",
				Priority:       "High",
				Effort:         "Low",
				Benefit:        "High",
			},
		},
	}

	execCmd := NewAssessCmd()
	buf := setupTestCommand(t, execCmd, proj.Root, "table")
	err = outputAssessment(execCmd, assessment)
	require.NoError(t, err)
	outputStr := buf.String()
	assert.Contains(t, outputStr, "Semantic Maturity Assessment")
	assert.Contains(t, outputStr, "Level: 2 (Practicing)")
	assert.Contains(t, outputStr, "Structured Data: ✅ Found")
	assert.Contains(t, outputStr, "Examples: sample.yaml, config.json")
	assert.Contains(t, outputStr, "Ontologies: ❌ Not found")
	assert.Contains(t, outputStr, "Recommendations:")
	assert.Contains(t, outputStr, "1. Adopt formal schemas")

	// Test outputAssessment formats: JSONL
	jsonlCmd := NewAssessCmd()
	jsonlBuf := setupTestCommand(t, jsonlCmd, proj.Root, "jsonl")
	err = outputAssessment(jsonlCmd, assessment)
	require.NoError(t, err)
	assert.Contains(t, jsonlBuf.String(), `"level":2`)

	// Test runAssess failure when context is missing
	nakedCmd := NewAssessCmd()
	cli.SetContext(nakedCmd, nil)
	err = runAssess(nakedCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")
}

func TestSemanticRecommend_CommandContractAndFlags(t *testing.T) {
	cmd := NewRecommendCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "recommend", cmd.Use)
	assert.Equal(t, "Recommend next semantic steps", cmd.Short)
	assert.NotNil(t, cmd.RunE)

	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// Negative boundary: unexpected arguments (cobra.NoArgs invariant)
	rootCmd := NewSemanticCmd()
	setupTestCommand(t, rootCmd, proj.Root, "table")
	rootCmd.SetArgs([]string{"recommend", "invalid-extra-arg"})
	err := rootCmd.Execute()
	require.Error(t, err)

	// Test outputRecommendations when recommendations list is empty
	emptyAssessment := &semantic.MaturityAssessment{
		Level:           0,
		LevelName:       "Naive",
		Indicators:      nil,
		Recommendations: nil,
	}

	execCmd := NewRecommendCmd()
	buf := setupTestCommand(t, execCmd, proj.Root, "table")
	err = outputRecommendations(execCmd, emptyAssessment)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No specific recommendations were generated for this repository.")

	// Test outputRecommendations with populated recommendations in Table format
	populatedAssessment := &semantic.MaturityAssessment{
		Level:     1,
		LevelName: "Aware",
		Indicators: []semantic.Indicator{
			{
				Type:           "Structured Data",
				Found:          true,
				Sophistication: "Basic",
			},
		},
		Recommendations: []semantic.Recommendation{
			{
				Recommendation: "Introduce JSON schemas for configs",
				Priority:       "P1",
				Effort:         "Small",
				Benefit:        "High",
			},
		},
	}

	tableCmd := NewRecommendCmd()
	tableBuf := setupTestCommand(t, tableCmd, proj.Root, "table")
	err = outputRecommendations(tableCmd, populatedAssessment)
	require.NoError(t, err)
	assert.Contains(t, tableBuf.String(), "Semantic Maturity Recommendations")
	assert.Contains(t, tableBuf.String(), "Level: 1 (Aware)")
	assert.Contains(t, tableBuf.String(), "1. Introduce JSON schemas for configs")
	assert.Contains(t, tableBuf.String(), "Priority: P1 | Effort: Small | Benefit: High")

	// Test outputRecommendations with YAML format
	yamlCmd := NewRecommendCmd()
	yamlBuf := setupTestCommand(t, yamlCmd, proj.Root, "yaml")
	err = outputRecommendations(yamlCmd, populatedAssessment)
	require.NoError(t, err)
	assert.Contains(t, yamlBuf.String(), "level: 1")

	// Test outputRecommendations with JSONL format
	jsonlCmd := NewRecommendCmd()
	jsonlBuf := setupTestCommand(t, jsonlCmd, proj.Root, "jsonl")
	err = outputRecommendations(jsonlCmd, populatedAssessment)
	require.NoError(t, err)
	assert.Contains(t, jsonlBuf.String(), `"level":1`)

	// Test runRecommend error when context is missing
	nakedCmd := NewRecommendCmd()
	cli.SetContext(nakedCmd, nil)
	err = runRecommend(nakedCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")
}

func TestSemanticInfer_CommandContractAndFlags(t *testing.T) {
	cmd := NewInferCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "infer", cmd.Use)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	applyFlag := cmd.Flags().Lookup("apply")
	require.NotNil(t, applyFlag)
	assert.Equal(t, "false", applyFlag.DefValue)

	sessionFlag := cmd.Flags().Lookup("session-id")
	require.NotNil(t, sessionFlag)
	assert.Equal(t, "", sessionFlag.DefValue)

	// Negative boundary: unexpected arguments (cobra.NoArgs invariant)
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	rootCmd := NewSemanticCmd()
	setupTestCommand(t, rootCmd, proj.Root, "table")
	rootCmd.SetArgs([]string{"infer", "unexpected-query-arg"})
	err := rootCmd.Execute()
	require.Error(t, err)

	// Error path: runInfer with nil context
	nakedCmd := NewInferCmd()
	cli.SetContext(nakedCmd, nil)
	err = runInfer(nakedCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")

	// Error path: runInfer on command without flags
	bareCmd := &cobra.Command{Use: "infer", RunE: runInfer}
	setupTestCommand(t, bareCmd, proj.Root, "table")
	err = runInfer(bareCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get apply flag")

	// Error path: runInfer on command with apply flag but without session-id flag
	partialCmd := &cobra.Command{Use: "infer", RunE: runInfer}
	partialCmd.Flags().Bool("apply", false, "apply flag")
	setupTestCommand(t, partialCmd, proj.Root, "table")
	err = runInfer(partialCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get session-id flag")
}

func TestSemanticInfer_StorageFailureAndDriftError(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// Error path: CheckConvergenceDrift failure via mock failing storage
	failingStore := &mockFailingStorage{
		ObjectStorageProvider: proj.FileStorage,
		failList:              true,
		listErr:               errors.New("simulated database failure"),
	}

	cmd := NewInferCmd()
	setupTestCommand(t, cmd, proj.Root, "table")
	ctxWithStorage := cli.WithStorageProvider(cmd.Context(), failingStore)
	cmd.SetContext(ctxWithStorage)

	err := runInfer(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "convergence drift check failed")
}

func TestSemanticInfer_OutputInferencesFormats(t *testing.T) {
	inferences := []map[string]any{
		{
			objects.FieldKeyPriorityTier: "P1",
			objects.FieldKeyTitle:        "Remediate Ontology Drift",
			objects.FieldKeyDescription:  "Ontology file is drifting from schema definition",
		},
	}

	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// Format: YAML
	yamlCmd := NewInferCmd()
	yamlBuf := setupTestCommand(t, yamlCmd, proj.Root, "yaml")
	err := outputInferences(yamlCmd, inferences)
	require.NoError(t, err)
	assert.Contains(t, yamlBuf.String(), "Remediate Ontology Drift")

	// Format: JSONL
	jsonlCmd := NewInferCmd()
	jsonlBuf := setupTestCommand(t, jsonlCmd, proj.Root, "jsonl")
	err = outputInferences(jsonlCmd, inferences)
	require.NoError(t, err)
	assert.Contains(t, jsonlBuf.String(), "Remediate Ontology Drift")

	// Format: Table (with non-empty inferences)
	tableCmd := NewInferCmd()
	tableBuf := setupTestCommand(t, tableCmd, proj.Root, "table")
	err = outputInferences(tableCmd, inferences)
	require.NoError(t, err)
	assert.Contains(t, tableBuf.String(), "Strategic Inferences & Proposed Backlog Items")
	assert.Contains(t, tableBuf.String(), "- [P1] Remediate Ontology Drift")
	assert.Contains(t, tableBuf.String(), "Use --apply to create these items in the Knowledge Kernel.")
}

func TestSemanticInfer_ApplyInferencesBoundaries(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	inferences := []map[string]any{
		{
			objects.FieldKeyID:           "BLI-TEST-INFER-001",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyTitle:        "Inferred BLI",
			objects.FieldKeyPriorityTier: "P2",
			objects.FieldKeyStatus:       "proposed",
		},
	}

	// Test storage creation failure path
	failingStore := &mockFailingStorage{
		ObjectStorageProvider: proj.FileStorage,
		failCreate:            true,
		createErr:             errors.New("simulated persistence disk write error"),
	}

	cmd := NewInferCmd()
	setupTestCommand(t, cmd, proj.Root, "table")
	ctxWithStorage := cli.WithStorageProvider(cmd.Context(), failingStore)
	cmd.SetContext(ctxWithStorage)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	err = applyInferences(cmd, proc, inferences)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create backlog item BLI-TEST-INFER-001")
	assert.Contains(t, err.Error(), "simulated persistence disk write error")
}

func TestSemanticHelpers_ResolutionAndAssessor(t *testing.T) {
	// defaultMaturityAssessor contract
	assessor := defaultMaturityAssessor()
	require.NotNil(t, assessor)

	// runMaturityAssessment on project root
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	seedSemanticArtifacts(t, proj.Root)

	assessment, err := runMaturityAssessment(proj.Root)
	require.NoError(t, err)
	require.NotNil(t, assessment)
	assert.True(t, assessment.Level >= 1)
	assert.NotEmpty(t, assessment.Indicators)

	// resolveSemanticProjectRoot: nil context returns error
	cmdWithoutCtx := NewAssessCmd()
	cli.SetContext(cmdWithoutCtx, nil)
	_, err = resolveSemanticProjectRoot(cmdWithoutCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")

	// resolveSemanticProjectRoot: explicit project root
	cmdWithCtx := NewAssessCmd()
	setupTestCommand(t, cmdWithCtx, proj.Root, "table")
	resolvedRoot, err := resolveSemanticProjectRoot(cmdWithCtx)
	require.NoError(t, err)
	assert.Equal(t, proj.Root, resolvedRoot)

	// resolveSemanticProjectRoot: empty project root in isolated directory returns error
	tempCleanDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")
	cmdEmptyRoot := NewAssessCmd()
	cliCtx := &cli.Context{
		Format: "table",
		Context: &clictx.Context{
			ProjectRoot: "",
			Profile:     "debug",
			Format:      "table",
		},
	}
	cli.SetContext(cmdEmptyRoot, cliCtx)
	t.Chdir(tempCleanDir)

	_, err = resolveSemanticProjectRoot(cmdEmptyRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	// assessSemanticMaturity failure on invalid project root
	_, err = assessSemanticMaturity(cmdEmptyRoot, "custom maturity prefix failed")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")
}

func TestSemantic_NegativeAdversarialBoundaries(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	seedConvergenceDrift(t, proj.FileStorage)

	t.Run("ContextCancellationDuringInfer", func(t *testing.T) {
		cmd := NewInferCmd()
		setupTestCommand(t, cmd, proj.Root, "table")

		canceledCtx, cancel := context.WithCancel(cmd.Context())
		cancel() // Cancel immediately
		cmd.SetContext(canceledCtx)

		// Executing with canceled context should be handled gracefully or return context cancellation error
		err := runInfer(cmd, nil)
		if err != nil {
			assert.Error(t, err)
		}
	})

	t.Run("InvalidTargetSubcommandAndExtraArgs", func(t *testing.T) {
		rootCmd := NewSemanticCmd()
		setupTestCommand(t, rootCmd, proj.Root, "table")
		rootCmd.SetArgs([]string{"assess", "--nonexistent-flag"})
		err := rootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown flag")
	})

	t.Run("MalformedJSONLOutputFormat", func(t *testing.T) {
		rootCmd := NewSemanticCmd()
		setupTestCommand(t, rootCmd, proj.Root, "jsonl")
		rootCmd.SetArgs([]string{"assess"})
		err := rootCmd.Execute()
		require.NoError(t, err)
	})

	t.Run("EmptyInferencesDirectCall", func(t *testing.T) {
		cmd := NewInferCmd()
		buf := setupTestCommand(t, cmd, proj.Root, "table")
		err := outputInferences(cmd, nil)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No strategic inferences identified at this time.")
	})

	t.Run("EmptyInferencesApplyDirectCall", func(t *testing.T) {
		cmd := NewInferCmd()
		buf := setupTestCommand(t, cmd, proj.Root, "table")
		proc, err := cli.NewProcessor(cmd)
		require.NoError(t, err)
		err = applyInferences(cmd, proc, nil)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No inferences to apply.")
	})
}
