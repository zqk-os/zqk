package reports

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReportsCmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewReportsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "reports", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["pcs"], "reports command must have pcs subcommand")
	assert.True(t, subNames["edd"], "reports command must have edd subcommand")
	assert.True(t, subNames["blockers"], "reports command must have blockers subcommand")
	assert.True(t, subNames["quick"], "reports command must have quick subcommand")
}

func TestReports_CLIExecution(t *testing.T) {
	root, storageProvider := setupTestProject(t)
	createTestData(t, storageProvider)
	t.Setenv("ZQK_PROJECT_ROOT", root)

	t.Run("PCS", func(t *testing.T) {
		// 1. Table format (default)
		cmdTable := NewReportsCmd()
		outTable := bytes.NewBuffer(nil)
		cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outTable))
		cmdTable.SetOut(outTable)
		cmdTable.SetErr(outTable)
		cmdTable.SetArgs([]string{"pcs"})

		err := cmdTable.Execute()
		require.NoError(t, err)
		assert.Contains(t, outTable.String(), "Project Confidence Score (PCS)")
		assert.Contains(t, outTable.String(), "Score:")

		// 2. JSON format
		cmdJSON := NewReportsCmd()
		outJSON := bytes.NewBuffer(nil)
		cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outJSON))
		cmdJSON.SetOut(outJSON)
		cmdJSON.SetErr(outJSON)
		cmdJSON.SetArgs([]string{"pcs", "--format", "json"})

		err = cmdJSON.Execute()
		require.NoError(t, err)
		assert.Contains(t, outJSON.String(), `"pcs"`)
		assert.Contains(t, outJSON.String(), `"status"`)

		// 3. YAML format
		cmdYAML := NewReportsCmd()
		outYAML := bytes.NewBuffer(nil)
		cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outYAML))
		cmdYAML.SetOut(outYAML)
		cmdYAML.SetErr(outYAML)
		cmdYAML.SetArgs([]string{"pcs", "--format", "yaml", "--include-commit-data"})

		err = cmdYAML.Execute()
		require.NoError(t, err)
		assert.Contains(t, outYAML.String(), "pcs:")
		assert.Contains(t, outYAML.String(), "status:")
	})

	t.Run("EDD", func(t *testing.T) {
		// 1. Table format (default)
		cmdTable := NewReportsCmd()
		outTable := bytes.NewBuffer(nil)
		cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outTable))
		cmdTable.SetOut(outTable)
		cmdTable.SetErr(outTable)
		cmdTable.SetArgs([]string{"edd"})

		err := cmdTable.Execute()
		require.NoError(t, err)
		assert.Contains(t, outTable.String(), "Effort Distribution Discrepancy (EDD)")
		assert.Contains(t, outTable.String(), "Score:")

		// 2. JSON format
		cmdJSON := NewReportsCmd()
		outJSON := bytes.NewBuffer(nil)
		cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outJSON))
		cmdJSON.SetOut(outJSON)
		cmdJSON.SetErr(outJSON)
		cmdJSON.SetArgs([]string{"edd", "--format", "json"})

		err = cmdJSON.Execute()
		require.NoError(t, err)
		assert.Contains(t, outJSON.String(), `"edd"`)
		assert.Contains(t, outJSON.String(), `"status"`)

		// 3. YAML format
		cmdYAML := NewReportsCmd()
		outYAML := bytes.NewBuffer(nil)
		cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outYAML))
		cmdYAML.SetOut(outYAML)
		cmdYAML.SetErr(outYAML)
		cmdYAML.SetArgs([]string{"edd", "--format", "yaml", "--include-commit-data"})

		err = cmdYAML.Execute()
		require.NoError(t, err)
		assert.Contains(t, outYAML.String(), "edd:")
		assert.Contains(t, outYAML.String(), "status:")
	})

	t.Run("Blockers", func(t *testing.T) {
		// 1. Table format (default)
		cmdTable := NewReportsCmd()
		outTable := bytes.NewBuffer(nil)
		cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outTable))
		cmdTable.SetOut(outTable)
		cmdTable.SetErr(outTable)
		cmdTable.SetArgs([]string{"blockers"})

		err := cmdTable.Execute()
		require.NoError(t, err)
		assert.Contains(t, outTable.String(), "Dependencies & Blockers (D&B)")

		// 2. JSON format
		cmdJSON := NewReportsCmd()
		outJSON := bytes.NewBuffer(nil)
		cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outJSON))
		cmdJSON.SetOut(outJSON)
		cmdJSON.SetErr(outJSON)
		cmdJSON.SetArgs([]string{"blockers", "--format", "json"})

		err = cmdJSON.Execute()
		require.NoError(t, err)
		assert.Contains(t, outJSON.String(), `"summary"`)
		assert.Contains(t, outJSON.String(), `"dependencies"`)
		assert.Contains(t, outJSON.String(), `"blockers"`)

		// 3. YAML format
		cmdYAML := NewReportsCmd()
		outYAML := bytes.NewBuffer(nil)
		cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outYAML))
		cmdYAML.SetOut(outYAML)
		cmdYAML.SetErr(outYAML)
		cmdYAML.SetArgs([]string{"blockers", "--format", "yaml", "--include-commit-data"})

		err = cmdYAML.Execute()
		require.NoError(t, err)
		assert.Contains(t, outYAML.String(), "summary:")
		assert.Contains(t, outYAML.String(), "dependencies:")
	})
}

func TestReports_OutputPCSTable_AllTiers(t *testing.T) {
	scores := []struct {
		score    float64
		expected string
	}{
		{90.0, "Status: Excellent"},
		{70.0, "Status: Good"},
		{50.0, "Status: Fair"},
		{30.0, "Status: Poor"},
		{10.0, "Status: Critical"},
	}

	for _, tc := range scores {
		cmd := NewPCSCmd()
		out := bytes.NewBuffer(nil)
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
		cmd.SetOut(out)
		cmd.SetErr(out)

		err := outputPCSTable(cmd, tc.score)
		require.NoError(t, err)
		assert.Contains(t, out.String(), tc.expected)
	}
}

func TestReports_OutputEDDTable_AllTiers(t *testing.T) {
	scores := []struct {
		score    float64
		expected string
	}{
		{30.0, "Status: Significant Underestimation"},
		{10.0, "Status: Moderate Underestimation"},
		{0.0, "Status: Good Estimation Accuracy"},
		{-10.0, "Status: Moderate Overestimation"},
		{-30.0, "Status: Significant Overestimation"},
	}

	for _, tc := range scores {
		cmd := NewEDDCmd()
		out := bytes.NewBuffer(nil)
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
		cmd.SetOut(out)
		cmd.SetErr(out)

		err := outputEDDTable(cmd, tc.score)
		require.NoError(t, err)
		assert.Contains(t, out.String(), tc.expected)
	}
}

func TestReports_OutputBlockersTable_Details(t *testing.T) {
	// 1. Empty D&B
	cmdEmpty := NewBlockersCmd()
	outEmpty := bytes.NewBuffer(nil)
	cmdEmpty.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outEmpty))
	cmdEmpty.SetOut(outEmpty)
	cmdEmpty.SetErr(outEmpty)

	err := outputBlockersTable(cmdEmpty, &metrics.DependenciesBlockers{})
	require.NoError(t, err)
	assert.Contains(t, outEmpty.String(), "Dependencies: None identified")
	assert.Contains(t, outEmpty.String(), "Blockers: None identified")

	// 2. Populated D&B
	db := &metrics.DependenciesBlockers{
		Summary: "Project risk summary",
		Dependencies: []metrics.DependencyItem{
			{
				Type:        "code",
				Severity:    "high",
				Description: "tight coupling between modules",
			},
		},
		Blockers: []metrics.BlockerItem{
			{
				Type:        "work_item",
				Severity:    "critical",
				Description: "blocked on external API",
			},
		},
		ActionItems: []metrics.ActionItem{
			{
				Priority:    "P1",
				Description: "Decouple auth from core",
			},
		},
	}

	cmdPopulated := NewBlockersCmd()
	outPopulated := bytes.NewBuffer(nil)
	cmdPopulated.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outPopulated))
	cmdPopulated.SetOut(outPopulated)
	cmdPopulated.SetErr(outPopulated)

	err = outputBlockersTable(cmdPopulated, db)
	require.NoError(t, err)
	assert.Contains(t, outPopulated.String(), "Summary: Project risk summary")
	assert.Contains(t, outPopulated.String(), "Dependencies (1):")
	assert.Contains(t, outPopulated.String(), "[high] code - tight coupling")
	assert.Contains(t, outPopulated.String(), "Blockers (1):")
	assert.Contains(t, outPopulated.String(), "[critical] work_item - blocked on external API")
	assert.Contains(t, outPopulated.String(), "Action Items (1):")
	assert.Contains(t, outPopulated.String(), "[P1] Decouple auth from core")
}
func TestReports_CalculateMetrics_Errors(t *testing.T) {
	// Storage initialization failure with invalid directory
	tmpFile := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, fileutil.WriteFile(tmpFile, []byte("invalid"), paths.FilePerm644))
	t.Setenv("ZQK_PROJECT_ROOT", tmpFile)

	cmd := NewPCSCmd()
	cmd.SetContext(context.Background())

	_, err := calculateReportMetrics(cmd, "PCS")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize storage")
}
